package domain

import (
	"strings"
	"testing"
)

func TestStaffAliasShellEmail(t *testing.T) {
	if got := StaffAliasShellEmail(7); got != "staff-7@staff.internal" {
		t.Fatalf("StaffAliasShellEmail(7) = %q, want %q", got, "staff-7@staff.internal")
	}
}

func TestIsStaffShellEmail(t *testing.T) {
	cases := []struct {
		name  string
		email string
		want  bool
	}{
		{"职位小号邮箱", "staff-7@staff.internal", true},
		{"大小写混写", "Staff-7@STAFF.Internal", true},
		{"前后空格", " staff-7@staff.internal ", true},
		{"真人邮箱", "dev@example.com", false},
		{"昵称里带 staff 的真人邮箱", "stafflover@example.com", false},
		{"同名域但不是小号前缀", "someone@staff.internal.example.com", false},
		{"空邮箱", "", false},
	}
	for _, tc := range cases {
		if got := IsStaffShellEmail(tc.email); got != tc.want {
			t.Fatalf("%s: IsStaffShellEmail(%q) = %v, want %v", tc.name, tc.email, got, tc.want)
		}
	}
}

func TestValidateStaffAliasRow(t *testing.T) {
	cases := []struct {
		name    string
		alias   string
		title   string
		staffID int
		wantErr bool
	}{
		{"正常", "iwiki开发人员", "iwiki开发人员", 42, false},
		{"缺 userID", "iwiki开发人员", "iwiki开发人员", 0, true},
		{"缺 alias", "", "开发人员", 42, true},
		{"alias 超长", strings.Repeat("长", StaffAliasMaxRunes+1), "开发人员", 42, true},
		{"title 超长", "iwiki", strings.Repeat("长", StaffAliasTitleMaxRunes+1), 42, true},
	}
	for _, tc := range cases {
		err := ValidateStaffAliasRow(tc.alias, tc.title, tc.staffID)
		if (err != nil) != tc.wantErr {
			t.Fatalf("%s: ValidateStaffAliasRow(%q,%q,%d) err=%v, wantErr=%v",
				tc.name, tc.alias, tc.title, tc.staffID, err, tc.wantErr)
		}
	}
}

func TestStaffAliasesMentioned(t *testing.T) {
	aliases := []*StaffAlias{
		{ID: 1, Alias: "iwiki开发人员", Title: "iwiki", StaffUserID: 42, AliasUserID: 901},
		{ID: 2, Alias: "小汐运营", Title: "运营", StaffUserID: 43, AliasUserID: 0}, // 尚未派生小号
	}
	tokens := []string{"张三", "iwiki开发人员"}
	got := StaffAliasesMentioned(tokens, aliases)
	if len(got) != 1 || got[0].AliasUserID != 901 {
		t.Fatalf("StaffAliasesMentioned = %+v, want only the provisioned alias", got)
	}
	if got2 := StaffAliasesMentioned(nil, aliases); got2 != nil {
		t.Fatalf("empty tokens should match nothing, got %+v", got2)
	}
}
