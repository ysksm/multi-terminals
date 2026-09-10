package agentstatus

import (
	"fmt"
	"regexp"
	"strings"
)

// このファイルは herdr(https://github.com/herdrdev/herdr)のマニフェスト式
// 画面検知エンジン(src/detect/manifest.rs)を Go に移植したもの。
// 画面テキストの「領域」に対して contains / regex / line_regex / any / all / not
// のゲートを評価し、一致したルールのうち最も priority の高いものの状態を採用する。

// Gate は 1 つの判定条件。全フィールドが AND で結合される(空のものは無条件成立)。
// contains は小文字化した領域テキストに対する部分一致、regex は領域全体、
// line_regex は「いずれかの行」に対する一致。
type Gate struct {
	Contains  []string
	Regex     []string
	LineRegex []string
	All       []Gate // 全て成立
	Any       []Gate // いずれか成立(空なら無条件成立)
	Not       []Gate // いずれかが成立したら不成立
}

// Rule は 1 つの判定ルール。
type Rule struct {
	ID       string
	State    State
	Priority int
	Region   string // regions.go の領域指定
	Gate     Gate
	// SkipStateUpdate: 一致しても状態を更新しない(トランスクリプト表示などの
	// 「ライブ状態を映していない画面」を検出したとき)。
	SkipStateUpdate bool
	// Visible*: 画面上に「いまその状態である」ことを示すライブ UI が
	// 実際に見えている(スクロールバックの残骸ではない)ことを表す。
	VisibleIdle    bool
	VisibleBlocker bool
	VisibleWorking bool
}

// Manifest は 1 ツール分のルール集合。
type Manifest struct {
	Tool  string
	Rules []Rule
}

// Detection は画面検知の結果。
type Detection struct {
	State           State
	MatchedRule     string // 一致したルール ID("" ならフォールバック)
	SkipStateUpdate bool
	VisibleIdle     bool
	VisibleBlocker  bool
	VisibleWorking  bool
}

// Input は画面検知の入力。
type Input struct {
	Screen      string // 描画済み画面テキスト(改行区切り)
	OSCTitle    string // 直近の OSC 0/2 タイトル
	OSCProgress string // 直近の OSC 9 ペイロード("9;" 除く)
}

type compiledGate struct {
	contains  []string // 小文字化済み
	regex     []*regexp.Regexp
	lineRegex []*regexp.Regexp
	all       []compiledGate
	any       []compiledGate
	not       []compiledGate
}

type compiledRule struct {
	rule Rule
	gate compiledGate
}

// CompiledManifest は正規表現をコンパイル済みの Manifest。
type CompiledManifest struct {
	tool  string
	rules []compiledRule
}

// Compile は Manifest の正規表現をコンパイルし、不正なら error を返す。
func Compile(m Manifest) (*CompiledManifest, error) {
	cm := &CompiledManifest{tool: m.Tool}
	for _, r := range m.Rules {
		if err := validateRegion(r.Region); err != nil {
			return nil, fmt.Errorf("agentstatus: manifest %s rule %s: %w", m.Tool, r.ID, err)
		}
		g, err := compileGate(r.Gate)
		if err != nil {
			return nil, fmt.Errorf("agentstatus: manifest %s rule %s: %w", m.Tool, r.ID, err)
		}
		cm.rules = append(cm.rules, compiledRule{rule: r, gate: g})
	}
	return cm, nil
}

// MustCompile は Compile の panic 版(組み込みマニフェスト用)。
func MustCompile(m Manifest) *CompiledManifest {
	cm, err := Compile(m)
	if err != nil {
		panic(err)
	}
	return cm
}

func compileGate(g Gate) (compiledGate, error) {
	var cg compiledGate
	for _, c := range g.Contains {
		cg.contains = append(cg.contains, strings.ToLower(c))
	}
	for _, p := range g.Regex {
		re, err := regexp.Compile(p)
		if err != nil {
			return cg, fmt.Errorf("regex %q: %w", p, err)
		}
		cg.regex = append(cg.regex, re)
	}
	for _, p := range g.LineRegex {
		re, err := regexp.Compile(p)
		if err != nil {
			return cg, fmt.Errorf("line_regex %q: %w", p, err)
		}
		cg.lineRegex = append(cg.lineRegex, re)
	}
	var err error
	if cg.all, err = compileGates(g.All); err != nil {
		return cg, err
	}
	if cg.any, err = compileGates(g.Any); err != nil {
		return cg, err
	}
	if cg.not, err = compileGates(g.Not); err != nil {
		return cg, err
	}
	return cg, nil
}

func compileGates(gs []Gate) ([]compiledGate, error) {
	out := make([]compiledGate, 0, len(gs))
	for _, g := range gs {
		cg, err := compileGate(g)
		if err != nil {
			return nil, err
		}
		out = append(out, cg)
	}
	return out, nil
}

func (g *compiledGate) matches(text, lower string) bool {
	for _, c := range g.contains {
		if !strings.Contains(lower, c) {
			return false
		}
	}
	for _, re := range g.regex {
		if !re.MatchString(text) {
			return false
		}
	}
	for _, re := range g.lineRegex {
		if !anyLineMatches(text, re) {
			return false
		}
	}
	for i := range g.all {
		if !g.all[i].matches(text, lower) {
			return false
		}
	}
	if len(g.any) > 0 {
		ok := false
		for i := range g.any {
			if g.any[i].matches(text, lower) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	for i := range g.not {
		if g.not[i].matches(text, lower) {
			return false
		}
	}
	return true
}

func anyLineMatches(text string, re *regexp.Regexp) bool {
	for _, line := range strings.Split(text, "\n") {
		if re.MatchString(line) {
			return true
		}
	}
	return false
}

// Detect は in に対してマニフェストを評価する。一致ルールが無ければ
// herdr 同様「既知エージェントが居るなら idle」をフォールバックとする。
func (cm *CompiledManifest) Detect(in Input) Detection {
	var best *compiledRule
	for i := range cm.rules {
		r := &cm.rules[i]
		text := region(in, r.rule.Region)
		if !r.gate.matches(text, strings.ToLower(text)) {
			continue
		}
		if best == nil || r.rule.Priority > best.rule.Priority {
			best = r
		}
	}
	if best == nil {
		return Detection{State: StateIdle}
	}
	r := best.rule
	return Detection{
		State:           r.State,
		MatchedRule:     r.ID,
		SkipStateUpdate: r.SkipStateUpdate,
		VisibleIdle:     r.VisibleIdle && r.State == StateIdle,
		VisibleBlocker:  r.VisibleBlocker && r.State == StateBlocked,
		VisibleWorking:  r.VisibleWorking && r.State == StateWorking,
	}
}
