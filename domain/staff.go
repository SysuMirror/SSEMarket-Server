package domain

import (
	"fmt"
	"unicode/utf8"
)

// StaffAlias 职位账号领域实体。StaffUserID 只在服务端流转，绝不出领域层对前端。
type StaffAlias struct {
	ID          int
	Alias       string
	Title       string
	StaffUserID int
	AliasUserID int // 职位小号（对外身份）的 userID；0 表示尚未派生
}

// StaffAliasPublic 对外（前端）可见字段——没有 StaffUserID，保证开发者匿名。
type StaffAliasPublic struct {
	AliasID int    `json:"aliasID"`
	Alias   string `json:"alias"`
	Title   string `json:"title"`
	UserID  int    `json:"userID"` // 职位小号 userID：@渲染成链接、点进主页都靠它
}

// 职位小号口径：
//   - Score 取 5000 = 满级（前端 level.ts：score>=5000 为 Lv8，且 customTitle 生效），
//     即「等级自动是满级」；
//   - CustomTitle = title，即「对应称号也是其职位」；
//   - 邮箱固定 staff-<aliasID>@staff.internal（不可收信、不可找回密码、无人可登录）。
const (
	StaffAliasScore         = 5000
	StaffAliasEmailDomain   = "staff.internal"
	StaffAliasMaxRunes      = 50 // 与 users.name / staff_aliases.alias 的 varchar(50) 对齐
	StaffAliasTitleMaxRunes = 32 // 与 users.custom_title varchar(32) 对齐
)

// StaffAliasShellEmail 职位小号的固定邮箱（users.email 唯一索引兜底，重复派生会被挡下）。
func StaffAliasShellEmail(aliasID int) string {
	return fmt.Sprintf("staff-%d@%s", aliasID, StaffAliasEmailDomain)
}

// ValidateStaffAliasRow 校验管理员手工插入的职位行（启动同步与懒同步共用一套口径）。
func ValidateStaffAliasRow(alias, title string, staffUserID int) error {
	if staffUserID <= 0 {
		return fmt.Errorf("staff_user_id 必须为真实开发者的正整数 userID")
	}
	if n := utf8.RuneCountInString(alias); n == 0 || n > StaffAliasMaxRunes {
		return fmt.Errorf("alias 需为 1~%d 个字符", StaffAliasMaxRunes)
	}
	if n := utf8.RuneCountInString(title); n == 0 || n > StaffAliasTitleMaxRunes {
		return fmt.Errorf("title 需为 1~%d 个字符", StaffAliasTitleMaxRunes)
	}
	return nil
}

// StaffAliasesMentioned 判断 @ 解析出的昵称里命中了哪些职位名（精确相等，与 notifyMentions 同口径）。
func StaffAliasesMentioned(tokens []string, aliases []*StaffAlias) []*StaffAlias {
	if len(tokens) == 0 || len(aliases) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(tokens))
	for _, t := range tokens {
		set[t] = struct{}{}
	}
	var out []*StaffAlias
	for _, a := range aliases {
		if a == nil || a.AliasUserID <= 0 {
			continue
		}
		if _, ok := set[a.Alias]; ok {
			out = append(out, a)
		}
	}
	return out
}
