package main

import (
	"log"
	"os"

	"github.com/ysksm/multi-terminals/apps/web"
	"github.com/ysksm/multi-terminals/apps/web/webui"
	"github.com/ysksm/multi-terminals/core/infrastructure/jsonstore"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

func main() {
	baseDir := os.Getenv("MULTI_TERMINALS_DIR")
	if baseDir == "" {
		var err error
		baseDir, err = jsonstore.DefaultBaseDir()
		if err != nil {
			log.Fatalf("multi-terminals (wails): default base dir: %v", err)
		}
	}

	deps, err := web.BuildDeps(baseDir)
	if err != nil {
		log.Fatalf("multi-terminals (wails): build deps: %v", err)
	}

	// Reuse the web mux for REST + embedded SPA, served in-process by Wails.
	mux := web.NewMux(deps)
	mux.Handle("/", webui.Handler())

	app := NewApp(deps)

	if err := wails.Run(&options.App{
		Title:  "multi-terminals",
		Width:  1200,
		Height: 800,
		AssetServer: &assetserver.Options{
			Handler: mux,
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind: []interface{}{
			app,
		},
	}); err != nil {
		log.Fatalf("multi-terminals (wails): run: %v", err)
	}
}
