package port

// DirectoryOpener はホスト OS 上のアプリケーションでディレクトリを開くポート。
// 実装はプラットフォーム依存（macOS: Finder/open、Windows: Explorer、Linux: xdg-open 等）。
type DirectoryOpener interface {
	// RevealInFileManager は OS のファイルマネージャ（Finder 等）でディレクトリを開く。
	RevealInFileManager(dir string) error

	// OpenInEditor はコードエディタ（VS Code）でディレクトリを開く。
	OpenInEditor(dir string) error

	// OpenURL は URL を既定のブラウザで開く。
	OpenURL(url string) error
}
