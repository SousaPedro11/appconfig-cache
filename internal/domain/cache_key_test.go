package domain

import "testing"

func TestNewCacheKeyDoesNotIncludeProfile(t *testing.T) {
	key := NewCacheKey(ApplicationID("my-app"), EnvironmentID("prd"))
	if got, want := key.String(), "appconfig:my-app:prd"; got != want {
		t.Fatalf("unexpected cache key: got=%q want=%q", got, want)
	}
}
