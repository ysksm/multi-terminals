package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/ysksm/multi-terminals/apps/web"
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

	addr := ":" + portFromEnv("8080")
	fmt.Printf("multi-terminals: listening on %s (baseDir=%s)\n", addr, baseDir)
	if err := http.ListenAndServe(addr, mux); err != nil {
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
