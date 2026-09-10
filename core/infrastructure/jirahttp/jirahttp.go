// Package jirahttp は Jira REST API を net/http で叩く port.JiraClient 実装を提供する。
// Jira Cloud(API v3 / Basic 認証)と Server / Data Center(API v2 / Bearer PAT)の
// 両方に対応する。読み取り専用で、書き込み API は呼ばない。
package jirahttp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/ysksm/multi-terminals/core/application/port"
	"github.com/ysksm/multi-terminals/core/domain/task"
)

// コンパイル時インターフェース適合確認
var _ port.JiraClient = (*Client)(nil)

// Factory は設定とトークンからクライアントを作る port.JiraClientFactory。
var Factory port.JiraClientFactory = func(cfg task.JiraConfig, token string) port.JiraClient {
	return New(cfg, token)
}

// defaultTimeout は 1 リクエストの上限時間。
const defaultTimeout = 15 * time.Second

// カスタムフィールド名 → FieldIDs のキー。
const (
	fieldSprint   = "Sprint"
	fieldEpicLink = "Epic Link"
	fieldEpicName = "Epic Name"
)

// Client は port.JiraClient の HTTP 実装。
type Client struct {
	cfg   task.JiraConfig
	token string
	http  *http.Client

	// fieldIDs は名前 → customfield ID のキャッシュ。初回の必要時に /field で解決し、
	// 以後はクライアントの寿命の間使い回す。
	mu             sync.Mutex
	fieldIDs       map[string]string
	fieldsResolved bool
}

// New は既定の HTTP クライアント(タイムアウト 15 秒)で Client を返す。
func New(cfg task.JiraConfig, token string) port.JiraClient {
	return NewWithHTTPClient(cfg, token, &http.Client{Timeout: defaultTimeout})
}

// NewWithHTTPClient は HTTP クライアントを差し替えて Client を返す(テスト用)。
func NewWithHTTPClient(cfg task.JiraConfig, token string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: defaultTimeout}
	}
	c := &Client{cfg: cfg, token: token, http: hc, fieldIDs: map[string]string{}}
	if len(cfg.FieldIDs) > 0 {
		for k, v := range cfg.FieldIDs {
			c.fieldIDs[k] = v
		}
		c.fieldsResolved = true
	}
	return c
}

// FieldIDs は解決済みのカスタムフィールド ID を返す(設定へキャッシュしたいときに使う)。
func (c *Client) FieldIDs() map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]string, len(c.fieldIDs))
	for k, v := range c.fieldIDs {
		out[k] = v
	}
	return out
}

// apiVersion は種類ごとの REST API バージョン。
func (c *Client) apiVersion() string {
	if c.cfg.Kind == task.JiraCloud {
		return "3"
	}
	return "2"
}

// baseURL は末尾スラッシュを除いた接続先。
func (c *Client) baseURL() string {
	return strings.TrimRight(strings.TrimSpace(c.cfg.BaseURL), "/")
}

// apiURL は /rest/api/{v}/<path> の絶対 URL。
func (c *Client) apiURL(path string) string {
	return c.baseURL() + "/rest/api/" + c.apiVersion() + "/" + strings.TrimLeft(path, "/")
}

// authorize は種類に応じた Authorization ヘッダを付ける。
func (c *Client) authorize(req *http.Request) {
	if c.cfg.Kind == task.JiraCloud {
		cred := base64.StdEncoding.EncodeToString([]byte(c.cfg.Email + ":" + c.token))
		req.Header.Set("Authorization", "Basic "+cred)
		return
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
}

// httpError は 2xx 以外のレスポンス。ステータスと本文の先頭だけを保持する。
type httpError struct {
	Status int
	Body   string
}

func (e *httpError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.Status, e.Body)
}

