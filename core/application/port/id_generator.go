package port

// IDGenerator は新しい一意識別子を生成するポート。
// 実装は infrastructure 層が提供する（UUID 生成等）。
type IDGenerator interface {
	NewID() string
}
