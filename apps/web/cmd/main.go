package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ysksm/multi-terminals/apps/web"
	"github.com/ysksm/multi-terminals/apps/web/webui"
	"github.com/ysksm/multi-terminals/core/infrastructure/jsonstore"
)

func main() {
	// Determine base directory. The MULTI_TERMINALS_DIR env var overrides the default.
	baseDir := os.Getenv("MULTI_TERMINALS_DIR")
	if baseDir == "" {
		var err error
		baseDir, err = jsonstore.DefaultBaseDir()
		if err != nil {
			log.Fatalf("multi-terminals: get default base dir: %v", err)
		}
	}

	deps, err := web.BuildDeps(baseDir)
	if err != nil {
		log.Fatalf("multi-terminals: build deps: %v", err)
	}

	mux := web.NewMux(deps)

	// Serve the embedded frontend (if built into the binary) for all non-/api
	// routes. The "/api/..." patterns registered by NewMux are more specific and
	// take precedence over this root catch-all.
	mux.Handle("/", webui.Handler())

	addr := ":" + portFromEnv("8080")
	ui := "embedded UI"
	if !webui.IsBuilt() {
		ui = "API only (frontend not embedded — run scripts/dev.sh build, or use the Vite dev server)"
	}
	fmt.Printf("multi-terminals: listening on %s (baseDir=%s, %s)\n", addr, baseDir, ui)

	srv := &http.Server{Addr: addr, Handler: mux}

	// A6.2: graceful shutdown on SIGINT / SIGTERM.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		log.Println("multi-terminals: shutting down …")

		// Close all running PTY sessions so child processes are not orphaned.
		deps.Registry.CloseAll()

		// Give in-flight HTTP requests 10 seconds to finish.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("multi-terminals: server shutdown error: %v", err)
		}
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("multi-terminals: server error: %v", err)
	}
}

// portFromEnv returns the PORT environment variable value or the given default.
func portFromEnv(defaultPort string) string {
	if p := os.Getenv("PORT"); p != "" {
		return p
	}
	return defaultPort
}
