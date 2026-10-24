package domain

import "testing"

func TestFilterBadgesByProfileSelection(t *testing.T) {
	badges := []AwardedBadge{
		{Badge: Badge{ID: 1, Code: "anniversary-1"}},
		{Badge: Badge{ID: 2, Code: "anniversary-2"}},
		{Badge: Badge{ID: 3, Code: "programmers-day-2026"}},
	}

	// nil = 默认全部展示。
	if got := FilterBadgesByProfileSelection(badges, nil); len(got) != 3 {
		t.Fatalf("nil selection should keep all, got %d", len(got))
	}

	// 空切片 = 明确不展示。
	if got := FilterBadgesByProfileSelection(badges, []int{}); len(got) != 0 {
		t.Fatalf("empty selection should hide all, got %d", len(got))
	}

	// 只保留选中的，保持原列表顺序。
	got := FilterBadgesByProfileSelection(badges, []int{3, 1})
	if len(got) != 2 || got[0].ID != 1 || got[1].ID != 3 {
		t.Fatalf("unexpected filtered badges: %+v", got)
	}

	// 已失效/未拥有的 ID 直接忽略。
	if got := FilterBadgesByProfileSelection(badges, []int{99}); len(got) != 0 {
		t.Fatalf("unknown id should be ignored, got %d", len(got))
	}
}