// do はリクエストを送り、2xx なら本文を返す。それ以外は種別付きのエラーにする。
func (c *Client) do(ctx context.Context, method, rawURL string, body any) ([]byte, error) {
	if !c.cfg.IsConfigured() {
		return nil, port.ErrJiraNotConfigured
	}
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("jira: encode request: %w", err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, reader)
	if err != nil {
		return nil, fmt.Errorf("jira: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	c.authorize(req)

	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jira: %s %s: %v: %w", method, rawURL, err, port.ErrJiraUnavailable)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("jira: read response: %v: %w", err, port.ErrJiraUnavailable)
	}
	if res.StatusCode >= 200 && res.StatusCode < 300 {
		return data, nil
	}
	he := &httpError{Status: res.StatusCode, Body: excerpt(data)}
	switch {
	case res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden:
		return nil, fmt.Errorf("jira: %s %s: %v: %w", method, rawURL, he, port.ErrJiraAuth)
	case res.StatusCode >= 500:
		return nil, fmt.Errorf("jira: %s %s: %v: %w", method, rawURL, he, port.ErrJiraUnavailable)
	default:
		return nil, fmt.Errorf("jira: %s %s: %w", method, rawURL, he)
	}
}

// excerpt はエラーメッセージ用に本文を短く切り詰める。
func excerpt(b []byte) string {
	s := strings.TrimSpace(string(b))
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}

// TestConnection は /myself で認証を確認する。
func (c *Client) TestConnection(ctx context.Context) (port.JiraUser, error) {
	data, err := c.do(ctx, http.MethodGet, c.apiURL("myself"), nil)
	if err != nil {
		return port.JiraUser{}, err
	}
	var me struct {
		DisplayName  string `json:"displayName"`
		EmailAddress string `json:"emailAddress"`
	}
	if err := json.Unmarshal(data, &me); err != nil {
		return port.JiraUser{}, fmt.Errorf("jira: decode /myself: %w", err)
	}
	return port.JiraUser{DisplayName: me.DisplayName, Email: me.EmailAddress}, nil
}

// normalizeKeys は空白除去・大文字化・空要素除去を行う。
func normalizeKeys(keys []string) []string {
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		k = strings.ToUpper(strings.TrimSpace(k))
		if k != "" {
			out = append(out, k)
		}
	}
	return out
}

// FetchIssues は指定番号の issue を取得する。1 件なら issue エンドポイント、複数なら
// key in (...) の検索を使い、足りない番号があれば ErrJiraNotFound を返す。
func (c *Client) FetchIssues(ctx context.Context, keys []string) ([]port.JiraIssue, error) {
	keys = normalizeKeys(keys)
	if len(keys) == 0 {
		return nil, errors.New("jira: at least one issue key is required")
	}
	fields, err := c.searchFields(ctx)
	if err != nil {
		return nil, err
	}

	if len(keys) == 1 {
		key := keys[0]
		q := url.Values{"fields": {strings.Join(fields, ",")}}
		data, err := c.do(ctx, http.MethodGet, c.apiURL("issue/"+url.PathEscape(key))+"?"+q.Encode(), nil)
		if err != nil {
			var he *httpError
			if errors.As(err, &he) && he.Status == http.StatusNotFound {
				return nil, fmt.Errorf("jira: issue %s: %w", key, port.ErrJiraNotFound)
			}
			return nil, err
		}
		var raw rawIssue
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("jira: decode issue %s: %w", key, err)
		}
		return []port.JiraIssue{c.toIssue(raw)}, nil
	}

	jql := "key in (" + strings.Join(keys, ",") + ")"
	issues, err := c.search(ctx, jql, len(keys), fields)
	if err != nil {
		return nil, err
	}
	found := make(map[string]port.JiraIssue, len(issues))
	for _, is := range issues {
		found[is.Key] = is
	}
	var missing []string
	out := make([]port.JiraIssue, 0, len(keys))
	for _, k := range keys {
		is, ok := found[k]
		if !ok {
			missing = append(missing, k)
			continue
		}
		out = append(out, is)
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("jira: issues %s: %w", strings.Join(missing, ", "), port.ErrJiraNotFound)
	}
	return out, nil
}

// Search は JQL で検索する。max は上限件数(0 以下なら 50)。
func (c *Client) Search(ctx context.Context, jql string, max int) ([]port.JiraIssue, error) {
	if strings.TrimSpace(jql) == "" {
		return nil, errors.New("jira: jql is required")
	}
	if max <= 0 {
		max = 50
	}
	fields, err := c.searchFields(ctx)
	if err != nil {
		return nil, err
	}
	return c.search(ctx, jql, max, fields)
}

// searchResponse は検索結果の共通形(v2/v3 とも issues 配列)。
type searchResponse struct {
	Issues []rawIssue `json:"issues"`
}

