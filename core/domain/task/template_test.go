package task

import (
	"strings"
	"testing"
)

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Jira 連携の設定画面":           "jira",
		"ログ出力の JSON 化":           "json",
		"決済画面のバリデーション修正":         "",
		"Add JSON logging (v2)!": "add-json-logging-v2",
		"  --Hello--World--  ":   "hello-world",
		"UPPER lower 123":        "upper-lower-123",
		"":                       "",
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
	long := Slugify(strings.Repeat("abcde ", 20))
	if len(long) > SlugMaxLen {
		t.Errorf("slug too long: %d", len(long))
	}
	if strings.HasSuffix(long, "-") {
		t.Errorf("truncated slug must not end with dash: %q", long)
	}
	// 30 文字ちょうどでダッシュの位置に切れるケース
	if got := Slugify("aaaaaaaaaaaaaaaaaaaaaaaaaaaaa bbbbbb"); got != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Errorf("trim dash at cap: %q", got)
	}
}

func TestExpandPattern(t *testing.T) {
	vars := map[string]string{"KEY": "PROJ-1", "slug": ""}
	if got := ExpandPattern("feature/{KEY}-{slug}", vars); got != "feature/PROJ-1" {
		t.Errorf("empty slug: %q", got)
	}
	if got := ExpandPattern("feature/{KEY}_{slug}", vars); got != "feature/PROJ-1" {
		t.Errorf("empty slug underscore: %q", got)
	}
	if got := ExpandPattern("{slug}/{KEY}", vars); got != "/PROJ-1" {
		t.Errorf("leading empty: %q", got)
	}
	vars["slug"] = "jira"
	if got := ExpandPattern("feature/{KEY}-{slug}", vars); got != "feature/PROJ-1-jira" {
		t.Errorf("with slug: %q", got)
	}
	if got := ExpandPattern("~/work/{KEY}/{unknown}", vars); got != "~/work/PROJ-1/{unknown}" {
		t.Errorf("unknown var kept: %q", got)
	}
}

func sampleTemplate() *Template {
	return &Template{
		ID: "tpl1", Name: "web", WorkDirPattern: "~/work/{KEY}", BranchPattern: "feature/{KEY}-{slug}", Layout: "grid_2x2",
		Repos: []Repo{
			{Name: "frontend", Source: RepoSource{Kind: SourceBaseCopy, BaseCloneID: "b1"}, SetupCommands: []string{"npm ci"}},
			{Name: "backend", Source: RepoSource{Kind: SourceClone, URL: "git@x:be.git"}},
		},
		Panes: []PaneSpec{{Slot: 0, RepoName: "frontend", Commands: []Command{{Command: "npm run dev", AutoRun: true}}}, {Slot: 1, RepoName: "backend"}},
	}
}

func TestTemplate_Expand(t *testing.T) {
	tpl := sampleTemplate()
	env := tpl.Expand("PROJ-1301", "Jira 連携の設定画面")
	if env.TemplateID != "tpl1" || env.WorkDir != "~/work/PROJ-1301" || env.Branch != "feature/PROJ-1301-jira" || env.Layout != "grid_2x2" {
		t.Errorf("expand: %+v", env)
	}
	if len(env.Repos) != 2 || len(env.Panes) != 2 {
		t.Fatalf("repos/panes not copied: %+v", env)
	}
	// 防御的コピー: 展開結果を書き換えてもテンプレートは変わらない
	env.Repos[0].SetupCommands[0] = "changed"
	env.Panes[0].Commands[0].Command = "changed"
	if tpl.Repos[0].SetupCommands[0] != "npm ci" || tpl.Panes[0].Commands[0].Command != "npm run dev" {
		t.Error("Expand must copy slices")
	}
	if err := env.Validate(); err != nil {
		t.Errorf("expanded env invalid: %v", err)
	}
	env2 := tpl.Expand("PROJ-2", "日本語のみ")
	if env2.Branch != "feature/PROJ-2" {
		t.Errorf("japanese-only slug: %q", env2.Branch)
	}
}

