package vtscreen

import (
	"strings"
	"testing"
)

func TestScreenRendersOverwritesAndOSC(t *testing.T) {
	s := New(40, 6)
	s.Write([]byte("\x1b]0;⠋ Claude Code\x07"))
	s.Write([]byte("\x1b]9;4;1;50\x07"))
	s.Write([]byte("line1\r\nline2\r\n\x1b[31mred\x1b[0m text\r\n"))
	s.Write([]byte("\x1b[2;1Hoverwritten\x1b[K"))

	if got := s.Title(); got != "⠋ Claude Code" {
		t.Errorf("Title = %q", got)
	}
	if got := s.Progress(); got != "4;1;50" {
		t.Errorf("Progress = %q", got)
	}
	lines := strings.Split(s.Text(), "\n")
	if len(lines) < 3 || lines[0] != "line1" || lines[1] != "overwritten" || lines[2] != "red text" {
		t.Errorf("Text lines = %q", lines)
	}
}

func TestScreenResizeAndZeroSize(t *testing.T) {
	s := New(0, 0)
	s.Write([]byte("hello"))
	s.Resize(0, 0) // 無視される
	s.Resize(20, 4)
	if !strings.HasPrefix(s.Text(), "hello") {
		t.Errorf("Text after resize = %q", s.Text())
	}
}
