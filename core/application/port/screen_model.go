package port

// ScreenModel はターミナル出力を解釈して「いま画面に見えているテキスト」を
// 保持する仮想端末。エージェント状態の判定(agentstatus)は生のバイト列ではなく
// この描画済みテキストと OSC(タイトル/進捗)を入力にする。
type ScreenModel interface {
	// Write は PTY からの出力チャンクを解釈して画面を更新する。
	Write(chunk []byte)

	// Resize は画面サイズを変更する。
	Resize(cols, rows uint16)

	// Text は現在の画面(プライマリ/代替スクリーンのうち表示中のもの)の
	// テキストを行ごとに改行で連結して返す。行末の空白は除去する。
	Text() string

	// Title は直近の OSC 0/2 タイトルを返す。未設定または空でクリアされて
	// いれば ""。
	Title() string

	// Progress は直近の OSC 9 ペイロード("9;" を除いた部分)を返す。
	// 未設定なら ""。
	Progress() string
}

// ScreenModelFactory は指定サイズの ScreenModel を生成する。
type ScreenModelFactory func(cols, rows uint16) ScreenModel