func TestTemplate_Validate(t *testing.T) {
	if err := sampleTemplate().Validate(); err != nil {
		t.Fatalf("sample should be valid: %v", err)
	}
	mut := func(f func(*Template)) *Template { tp := sampleTemplate(); f(tp); return tp }
	cases := map[string]*Template{
		"empty id":         mut(func(tp *Template) { tp.ID = "" }),
		"empty name":       mut(func(tp *Template) { tp.Name = " " }),
		"empty workdir":    mut(func(tp *Template) { tp.WorkDirPattern = "" }),
		"workdir no KEY":   mut(func(tp *Template) { tp.WorkDirPattern = "~/work/fixed" }),
		"empty branch":     mut(func(tp *Template) { tp.BranchPattern = "" }),
		"empty layout":     mut(func(tp *Template) { tp.Layout = "" }),
		"bad repo name":    mut(func(tp *Template) { tp.Repos[0].Name = "../x" }),
		"dup slot":         mut(func(tp *Template) { tp.Panes[1].Slot = 0 }),
		"unknown repo ref": mut(func(tp *Template) { tp.Panes[1].RepoName = "nope" }),
	}
	for name, tp := range cases {
		if err := tp.Validate(); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestDefaultTemplate(t *testing.T) {
	tp := DefaultTemplate("d1", t0)
	if err := tp.Validate(); err != nil {
		t.Fatalf("default template invalid: %v", err)
	}
	if !tp.IsDefault || tp.Layout != "single" || len(tp.Panes) != 1 {
		t.Errorf("default template: %+v", tp)
	}
	env := tp.Expand("PROJ-9", "x")
	if env.WorkDir != "~/work/PROJ-9" || env.Branch != "feature/PROJ-9-x" {
		t.Errorf("default expand: %+v", env)
	}
}

func TestJiraConfig(t *testing.T) {
	c := JiraConfig{Kind: JiraCloud, BaseURL: "https://x.atlassian.net", Email: "a@b"}
	if err := c.Validate(); err != nil || !c.IsConfigured() {
		t.Errorf("cloud config: %v", err)
	}
	c.Email = ""
	if err := c.Validate(); err == nil || c.IsConfigured() {
		t.Error("cloud without email should be invalid")
	}
	s := JiraConfig{Kind: JiraServer, BaseURL: "https://jira.internal"}
	if err := s.Validate(); err != nil || !s.IsConfigured() {
		t.Errorf("server config: %v", err)
	}
	if err := (JiraConfig{Kind: JiraServer, BaseURL: "jira.internal"}).Validate(); err == nil {
		t.Error("baseUrl without scheme should be invalid")
	}
	if err := (JiraConfig{Kind: "x", BaseURL: "https://a"}).Validate(); err == nil {
		t.Error("bad kind should be invalid")
	}
	if (JiraConfig{}).IsConfigured() {
		t.Error("zero config is not configured")
	}
}

func TestBaseClone_Validate(t *testing.T) {
	b := &BaseClone{ID: "b1", Name: "frontend", URL: "u", Path: "/p"}
	if err := b.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, f := range map[string]func(*BaseClone){
		"id":   func(b *BaseClone) { b.ID = "" },
		"name": func(b *BaseClone) { b.Name = "a/b" },
		"url":  func(b *BaseClone) { b.URL = "" },
		"path": func(b *BaseClone) { b.Path = "" },
	} {
		c := *b
		f(&c)
		if err := c.Validate(); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
	b.MarkFetched("main", 42, t0)
	if b.DefaultBranch != "main" || b.SizeBytes != 42 || !b.LastFetchedAt.Equal(t0) {
		t.Errorf("MarkFetched: %+v", b)
	}
	b.MarkFetched("", 1, t0)
	if b.DefaultBranch != "main" {
		t.Error("empty default branch must keep previous")
	}
}
