package midwing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// UpdateChecker reads public release metadata independently of account credentials.
type UpdateChecker struct {
	Repository string
	Version    string
	HTTP       *http.Client
	mu         sync.Mutex
	nextCheck  time.Time
	latest     UpdateInfo
}

type UpdateInfo struct {
	Version     string `json:"version"`
	DownloadURL string `json:"downloadURL"`
}

var releaseVersion = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var releaseRepository = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9_.-]+$`)

func newerRelease(latest, current string) bool {
	a, b := releaseVersion.FindStringSubmatch(latest), releaseVersion.FindStringSubmatch(current)
	if a == nil || b == nil {
		return false
	}
	for i := 1; i <= 3; i++ {
		x, ex := strconv.ParseUint(a[i], 10, 64)
		y, ey := strconv.ParseUint(b[i], 10, 64)
		if ex != nil || ey != nil {
			return false
		}
		if x != y {
			return x > y
		}
	}
	return false
}

// Check caches successful checks for a day and retries failures after an hour.
// Existing notices remain available if a later check fails.
func (c *UpdateChecker) Check(ctx context.Context) (UpdateInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Repository == "" || !releaseVersion.MatchString(c.Version) {
		return UpdateInfo{}, nil
	}
	if !releaseRepository.MatchString(c.Repository) {
		return UpdateInfo{}, errors.New("invalid release repository")
	}
	if time.Now().Before(c.nextCheck) {
		return c.latest, nil
	}
	c.nextCheck = time.Now().Add(time.Hour)
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/"+c.Repository+"/releases/latest", nil)
	if err != nil {
		return c.latest, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "Midwing/"+c.Version)
	client := http.Client{Timeout: 8 * time.Second}
	if c.HTTP != nil {
		client = *c.HTTP
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return c.latest, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		c.latest = UpdateInfo{}
		c.nextCheck = time.Now().Add(24 * time.Hour)
		return c.latest, nil
	}
	if resp.StatusCode != http.StatusOK {
		return c.latest, fmt.Errorf("release check failed (HTTP %d)", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024+1))
	if err != nil || len(body) > 1024*1024 {
		return c.latest, errors.New("could not read release metadata")
	}
	var release struct {
		Tag        string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	if err := json.Unmarshal(body, &release); err != nil {
		return c.latest, err
	}
	c.latest = UpdateInfo{}
	if !release.Draft && !release.Prerelease && newerRelease(release.Tag, c.Version) {
		c.latest = UpdateInfo{Version: strings.TrimPrefix(release.Tag, "v"), DownloadURL: "https://github.com/" + c.Repository + "/releases/tag/" + url.PathEscape(release.Tag)}
	}
	c.nextCheck = time.Now().Add(24 * time.Hour)
	return c.latest, nil
}
