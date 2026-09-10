package jirahttp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ysksm/multi-terminals/core/application/port"
	"github.com/ysksm/multi-terminals/core/domain/task"
)

// fieldList は /field のスタブ応答。
var fieldList = `[
  {"id":"summary","name":"Summary"},
  {"id":"customfield_10020","name":"Sprint"},
  {"id":"customfield_10014","name":"Epic Link"},
  {"id":"customfield_10011","name":"Epic Name"}
]`

// issueJSON は 1 issue のスタブ。sprint / description は引数で差し替える。
func issueJSON(key, sprint, description string) string {
	return `{"key":"` + key + `","fields":{
    "summary":"概要 ` + key + `",
    "status":{"name":"In Progress","statusCategory":{"key":"indeterminate"}},
    "assignee":{"displayName":"笠松"},
    "priority":{"name":"High"},
    "description":` + description + `,
    "customfield_10020":` + sprint + `,
    "customfield_10014":"PROJ-1200",
    "parent":{"key":"PROJ-1200","fields":{"summary":"外部サービス連携","issuetype":{"name":"Epic"}}}
  }}`
}

type stub struct {
	fieldCalls int32
	lastAuth   string
	lastPath   string
	lastMethod string
	lastBody   string
	handler    func(w http.ResponseWriter, r *http.Request) bool // true なら処理済み
}

func newServer(t *testing.T, s *stub) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.lastAuth = r.Header.Get("Authorization")
		s.lastPath = r.URL.Path
		s.lastMethod = r.Method
		var b strings.Builder
		if r.Body != nil {
			buf := make([]byte, 4096)
			n, _ := r.Body.Read(buf)
			b.Write(buf[:n])
		}
		s.lastBody = b.String()
		if s.handler != nil && s.handler(w, r) {
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/field"):
			atomic.AddInt32(&s.fieldCalls, 1)
			_, _ = w.Write([]byte(fieldList))
		case strings.HasSuffix(r.URL.Path, "/myself"):
			_, _ = w.Write([]byte(`{"displayName":"Kasa","emailAddress":"k@example.com"}`))
		case strings.Contains(r.URL.Path, "/issue/"):
			key := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			if key == "PROJ-404" {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"errorMessages":["Issue does not exist"]}`))
				return
			}
			_, _ = w.Write([]byte(issueJSON(key, `[{"name":"Sprint 42"}]`, `"plain text"`)))
		default:
			http.NotFound(w, r)
		}
	}))
}

func cloudCfg(u string) task.JiraConfig {
	return task.JiraConfig{Kind: task.JiraCloud, BaseURL: u + "/", Email: "me@example.com"}
}

func serverCfg(u string) task.JiraConfig {
	return task.JiraConfig{Kind: task.JiraServer, BaseURL: u}
}

func TestCloud_BasicAuthAndV3Paths(t *testing.T) {
	s := &stub{}
	srv := newServer(t, s)
	defer srv.Close()
	c := NewWithHTTPClient(cloudCfg(srv.URL), "tok", srv.Client())

	issues, err := c.FetchIssues(context.Background(), []string{" proj-1 "})
	if err != nil {
		t.Fatalf("FetchIssues: %v", err)
	}
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("me@example.com:tok"))
	if s.lastAuth != want {
		t.Errorf("auth = %q, want %q", s.lastAuth, want)
	}
	if s.lastPath != "/rest/api/3/issue/PROJ-1" {
		t.Errorf("path = %q", s.lastPath)
	}
	is := issues[0]
	if is.Key != "PROJ-1" || is.Summary != "概要 PROJ-1" || is.Status != "In Progress" ||
		is.StatusCategory != "indeterminate" || is.Assignee != "笠松" || is.Priority != "High" ||
		is.Sprint != "Sprint 42" || is.Description != "plain text" {
		t.Errorf("issue = %+v", is)
	}
	if is.Epic != "PROJ-1200 外部サービス連携" {
		t.Errorf("epic = %q (parent should win on cloud)", is.Epic)
	}
	if is.URL != srv.URL+"/browse/PROJ-1" {
		t.Errorf("url = %q", is.URL)
	}
}

func TestServer_BearerAndV2Paths(t *testing.T) {
	s := &stub{}
	srv := newServer(t, s)
	defer srv.Close()
	c := NewWithHTTPClient(serverCfg(srv.URL), "pat", srv.Client())

	u, err := c.TestConnection(context.Background())
	if err != nil {
		t.Fatalf("TestConnection: %v", err)
	}
	if u.DisplayName != "Kasa" || u.Email != "k@example.com" {
		t.Errorf("user = %+v", u)
	}
	if s.lastAuth != "Bearer pat" {
		t.Errorf("auth = %q", s.lastAuth)
	}
	if s.lastPath != "/rest/api/2/myself" {
		t.Errorf("path = %q", s.lastPath)
	}
}

func TestFetchIssues_MultiWithMissing(t *testing.T) {
	s := &stub{}
	s.handler = func(w http.ResponseWriter, r *http.Request) bool {
		if strings.HasSuffix(r.URL.Path, "/search") && r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"issues":[` + issueJSON("PROJ-1", "null", "null") + `]}`))
			return true
		}
		return false
	}
	srv := newServer(t, s)
	defer srv.Close()
	c := NewWithHTTPClient(serverCfg(srv.URL), "pat", srv.Client())

	_, err := c.FetchIssues(context.Background(), []string{"PROJ-1", "PROJ-2"})
	if !errors.Is(err, port.ErrJiraNotFound) {
		t.Fatalf("want ErrJiraNotFound, got %v", err)
	}
	if !strings.Contains(err.Error(), "PROJ-2") {
		t.Errorf("missing key not named: %v", err)
	}
	if !strings.Contains(s.lastBody, `key in (PROJ-1,PROJ-2)`) {
		t.Errorf("search body = %s", s.lastBody)
	}
}

