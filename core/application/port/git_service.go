package port

// GitInfo はディレクトリの git リポジトリ状態のスナップショット。
type GitInfo struct {
	IsRepo bool
	Branch string
	Dirty  bool
}

// BranchInfo は 1 ブランチの情報。
type BranchInfo struct {
	Name      string // ローカル名。リモートのみの場合も origin/ プレフィックスを除いた名前
	IsCurrent bool
	IsRemote  bool // リモートにのみ存在(ローカル未チェックアウト)
}

// GitService は git リポジトリの参照・取得を行うポート。
type GitService interface {
	// Info はディレクトリの git 状態を返す。リポジトリでない場合は
	// IsRepo=false を返し、エラーにはしない。
	Info(dir string) (GitInfo, error)

	// RemoteURL は origin リモートの URL を返す。リモートが無い場合はエラー。
	RemoteURL(dir string) (string, error)

	// Clone は url を dest に clone し、clone 先の絶対パスを返す。
	// dest が既に git リポジトリの場合は clone せず、そのパスを返す。
	Clone(url, dest string) (string, error)

	// Branches はローカル + リモート追跡ブランチを返す。ローカルと同名の
	// リモートブランチはローカル優先で重複除去する。origin/HEAD は除外。
	Branches(dir string) ([]BranchInfo, error)

	// Checkout は branch に切り替える(git switch 相当。リモートのみの
	// ブランチは追跡ブランチを自動作成して切り替える)。
	Checkout(dir, branch string) error

	// Pull は現在ブランチを pull する。認証が必要な場合は即エラー。
	Pull(dir string) error

	// Push は現在ブランチを push する。upstream 未設定は git のエラーを返す。
	Push(dir string) error

	// Fetch は全リモートを fetch --prune する。
	Fetch(dir string) error
}
