package util

import "testing"

func TestIsQueryablePhone(t *testing.T) {
	tests := []struct {
		phone string
		want  bool
	}{
		{"13800138000", true},
		{" 13800138000 ", true},
		{"", false},
		{"0", false},
		{"deleted_1150", false},
		{"legacy_1150", false},
	}
	for _, tt := range tests {
		if got := IsQueryablePhone(tt.phone); got != tt.want {
			t.Errorf("IsQueryablePhone(%q) = %v, want %v", tt.phone, got, tt.want)
		}
	}
}