func TestFetchIssues_MultiSuccessKeepsOrder(t *testing.T) {
	s := &stub{}
	s.handler = func(w http.ResponseWriter, r *http.Request) bool {
		if strings.HasSuffix(r.URL.Path, "/search/jql") {
			_, _ = w.Write([]byte(`{"issues":[` + issueJSON("PROJ-2", "null", "null") + `,` + issueJSON("PROJ-1", "null", "null") + `]}`))
			return true
		}
		return false
	}
	srv := newServer(t, s)
	defer srv.Close()
	c := NewWithHTTPClient(cloudCfg(srv.URL), "tok", srv.Client())

	issues, err := c.FetchIssues(context.Background(), []string{"PROJ-1", "PROJ-2"})
	if err != nil {
		t.Fatalf("FetchIssues: %v", err)
	}
	if len(issues) != 2 || issues[0].Key != "PROJ-1" || issues[1].Key != "PROJ-2" {
		t.Errorf("issues = %+v", issues)
	}
	if s.lastPath != "/rest/api/3/search/jql" || s.lastMethod != http.MethodPost {
		t.Errorf("path/method = %q %q", s.lastPath, s.lastMethod)
	}
}

func TestCloud_SearchFallsBackToLegacyOn410(t *testing.T) {
	s := &stub{}
	s.handler = func(w http.ResponseWriter, r *http.Request) bool {
		if strings.HasSuffix(r.URL.Path, "/search/jql") {
			w.WriteHeader(http.StatusGone)
			return true
		}
		if strings.HasSuffix(r.URL.Path, "/search") && r.Method == http.MethodGet {
			if r.URL.Query().Get("jql") != "project = PROJ" {
				t.Errorf("jql query = %q", r.URL.Query().Get("jql"))
			}
			_, _ = w.Write([]byte(`{"issues":[` + issueJSON("PROJ-9", "null", "null") + `]}`))
			return true
		}
		return false
	}
	srv := newServer(t, s)
	defer srv.Close()
	c := NewWithHTTPClient(cloudCfg(srv.URL), "tok", srv.Client())

	issues, err := c.Search(context.Background(), "project = PROJ", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(issues) != 1 || issues[0].Key != "PROJ-9" {
		t.Errorf("issues = %+v", issues)
	}
}

func TestSingleFetch_404(t *testing.T) {
	s := &stub{}
	srv := newServer(t, s)
	defer srv.Close()
	c := NewWithHTTPClient(serverCfg(srv.URL), "pat", srv.Client())

	_, err := c.FetchIssues(context.Background(), []string{"PROJ-404"})
	if !errors.Is(err, port.ErrJiraNotFound) || !strings.Contains(err.Error(), "PROJ-404") {
		t.Fatalf("want ErrJiraNotFound naming key, got %v", err)
	}
}

func TestErrors_AuthAndUnavailable(t *testing.T) {
	for _, tc := range []struct {
		status int
		want   error
	}{
		{http.StatusUnauthorized, port.ErrJiraAuth},
		{http.StatusForbidden, port.ErrJiraAuth},
		{http.StatusBadGateway, port.ErrJiraUnavailable},
	} {
		s := &stub{}
		s.handler = func(w http.ResponseWriter, r *http.Request) bool {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(`{"errorMessages":["nope"]}`))
			return true
		}
		srv := newServer(t, s)
		_, err := NewWithHTTPClient(serverCfg(srv.URL), "pat", srv.Client()).TestConnection(context.Background())
		srv.Close()
		if !errors.Is(err, tc.want) {
			t.Errorf("status %d: want %v, got %v", tc.status, tc.want, err)
		}
		if !strings.Contains(err.Error(), "nope") {
			t.Errorf("status %d: body excerpt missing: %v", tc.status, err)
		}
	}

	// 接続不能(閉じたサーバ)
	s := &stub{}
	srv := newServer(t, s)
	u := srv.URL
	srv.Close()
	_, err := NewWithHTTPClient(serverCfg(u), "pat", &http.Client{}).TestConnection(context.Background())
	if !errors.Is(err, port.ErrJiraUnavailable) {
		t.Errorf("closed server: want ErrJiraUnavailable, got %v", err)
	}
}

