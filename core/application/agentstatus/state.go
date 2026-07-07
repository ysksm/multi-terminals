package agentstatus

import (
	"strings"
	"time"
)

// State はペイン内エージェントの状態。
type State string

const (
	// StateActive: エージェントが実行中(プロンプト待ちでない)。
	StateActive State = "active"
	// StateWait: 許可プロンプトを表示してユーザー入力を待って停止中。
	StateWait State = "wait"
)

// WaitIdleThreshold: 末尾に許可プロンプトが見えていても、この時間以上
// 新規出力が無い場合にのみ wait と判定する。スクロールバックに過去の
// プロンプトが残っているだけのケース(直後に出力が続いた)を弾くための条件。
const WaitIdleThreshold = 2 * time.Second

// waitPatterns は「許可待ちで停止中」を示す既知のプロンプト文字列。
// 端末のエスケープシーケンス混じりの生バイト列に対する部分一致で判定する。
var waitPatterns = []string{
	"Do you want",   // claude code の許可プロンプト
	"❯ 1. Yes",      // claude code の選択プロンプト
	"Allow command", // codex の承認プロンプト
	"Approve",       // codex
}

// ClassifyState はスクロールバック末尾 tail と最終出力時刻から状態を判定する。
// wait = 末尾に許可プロンプト文字列があり、かつ WaitIdleThreshold 以上 idle。
func ClassifyState(tail []byte, lastOutput, now time.Time) State {
	if now.Sub(lastOutput) < WaitIdleThreshold {
		return StateActive
	}
	s := string(tail)
	for _, p := range waitPatterns {
		if strings.Contains(s, p) {
			return StateWait
		}
	}
	return StateActive
}