// search は種類に応じたエンドポイントで検索し、issue に変換する。
// Cloud は /search/jql(新 API)を使い、404/410 が返る古いサイトでは /search に落とす。
func (c *Client) search(ctx context.Context, jql string, max int, fields []string) ([]port.JiraIssue, error) {
	body := map[string]any{"jql": jql, "maxResults": max, "fields": fields}
	var data []byte
	var err error
	if c.cfg.Kind == task.JiraCloud {
		data, err = c.do(ctx, http.MethodPost, c.apiURL("search/jql"), body)
		var he *httpError
		if err != nil && errors.As(err, &he) && (he.Status == http.StatusNotFound || he.Status == http.StatusGone) {
			q := url.Values{
				"jql":        {jql},
				"maxResults": {fmt.Sprint(max)},
				"fields":     {strings.Join(fields, ",")},
			}
			data, err = c.do(ctx, http.MethodGet, c.apiURL("search")+"?"+q.Encode(), nil)
		}
	} else {
		data, err = c.do(ctx, http.MethodPost, c.apiURL("search"), body)
	}
	if err != nil {
		return nil, err
	}
	var res searchResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("jira: decode search: %w", err)
	}
	out := make([]port.JiraIssue, 0, len(res.Issues))
	for _, raw := range res.Issues {
		out = append(out, c.toIssue(raw))
	}
	return out, nil
}

// ---- フィールド解決 ----

// baseFields は常に要求する標準フィールド。
var baseFields = []string{"summary", "status", "assignee", "priority", "description", "parent"}

// searchFields は要求するフィールド一覧(標準 + 解決済みカスタムフィールド)。
func (c *Client) searchFields(ctx context.Context) ([]string, error) {
	ids, err := c.resolveFieldIDs(ctx)
	if err != nil {
		return nil, err
	}
	fields := append([]string(nil), baseFields...)
	for _, name := range []string{fieldSprint, fieldEpicLink, fieldEpicName} {
		if id := ids[name]; id != "" {
			fields = append(fields, id)
		}
	}
	return fields, nil
}

// resolveFieldIDs は Sprint / Epic Link / Epic Name のカスタムフィールド ID を
// /field から解決してキャッシュする。設定に FieldIDs があれば問い合わせない。
func (c *Client) resolveFieldIDs(ctx context.Context) (map[string]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fieldsResolved {
		return c.fieldIDs, nil
	}
	data, err := c.do(ctx, http.MethodGet, c.apiURL("field"), nil)
	if err != nil {
		return nil, err
	}
	var fields []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, fmt.Errorf("jira: decode /field: %w", err)
	}
	for _, f := range fields {
		switch f.Name {
		case fieldSprint, fieldEpicLink, fieldEpicName:
			if _, dup := c.fieldIDs[f.Name]; !dup {
				c.fieldIDs[f.Name] = f.ID
			}
		}
	}
	c.fieldsResolved = true
	return c.fieldIDs, nil
}

// ---- レスポンス → port.JiraIssue ----

// rawIssue は issue レスポンスの生データ。fields はカスタムフィールドが混ざるため
// map で受けて必要なものだけ取り出す。
type rawIssue struct {
	Key    string                     `json:"key"`
	Fields map[string]json.RawMessage `json:"fields"`
}

// named は {"name": "..."} 形式の値。
type named struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
}

// toIssue は生データを port.JiraIssue に変換する。取り出せない項目は空のまま。
func (c *Client) toIssue(raw rawIssue) port.JiraIssue {
	is := port.JiraIssue{Key: raw.Key, URL: c.baseURL() + "/browse/" + raw.Key}
	f := raw.Fields

	is.Summary = decodeString(f["summary"])

	var status struct {
		Name           string `json:"name"`
		StatusCategory struct {
			Key string `json:"key"`
		} `json:"statusCategory"`
	}
	if unmarshal(f["status"], &status) {
		is.Status = status.Name
		is.StatusCategory = status.StatusCategory.Key
	}

	var assignee named
	if unmarshal(f["assignee"], &assignee) {
		is.Assignee = assignee.DisplayName
	}
	var priority named
	if unmarshal(f["priority"], &priority) {
		is.Priority = priority.Name
	}

	is.Description = descriptionText(f["description"])

	ids := c.FieldIDs()
	if id := ids[fieldSprint]; id != "" {
		is.Sprint = sprintName(f[id])
	}
	is.Epic = c.epicOf(f, ids)
	return is
}