func TestNotConfigured(t *testing.T) {
	c := NewWithHTTPClient(task.JiraConfig{Kind: task.JiraCloud, BaseURL: "https://x.atlassian.net"}, "t", nil)
	_, err := c.TestConnection(context.Background())
	if !errors.Is(err, port.ErrJiraNotConfigured) {
		t.Fatalf("want ErrJiraNotConfigured, got %v", err)
	}
	if _, err := c.FetchIssues(context.Background(), nil); err == nil {
		t.Error("empty keys should fail")
	}
}

func TestFieldIDs_ResolvedOnceAndCached(t *testing.T) {
	s := &stub{}
	srv := newServer(t, s)
	defer srv.Close()
	c := NewWithHTTPClient(serverCfg(srv.URL), "pat", srv.Client())

	for i := 0; i < 3; i++ {
		if _, err := c.FetchIssues(context.Background(), []string{"PROJ-1"}); err != nil {
			t.Fatalf("FetchIssues #%d: %v", i, err)
		}
	}
	if n := atomic.LoadInt32(&s.fieldCalls); n != 1 {
		t.Errorf("/field called %d times, want 1", n)
	}
	ids := c.FieldIDs()
	if ids["Sprint"] != "customfield_10020" || ids["Epic Link"] != "customfield_10014" {
		t.Errorf("ids = %v", ids)
	}
	// フィールド要求に解決した ID が含まれる
	if q := s.lastPath; q != "/rest/api/2/issue/PROJ-1" {
		t.Errorf("path = %q", q)
	}
}

