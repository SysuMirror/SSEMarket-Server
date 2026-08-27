package config

import (
	"strconv"
	"strings"

	"github.com/spf13/viper"
)

func init() {
	_ = viper.BindEnv("quest.external_user_ids", "QUEST_EXTERNAL_USER_IDS")
	_ = viper.BindEnv("quest.test_allow_all_post", "QUEST_TEST_ALLOW_ALL_POST")
}

// QuestExternalUserIDs 测试期允许 Quest 发帖的用户 ID 列表。
func QuestExternalUserIDs() []int {
	raw := strings.TrimSpace(viper.GetString("quest.external_user_ids"))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		id, err := strconv.Atoi(p)
		if err == nil && id > 0 {
			out = append(out, id)
		}
	}
	return out
}

// QuestTestAllowAllPost 测试开关：任意登录用户可 Quest 发帖（正式应 false）。
func QuestTestAllowAllPost() bool {
	return viper.GetBool("quest.test_allow_all_post")
}
