package domain_test

import (
	"testing"

	"loginTest/domain"
)

func TestScoreUserSearchByUserName(t *testing.T) {
	cases := []struct {
		name      string
		query     string
		userName  string
		wantType  string
		wantAbove float64
	}{
		{"用户名全等", "磊哥游戏", "磊哥游戏", domain.UserSearchMatchName, 0.99},
		{"用户名包含关键词", "游戏", "磊哥游戏", domain.UserSearchMatchName, 0.9},
		{"搜索词包含用户名", "磊哥游戏", "磊哥", domain.UserSearchMatchName, 0.8},
		{"关键词全部命中用户名", "soft 集市", "Soft 集市小助手", domain.UserSearchMatchName, 0.8},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			score, matchType := domain.ScoreUserSearch(tc.query, &domain.User{Name: tc.userName})
			if matchType != tc.wantType {
				t.Fatalf("matchType = %q, 期望 %q (score=%.3f)", matchType, tc.wantType, score)
			}
			if score < tc.wantAbove {
				t.Fatalf("score = %.3f, 期望 >= %.3f", score, tc.wantAbove)
			}
		})
	}
}

func TestScoreUserSearchByIntro(t *testing.T) {
	user := &domain.User{Name: "小明", Intro: "计科大三，保研经验分享 / 前端开发"}
	score, matchType := domain.ScoreUserSearch("保研经验", user)
	if matchType != domain.UserSearchMatchIntro {
		t.Fatalf("matchType = %q, 期望 %q (score=%.3f)", matchType, domain.UserSearchMatchIntro, score)
	}
	if score < 0.7 {
		t.Fatalf("score = %.3f, 期望 >= 0.7", score)
	}
}

func TestScoreUserSearchSemanticThreshold(t *testing.T) {
	user := &domain.User{Name: "小明", Intro: "计科大三，保研经验分享 / 前端开发"}
	score, matchType := domain.ScoreUserSearch("想找保研的学长", user)
	if matchType != domain.UserSearchMatchSemantic {
		t.Fatalf("matchType = %q, 期望 %q (score=%.3f)", matchType, domain.UserSearchMatchSemantic, score)
	}
	if score < domain.UserSearchRelevanceThreshold {
		t.Fatalf("score = %.3f, 期望 >= 阈值 %.3f", score, domain.UserSearchRelevanceThreshold)
	}
}

func TestScoreUserSearchRejectsUnrelated(t *testing.T) {
	user := &domain.User{Name: "小明", Intro: "计科大三，保研经验分享 / 前端开发"}
	score, matchType := domain.ScoreUserSearch("篮球比赛裁判", user)
	if matchType != "" {
		t.Fatalf("无关关键词不应命中，matchType = %q (score=%.3f)", matchType, score)
	}
}

func TestScoreUserSearchIgnoresIntentOnlyQuery(t *testing.T) {
	user := &domain.User{Name: "小明", Intro: "计科大三，保研经验分享"}
	if score, matchType := domain.ScoreUserSearch("想找的", user); matchType != "" {
		t.Fatalf("纯意图词不应命中，matchType = %q (score=%.3f)", matchType, score)
	}
}

func TestEscapeUserSearchLike(t *testing.T) {
	got := domain.EscapeUserSearchLike(`50%_off!test`)
	want := `50!%!_off!!test`
	if got != want {
		t.Fatalf("EscapeUserSearchLike = %q, 期望 %q", got, want)
	}
}