func TestFieldIDs_PreconfiguredSkipsFieldCall(t *testing.T) {
	s := &stub{}
	srv := newServer(t, s)
	defer srv.Close()
	cfg := serverCfg(srv.URL)
	cfg.FieldIDs = map[string]string{"Sprint": "customfield_10020"}
	c := NewWithHTTPClient(cfg, "pat", srv.Client())

	if _, err := c.FetchIssues(context.Background(), []string{"PROJ-1"}); err != nil {
		t.Fatalf("FetchIssues: %v", err)
	}
	if n := atomic.LoadInt32(&s.fieldCalls); n != 0 {
		t.Errorf("/field called %d times, want 0", n)
	}
}

func TestServer_EpicLinkWhenNoEpicParent(t *testing.T) {
	s := &stub{}
	s.handler = func(w http.ResponseWriter, r *http.Request) bool {
		if strings.Contains(r.URL.Path, "/issue/") {
			_, _ = w.Write([]byte(`{"key":"PROJ-3","fields":{"summary":"x",
			  "parent":{"key":"PROJ-2","fields":{"summary":"story","issuetype":{"name":"Story"}}},
			  "customfield_10014":"PROJ-1200"}}`))
			return true
		}
		return false
	}
	srv := newServer(t, s)
	defer srv.Close()
	c := NewWithHTTPClient(serverCfg(srv.URL), "pat", srv.Client())
	issues, err := c.FetchIssues(context.Background(), []string{"PROJ-3"})
	if err != nil {
		t.Fatal(err)
	}
	if issues[0].Epic != "PROJ-1200" {
		t.Errorf("epic = %q, want Epic Link key", issues[0].Epic)
	}
}

func TestADFToText(t *testing.T) {
	doc := `{"type":"doc","version":1,"content":[
	  {"type":"heading","attrs":{"level":1},"content":[{"type":"text","text":"見出し"}]},
	  {"type":"paragraph","content":[
	    {"type":"text","text":"一行目"},{"type":"hardBreak"},{"type":"text","text":"二行目 "},
	    {"type":"mention","attrs":{"id":"1","text":"@笠松"}}
	  ]},
	  {"type":"bulletList","content":[
	    {"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"項目A"}]}]},
	    {"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"項目B"}]}]}
	  ]},
	  {"type":"codeBlock","content":[{"type":"text","text":"go run ./cmd"}]}
	]}`
	var node any
	if err := json.Unmarshal([]byte(doc), &node); err != nil {
		t.Fatal(err)
	}
	got := adfToText(node)
	want := "見出し\n一行目\n二行目 @笠松\n項目A\n\n項目B\n\ngo run ./cmd"
	if got != want {
		t.Errorf("adfToText =\n%q\nwant\n%q", got, want)
	}

	// description が ADF のときも FetchIssues 経由でテキストになる
	if txt := descriptionText(json.RawMessage(doc)); txt != want {
		t.Errorf("descriptionText(ADF) = %q", txt)
	}
	if txt := descriptionText(json.RawMessage(`"wiki *text*"`)); txt != "wiki *text*" {
		t.Errorf("descriptionText(string) = %q", txt)
	}
	if txt := descriptionText(nil); txt != "" {
		t.Errorf("descriptionText(nil) = %q", txt)
	}
}

func TestSprintName(t *testing.T) {
	cases := map[string]string{
		`[{"id":1,"name":"Sprint 41"},{"id":2,"name":"Sprint 42"}]`:                                                                    "Sprint 42",
		`["com.atlassian.greenhopper.service.sprint.Sprint@1a2b[id=7,rapidViewId=3,state=ACTIVE,name=スプリント 42,startDate=2026-09-01]"]`: "スプリント 42",
		`{"name":"Solo"}`: "Solo",
		`null`:            "",
		`[]`:              "",
	}
	for in, want := range cases {
		if got := sprintName(json.RawMessage(in)); got != want {
			t.Errorf("sprintName(%s) = %q, want %q", in, got, want)
		}
	}
}

func TestSearch_RequiresJQL(t *testing.T) {
	c := NewWithHTTPClient(serverCfg("http://x"), "pat", nil)
	if _, err := c.Search(context.Background(), " ", 5); err == nil {
		t.Error("empty jql should fail")
	}
}
