package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTempEnv(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp .env: %v", err)
	}
	return path
}

func TestLoadDotEnvBasic(t *testing.T) {
	path := writeTempEnv(t, "HOST=127.0.0.1\nPORT=9000\n")
	got, err := loadDotEnv(path)
	if err != nil {
		t.Fatalf("loadDotEnv: %v", err)
	}
	if got["HOST"] != "127.0.0.1" || got["PORT"] != "9000" {
		t.Errorf("got %v, want HOST=127.0.0.1 PORT=9000", got)
	}
}

func TestLoadDotEnvCommentsAndBlankLines(t *testing.T) {
	path := writeTempEnv(t, "# comment\n\nHOST=0.0.0.0\n   \n# PORT=1\n")
	got, err := loadDotEnv(path)
	if err != nil {
		t.Fatalf("loadDotEnv: %v", err)
	}
	if got["HOST"] != "0.0.0.0" {
		t.Errorf("HOST = %q, want 0.0.0.0", got["HOST"])
	}
	if _, ok := got["PORT"]; ok {
		t.Errorf("commented-out PORT should be ignored, got %v", got)
	}
}

func TestLoadDotEnvExportPrefixAndWhitespace(t *testing.T) {
	path := writeTempEnv(t, "export HOST = 192.168.1.10 \n")
	got, err := loadDotEnv(path)
	if err != nil {
		t.Fatalf("loadDotEnv: %v", err)
	}
	if got["HOST"] != "192.168.1.10" {
		t.Errorf("HOST = %q, want 192.168.1.10", got["HOST"])
	}
}

func TestLoadDotEnvQuotedValues(t *testing.T) {
	path := writeTempEnv(t, "HOST=\"127.0.0.1\"\nPORT='9000'\n")
	got, err := loadDotEnv(path)
	if err != nil {
		t.Fatalf("loadDotEnv: %v", err)
	}
	if got["HOST"] != "127.0.0.1" {
		t.Errorf("HOST = %q, want 127.0.0.1 (double quotes stripped)", got["HOST"])
	}
	if got["PORT"] != "9000" {
		t.Errorf("PORT = %q, want 9000 (single quotes stripped)", got["PORT"])
	}
}

func TestLoadDotEnvIgnoresMalformedLines(t *testing.T) {
	path := writeTempEnv(t, "not a valid line\nHOST=127.0.0.1\n")
	got, err := loadDotEnv(path)
	if err != nil {
		t.Fatalf("loadDotEnv: %v", err)
	}
	if got["HOST"] != "127.0.0.1" {
		t.Errorf("HOST = %q, want 127.0.0.1", got["HOST"])
	}
	if len(got) != 1 {
		t.Errorf("malformed line should be ignored, got %v", got)
	}
}

func TestLoadDotEnvMissingFile(t *testing.T) {
	got, err := loadDotEnv(filepath.Join(t.TempDir(), "no-such-file"))
	if err != nil {
		t.Fatalf("missing file should not be an error, got %v", err)
	}
	if len(got) != 0 {
		t.Errorf("missing file should yield empty map, got %v", got)
	}
}

func fakeEnv(vars map[string]string) func(string) string {
	return func(key string) string { return vars[key] }
}

func TestResolveAddrDefaults(t *testing.T) {
	got := resolveAddr(fakeEnv(nil), nil)
	if got != ":8080" {
		t.Errorf("resolveAddr = %q, want :8080", got)
	}
}

func TestResolveAddrFromDotEnv(t *testing.T) {
	dotenv := map[string]string{"HOST": "127.0.0.1", "PORT": "9000"}
	got := resolveAddr(fakeEnv(nil), dotenv)
	if got != "127.0.0.1:9000" {
		t.Errorf("resolveAddr = %q, want 127.0.0.1:9000", got)
	}
}

func TestResolveAddrEnvOverridesDotEnv(t *testing.T) {
	env := map[string]string{"HOST": "0.0.0.0", "PORT": "8888"}
	dotenv := map[string]string{"HOST": "127.0.0.1", "PORT": "9000"}
	got := resolveAddr(fakeEnv(env), dotenv)
	if got != "0.0.0.0:8888" {
		t.Errorf("resolveAddr = %q, want 0.0.0.0:8888", got)
	}
}

func TestResolveAddrHostOnly(t *testing.T) {
	dotenv := map[string]string{"HOST": "127.0.0.1"}
	got := resolveAddr(fakeEnv(nil), dotenv)
	if got != "127.0.0.1:8080" {
		t.Errorf("resolveAddr = %q, want 127.0.0.1:8080", got)
	}
}
