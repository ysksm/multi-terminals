package domain

// LayoutPreset はワークスペースの画面分割プリセットを表す Value Object。
type LayoutPreset string

const (
	LayoutSingle          LayoutPreset = "single"
	LayoutSplitVertical   LayoutPreset = "split_vertical"
	LayoutSplitHorizontal LayoutPreset = "split_horizontal"
	LayoutGrid2x2         LayoutPreset = "grid_2x2"
)

// Capacity はこのプリセットが収容できる pane の最大数を返す。
// 未知のプリセットは 0 を返す。
func (l LayoutPreset) Capacity() int {
	switch l {
	case LayoutSingle:
		return 1
	case LayoutSplitVertical, LayoutSplitHorizontal:
		return 2
	case LayoutGrid2x2:
		return 4
	default:
		return 0
	}
}

// IsValid は既知のプリセットかどうかを返す。
func (l LayoutPreset) IsValid() bool {
	return l.Capacity() > 0
}
