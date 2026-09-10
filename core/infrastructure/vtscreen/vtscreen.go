// Package vtscreen は port.ScreenModel の実装。charmbracelet/x/vt の仮想端末で
// PTY 出力を解釈し、描画済みの画面テキストと OSC タイトル/進捗を保持する。
package vtscreen

import (
	"strings"
	"sync"

	"github.com/charmbracelet/x/vt"

	"github.com/ysksm/multi-terminals/core/application/port"
)

// コンパイル時インターフェース適合確認
var _ port.ScreenModel = (*Screen)(nil)

// ScrollbackLines は仮想端末に保持するスクロールバック行数。判定は画面内の
// テキストだけを使うため大きくする必要はない。
const ScrollbackLines = 200

// Screen は 1 セッション分の仮想端末。全メソッド並行安全。
type Screen struct {
	mu       sync.Mutex
	em       *vt.Emulator
	title    string
	progress string
}

// New は cols×rows の Screen を返す。0 以下は 80×24 に丸める。
func New(cols, rows uint16) *Screen {
	if cols == 0 {
		cols = 80
	}
	if rows == 0 {
		rows = 24
	}
	s := &Screen{em: vt.NewEmulator(int(cols), int(rows))}
	s.em.SetScrollbackSize(ScrollbackLines)
	// Title コールバックは Write の中(mu 保持中)から呼ばれるので、ここでは
	// ロックを取らない。
	s.em.SetCallbacks(vt.Callbacks{Title: func(t string) { s.title = t }})
	// OSC 9 (ConEmu/Windows Terminal 進捗: "9;4;<state>;<pct>")。data は
	// "9;" を含む本文なので、herdr と同じく "9;" を除いた部分を保持する。
	s.em.RegisterOscHandler(9, func(data []byte) bool {
		s.progress = strings.TrimPrefix(string(data), "9;")
		return true
	})
	return s
}

// Factory は port.ScreenModelFactory 適合のコンストラクタ。
func Factory(cols, rows uint16) port.ScreenModel { return New(cols, rows) }

func (s *Screen) Write(chunk []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = s.em.Write(chunk)
}

func (s *Screen) Resize(cols, rows uint16) {
	if cols == 0 || rows == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.em.Resize(int(cols), int(rows))
}

func (s *Screen) Text() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.em.String()
}

func (s *Screen) Title() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.title
}

func (s *Screen) Progress() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.progress
}