// epicOf はエピック表示文字列を決める。Cloud では parent(issuetype が Epic)を
// 「KEY 概要」で優先し、無ければ Epic Link のキーをそのまま使う。
func (c *Client) epicOf(f map[string]json.RawMessage, ids map[string]string) string {
	var parent struct {
		Key    string `json:"key"`
		Fields struct {
			Summary   string `json:"summary"`
			IssueType named  `json:"issuetype"`
		} `json:"fields"`
	}
	if unmarshal(f["parent"], &parent) && parent.Key != "" &&
		strings.EqualFold(parent.Fields.IssueType.Name, "Epic") {
		return strings.TrimSpace(parent.Key + " " + parent.Fields.Summary)
	}
	if id := ids[fieldEpicLink]; id != "" {
		if key := decodeString(f[id]); key != "" {
			return key
		}
	}
	return ""
}

// unmarshal は raw が非空かつ null でないときに v へデコードし、成功したら true。
func unmarshal(raw json.RawMessage, v any) bool {
	if len(raw) == 0 || string(raw) == "null" {
		return false
	}
	return json.Unmarshal(raw, v) == nil
}

// decodeString は JSON 文字列を取り出す。文字列でなければ空。
func decodeString(raw json.RawMessage) string {
	var s string
	if unmarshal(raw, &s) {
		return s
	}
	return ""
}

// descriptionText は description をプレーンテキストにする。文字列(Server)は
// そのまま、オブジェクト(Cloud の ADF)はテキストノードを抽出する。
func descriptionText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var doc any
	if json.Unmarshal(raw, &doc) != nil {
		return ""
	}
	return adfToText(doc)
}

// adfBlockTypes は末尾で改行するブロックノード。
var adfBlockTypes = map[string]bool{
	"paragraph": true, "heading": true, "listItem": true, "codeBlock": true,
	"blockquote": true, "tableRow": true, "panel": true, "rule": true, "mediaGroup": true,
}

// adfToText は Atlassian Document Format のノード木からテキストだけを取り出す。
// text ノードを連結し、ブロック要素の後ろと hardBreak で改行する。
func adfToText(node any) string {
	var b strings.Builder
	walkADF(node, &b)
	return strings.TrimRight(b.String(), "\n")
}

func walkADF(node any, b *strings.Builder) {
	switch n := node.(type) {
	case map[string]any:
		typ, _ := n["type"].(string)
		switch typ {
		case "text":
			if t, ok := n["text"].(string); ok {
				b.WriteString(t)
			}
			return
		case "hardBreak":
			b.WriteByte('\n')
			return
		case "mention", "emoji":
			if attrs, ok := n["attrs"].(map[string]any); ok {
				if t, ok := attrs["text"].(string); ok {
					b.WriteString(t)
				}
			}
			return
		}
		if content, ok := n["content"].([]any); ok {
			for _, child := range content {
				walkADF(child, b)
			}
		}
		if adfBlockTypes[typ] {
			b.WriteByte('\n')
		}
	case []any:
		for _, child := range n {
			walkADF(child, b)
		}
	}
}

// sprintNameRe は Server 形式の文字列 "…[id=1,…,name=Sprint 42,…]" から name= を抜く。
var sprintNameRe = regexp.MustCompile(`(?:\[|,)name=([^,\]]*)`)

// sprintName は Sprint フィールドから表示名を決める。配列なら最後(最新)の要素を使う。
// 要素はオブジェクト({name}) でも Server 形式の文字列でもよい。
func sprintName(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var items []json.RawMessage
	if json.Unmarshal(raw, &items) != nil {
		items = []json.RawMessage{raw}
	}
	for i := len(items) - 1; i >= 0; i-- {
		if name := sprintItemName(items[i]); name != "" {
			return name
		}
	}
	return ""
}

func sprintItemName(raw json.RawMessage) string {
	var obj named
	if unmarshal(raw, &obj) && obj.Name != "" {
		return obj.Name
	}
	s := decodeString(raw)
	if m := sprintNameRe.FindStringSubmatch(s); m != nil {
		return strings.TrimSpace(m[1])
	}
	return ""
}
