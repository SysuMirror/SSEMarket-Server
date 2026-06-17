package util

import (
	"regexp"
	"strings"
)

// mentionTokenRE 匹配 @昵称，昵称后须为空格或行末（全量相等匹配，由调用方查库校验）。
var mentionTokenRE = regexp.MustCompile(`@([^\s@]+)(?:\s|$)`)

// ParseMentionTokens 从评论正文提取 @ 提及的昵称 token（去重，保持出现顺序）。
func ParseMentionTokens(content string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, m := range mentionTokenRE.FindAllStringSubmatch(content, -1) {
		if len(m) < 2 {
			continue
		}
		name := strings.TrimSpace(m[1])
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}
