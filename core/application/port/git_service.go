package port

// GitInfo はディレクトリの git リポジトリ状態のスナップショット。
type GitInfo struct {
	IsRepo bool
	Branch string
	Dirty  bool
}

// GitService は git リポジトリの参照・取得を行うポート。
type GitService interface {
	// Info はディレクトリの git 状態を返す。リポジトリでない場合は
	// IsRepo=false を返し、エラーにはしない。
	Info(dir string) (GitInfo, error)

	// RemoteURL は origin リモートの URL を返す。リモートが無い場合はエラー。
	RemoteURL(dir string) (string, error)

	// Clone は url を dest に clone し、clone 先の絶対パスを返す。
	Clone(url, dest string) (string, error)
}
