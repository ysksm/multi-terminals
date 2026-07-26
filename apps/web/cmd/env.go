package main

import (
	"bufio"
	"log"
	"os"
	"strings"
)

// loadDotEnv reads a dotenv-style file and returns its KEY=VALUE pairs.
// A missing file is not an error. Supported syntax: comments (#), blank
// lines, an optional "export " prefix, and single/double quoted values.
// Malformed lines are ignored with a warning.
func loadDotEnv(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	vars := make(map[string]string)
	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			log.Printf("multi-terminals: %s:%d: ignoring malformed line", path, lineNo)
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}
		vars[key] = value
	}
	if err := scanner.Err(); err != nil {
		return vars, err
	}
	return vars, nil
}

// resolveAddr builds the listen address from HOST and PORT, preferring
// process environment variables over dotenv values over the defaults
// (all interfaces, port 8080).
func resolveAddr(getenv func(string) string, dotenv map[string]string) string {
	lookup := func(key, fallback string) string {
		if v := getenv(key); v != "" {
			return v
		}
		if v := dotenv[key]; v != "" {
			return v
		}
		return fallback
	}
	return lookup("HOST", "") + ":" + lookup("PORT", "8080")
}
