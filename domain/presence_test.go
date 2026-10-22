package domain_test

import (
	"testing"

	"loginTest/domain"
)

func TestNormalizePresenceScene(t *testing.T) {
	cases := []struct {
		scene string
		want  string
		ok    bool
	}{
		{"", "", true},
		{domain.PresenceSceneHome, domain.PresenceSceneHome, true},
		{domain.PresenceScenePost, domain.PresenceScenePost, true},
		{domain.PresenceSceneNovel, domain.PresenceSceneNovel, true},
		{domain.PresenceSceneMusic, domain.PresenceSceneMusic, true},
		{domain.PresenceSceneResume, domain.PresenceSceneResume, true},
		{"admin", "", false},
		{"POST", "", false},
	}
	for _, tc := range cases {
		got, ok := domain.NormalizePresenceScene(tc.scene)
		if ok != tc.ok || got != tc.want {
			t.Errorf("NormalizePresenceScene(%q) = (%q, %v), 期望 (%q, %v)", tc.scene, got, ok, tc.want, tc.ok)
		}
	}
}
