package util

import (
	"reflect"
	"testing"
)

func TestParseMentionTokens(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"普通评论", nil},
		{"@秋 你好", []string{"秋"}},
		{"你好 @秋同学", []string{"秋同学"}},
		{"@张三 @李四 来看看", []string{"张三", "李四"}},
		{"重复 @秋 @秋 两次", []string{"秋"}},
	}
	for _, tc := range tests {
		got := ParseMentionTokens(tc.in)
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("ParseMentionTokens(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
