package domain

import "testing"

func TestLayoutPresetCapacity(t *testing.T) {
	cases := []struct {
		preset LayoutPreset
		want   int
	}{
		{LayoutSingle, 1},
		{LayoutSplitVertical, 2},
		{LayoutSplitHorizontal, 2},
		{LayoutGrid2x2, 4},
	}
	for _, c := range cases {
		if got := c.preset.Capacity(); got != c.want {
			t.Errorf("%s.Capacity() = %d, want %d", c.preset, got, c.want)
		}
	}
}

func TestLayoutPresetIsValid(t *testing.T) {
	if !LayoutGrid2x2.IsValid() {
		t.Error("LayoutGrid2x2 should be valid")
	}
	if LayoutPreset("unknown").IsValid() {
		t.Error("unknown preset should be invalid")
	}
}
