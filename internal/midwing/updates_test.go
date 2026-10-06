package midwing

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestNewerRelease(t *testing.T) {
	for _, tc := range []struct {
		latest, current string
		want            bool
	}{
		{"v0.2.0", "0.1.0", true}, {"v0.1.10", "0.1.9", true}, {"v1.0.0", "0.99.99", true},
		{"v0.1.0", "0.1.0", false}, {"v0.1.0", "0.2.0", false}, {"v0.2.0-beta.1", "0.1.0", false},
		{"v01.2.0", "0.1.0", false}, {"v1.0.0", "dev", false}, {"v999999999999999999999999.0.0", "0.1.0", false},
	} {
		if got := newerRelease(tc.latest, tc.current); got != tc.want {
			t.Errorf("%s vs %s: %v", tc.latest, tc.current, got)
		}
	}
}

func TestUpdateCheckPrivacyCachingAndRetry(t *testing.T) {
	calls, fail := 0, false
	checker := UpdateChecker{Repository: "example/midwing", Version: "0.1.0", HTTP: &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != "https://api.github.com/repos/example/midwing/releases/latest" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Fatal("unexpected update request")
		}
		status := 200
		if fail {
			status = 503
		}
		return &http.Response{StatusCode: status, Body: ioBody(`{"tag_name":"v0.2.0","html_url":"https://evil.test"}`)}, nil
	})}}
	first, err := checker.Check(context.Background())
	if err != nil || first.Version != "0.2.0" || first.DownloadURL != "https://github.com/example/midwing/releases/tag/v0.2.0" {
		t.Fatalf("update: %+v %v", first, err)
	}
	if _, err := checker.Check(context.Background()); err != nil || calls != 1 {
		t.Fatal("successful check not cached")
	}
	checker.nextCheck = time.Time{}
	fail = true
	retained, err := checker.Check(context.Background())
	if err == nil || retained != first {
		t.Fatal("failure lost existing notice")
	}
	_, _ = checker.Check(context.Background())
	if calls != 2 {
		t.Fatal("failed check retried immediately")
	}
}

func TestUpdateCheckIgnoresUnavailableAndUnstableReleases(t *testing.T) {
	for _, tc := range []struct {
		status    int
		body      string
		wantError bool
	}{
		{404, "", false}, {200, `{"tag_name":"v0.2.0","draft":true}`, false},
		{200, `{"tag_name":"v0.2.0","prerelease":true}`, false}, {200, `{"tag_name":"v0.0.9"}`, false},
		{200, `not JSON`, true}, {200, strings.Repeat("x", 1024*1024+1), true}, {302, `{}`, true}, {403, `{}`, true},
	} {
		checker := UpdateChecker{Repository: "example/midwing", Version: "0.1.0", HTTP: &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: tc.status, Body: ioBody(tc.body)}, nil
		})}}
		update, err := checker.Check(context.Background())
		if (err != nil) != tc.wantError || update.DownloadURL != "" {
			t.Errorf("status %d: %+v %v", tc.status, update, err)
		}
	}
}

func TestUpdateCheckDisabledAndCancelled(t *testing.T) {
	for _, repo := range []string{"", "https://evil.test/repo"} {
		checker := UpdateChecker{Repository: repo, Version: "0.1.0", HTTP: &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
			t.Fatal("unconfigured check made request")
			return nil, nil
		})}}
		_, _ = checker.Check(context.Background())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	checker := UpdateChecker{Repository: "example/midwing", Version: "0.1.0"}
	if _, err := checker.Check(ctx); err == nil {
		t.Fatal("ignored cancellation")
	}
}
