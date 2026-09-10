package setupexec

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func TestCopyTree(t *testing.T) {
	e := New()
	src := t.TempDir()
	write(t, filepath.Join(src, "a.txt"), "A", 0o644)
	write(t, filepath.Join(src, "sub", "deep", "b.txt"), "B", 0o644)
	write(t, filepath.Join(src, "node_modules", "x", "c.txt"), "C", 0o644)
	write(t, filepath.Join(src, "bin", "run.sh"), "#!/bin/sh\n", 0o755)
	if runtime.GOOS != "windows" {
		if err := os.Symlink("a.txt", filepath.Join(src, "link")); err != nil {
			t.Fatal(err)
		}
	}

	dst := filepath.Join(t.TempDir(), "dst")
	var log bytes.Buffer
	if err := e.CopyTree(context.Background(), src, dst, []string{"node_modules"}, &log); err != nil {
		t.Fatalf("CopyTree: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "sub", "deep", "b.txt")); string(b) != "B" {
		t.Errorf("nested file not copied: %q", b)
	}
	if _, err := os.Stat(filepath.Join(dst, "node_modules")); !os.IsNotExist(err) {
		t.Errorf("excluded dir should not be copied")
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(dst, "bin", "run.sh"))
		if err != nil || info.Mode().Perm()&0o100 == 0 {
			t.Errorf("exec bit not preserved: %v %v", info, err)
		}
		if l, err := os.Readlink(filepath.Join(dst, "link")); err != nil || l != "a.txt" {
			t.Errorf("symlink not preserved: %q %v", l, err)
		}
	}
	if !strings.Contains(log.String(), "copied") {
		t.Errorf("summary missing in log: %q", log.String())
	}

	// 空でない dst は拒否
	if err := e.CopyTree(context.Background(), src, dst, nil, nil); err == nil {
		t.Errorf("expected error for non-empty dst")
	}
}

func TestCopyTree_Cancel(t *testing.T) {
	e := New()
	src := t.TempDir()
	for i := 0; i < 50; i++ {
		write(t, filepath.Join(src, "f", string(rune('a'+i%26))+".txt"), "x", 0o644)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := e.CopyTree(ctx, src, filepath.Join(t.TempDir(), "d"), nil, nil)
	if err == nil || !strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Errorf("expected cancel error, got %v", err)
	}
}

func TestRunCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh-specific")
	}
	e := New()
	dir := t.TempDir()
	var log bytes.Buffer
	err := e.RunCommand(context.Background(), dir, "echo out; echo err 1>&2; echo $TASK_KEY; pwd", []string{"TASK_KEY=PROJ-1"}, &log)
	if err != nil {
		t.Fatalf("RunCommand: %v", err)
	}
	out := log.String()
	for _, want := range []string{"out", "err", "PROJ-1"} {
		if !strings.Contains(out, want) {
			t.Errorf("log missing %q: %q", want, out)
		}
	}
	if !strings.Contains(out, filepath.Base(dir)) {
		t.Errorf("pwd should be dir: %q", out)
	}

	err = e.RunCommand(context.Background(), dir, "exit 3", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "3") {
		t.Errorf("expected exit code 3 error, got %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	err = e.RunCommand(ctx, dir, "sleep 5", nil, nil)
	if err == nil {
		t.Errorf("expected timeout error")
	}
	if time.Since(start) > 3*time.Second {
		t.Errorf("timeout not honored: took %v", time.Since(start))
	}
}

func TestRemoveAll(t *testing.T) {
	e := New()
	if err := e.RemoveAll("/"); err == nil {
		t.Errorf("expected refusal for /")
	}
	if err := e.RemoveAll(""); err == nil {
		t.Errorf("expected refusal for empty")
	}
	home, _ := os.UserHomeDir()
	if err := e.RemoveAll(home); err == nil {
		t.Errorf("expected refusal for home")
	}
	dir := filepath.Join(t.TempDir(), "work", "PROJ-1")
	write(t, filepath.Join(dir, "x"), "x", 0o644)
	if err := e.RemoveAll(dir); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("dir should be removed")
	}
	if err := e.RemoveAll(dir); err != nil {
		t.Errorf("missing dir should be no-op: %v", err)
	}
}

func TestDirSize(t *testing.T) {
	e := New()
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "12345", 0o644)
	write(t, filepath.Join(dir, "s", "b"), "123", 0o644)
	n, err := e.DirSize(dir)
	if err != nil || n != 8 {
		t.Errorf("DirSize = %d, %v; want 8", n, err)
	}
}

func TestExpandPath(t *testing.T) {
	e := New()
	home, _ := os.UserHomeDir()
	got, err := e.ExpandPath("~/work/X")
	if err != nil || got != filepath.Join(home, "work", "X") {
		t.Errorf("ExpandPath = %q, %v", got, err)
	}
	if _, err := e.ExpandPath(""); err == nil {
		t.Errorf("expected error for empty")
	}
	if got, _ := e.ExpandPath("/tmp/../tmp/x"); got != filepath.Clean("/tmp/x") {
		t.Errorf("abs/clean = %q", got)
	}
}
