package sysopen

import (
	"reflect"
	"testing"
)

func TestFileManagerArgs(t *testing.T) {
	tests := []struct {
		goos string
		want []string
	}{
		{"darwin", []string{"open", "/tmp/p"}},
		{"windows", []string{"explorer", "/tmp/p"}},
		{"linux", []string{"xdg-open", "/tmp/p"}},
	}
	for _, tt := range tests {
		if got := fileManagerArgs(tt.goos, "/tmp/p"); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("fileManagerArgs(%q) = %v, want %v", tt.goos, got, tt.want)
		}
	}
}

func TestURLArgs(t *testing.T) {
	tests := []struct {
		goos string
		want []string
	}{
		{"darwin", []string{"open", "https://github.com/u/r"}},
		{"windows", []string{"rundll32", "url.dll,FileProtocolHandler", "https://github.com/u/r"}},
		{"linux", []string{"xdg-open", "https://github.com/u/r"}},
	}
	for _, tt := range tests {
		if got := urlArgs(tt.goos, "https://github.com/u/r"); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("urlArgs(%q) = %v, want %v", tt.goos, got, tt.want)
		}
	}
}

func TestEditorArgs(t *testing.T) {
	tests := []struct {
		goos string
		want []string
	}{
		// GUI 起動のアプリは PATH に code が無いことがあるため、macOS は open -a を使う。
		{"darwin", []string{"open", "-a", "Visual Studio Code", "/tmp/p"}},
		{"windows", []string{"code", "/tmp/p"}},
		{"linux", []string{"code", "/tmp/p"}},
	}
	for _, tt := range tests {
		if got := editorArgs(tt.goos, "/tmp/p"); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("editorArgs(%q) = %v, want %v", tt.goos, got, tt.want)
		}
	}
}
