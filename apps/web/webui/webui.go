// Package webui serves the production frontend (Svelte/xterm build) that is
// embedded into the server binary at build time. Run `scripts/dev.sh build`
// to populate dist/ and produce a self-contained binary.
package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// Handler returns an http.Handler serving the embedded SPA with a fallback to
// index.html (so deep links work). If the frontend has not been built into the
// binary (only the placeholder is present), it serves a short instructional
// page instead, so hitting the server root is never a bare 404.
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return notBuiltHandler()
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return notBuiltHandler()
	}

	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" {
			p = "index.html"
		}
		// Serve the real file if it exists; otherwise fall back to index.html
		// for client-side routing.
		if _, err := fs.Stat(sub, p); err != nil {
			http.ServeFileFS(w, r, sub, "index.html")
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

// IsBuilt reports whether a real frontend build is embedded.
func IsBuilt() bool {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return false
	}
	_, err = fs.Stat(sub, "index.html")
	return err == nil
}

func notBuiltHandler() http.Handler {
	const page = `<!doctype html><html lang="ja"><meta charset="utf-8">
<title>multi-terminals</title>
<body style="font-family:system-ui;background:#14161a;color:#d6dae0;padding:40px;line-height:1.7">
<h1>multi-terminals API server</h1>
<p>このバイナリにはフロントエンド(UI)が組み込まれていません。</p>
<p><b>本番ビルド:</b> <code>./scripts/dev.sh build</code> で UI 組み込みバイナリ <code>bin/multi-terminals</code> を生成できます。</p>
<p><b>開発時:</b> 別ターミナルで <code>./scripts/dev.sh frontend</code> を起動し
<a style="color:#3b82f6" href="http://localhost:5173">http://localhost:5173</a> を開いてください。</p>
<p>API は <code>/api/...</code> で動作しています。</p>
</body></html>`
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(page))
	})
}
