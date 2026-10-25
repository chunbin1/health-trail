// Package update finds a newer release of the app on GitHub and puts it in
// place of the running one: the release's zip is downloaded next to the app,
// checked to be signed by the same developer and notarized by Apple, swapped
// in, and the app relaunched once it has quit.
//
// Only a release built by release.yml carries the zip (HealthTrail-<v>.zip,
// the stapled .app); one without it is still shown, with its page to
// download the dmg from.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

// LatestURL is the newest release that isn't a pre-release or a draft.
const LatestURL = "https://api.github.com/repos/chunbin1/health-trail/releases/latest"

// ReleasesPage lists the releases, each with its dmg.
const ReleasesPage = "https://github.com/chunbin1/health-trail/releases/latest"

// Release is a published version.
type Release struct {
	Version   string `json:"version"` // 1.2.0, from the tag v1.2.0
	Name      string `json:"name"`
	Notes     string `json:"notes"` // the annotated tag's message, plain text
	Page      string `json:"page"`  // the release on GitHub, with the dmg
	Published string `json:"published"`
	// the zip the app installs; "" for a release without one
	ZipURL    string `json:"-"`
	ZipSize   int64  `json:"-"`
	ZipSHA256 string `json:"-"` // hex, when GitHub gives one
}

// Installable says whether the release can be installed from the app.
func (r *Release) Installable() bool { return r.ZipURL != "" }

// ErrNoRelease is a repository with no published release (or none it lets
// us see: a private one answers 404 too).
var ErrNoRelease = errors.New("no published release")

// Latest asks GitHub for the newest release.
func Latest(ctx context.Context, client *http.Client, url string) (*Release, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "health-trail")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNoRelease
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub answered %s", resp.Status)
	}
	var gh struct {
		Tag       string `json:"tag_name"`
		Name      string `json:"name"`
		Body      string `json:"body"`
		Page      string `json:"html_url"`
		Published string `json:"published_at"`
		Assets    []struct {
			Name   string `json:"name"`
			URL    string `json:"browser_download_url"`
			Size   int64  `json:"size"`
			Digest string `json:"digest"` // sha256:…
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&gh); err != nil {
		return nil, fmt.Errorf("reading the release: %w", err)
	}
	r := &Release{Version: strings.TrimPrefix(gh.Tag, "v"), Name: gh.Name, Notes: notes(gh.Body, gh.Name), Page: gh.Page, Published: gh.Published}
	if _, ok := parse(r.Version); !ok {
		return nil, fmt.Errorf("the release's tag %q isn't a version", gh.Tag)
	}
	for _, a := range gh.Assets {
		if a.Name == "HealthTrail-"+r.Version+".zip" {
			r.ZipURL, r.ZipSize, r.ZipSHA256 = a.URL, a.Size, strings.TrimPrefix(a.Digest, "sha256:")
		}
	}
	return r, nil
}

// notes is the release's text without its first line when that is the
// title again: the tag message starts with "健康足迹 1.2.0", as the release
// is named.
func notes(body, name string) string {
	first, rest, _ := strings.Cut(strings.TrimLeft(body, "\r\n"), "\n")
	if name != "" && strings.TrimSpace(first) == strings.TrimSpace(name) {
		return strings.TrimSpace(rest)
	}
	return strings.TrimSpace(body)
}

type version struct {
	nums [3]int
	pre  string // test.1 in 1.0.0-test.1
}

var (
	versionRe = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.]+))?$`)
	// what git describe adds when built from commits after the tag
	describeRe = regexp.MustCompile(`-\d+-g[0-9a-f]+$`)
)

func parse(s string) (version, bool) {
	m := versionRe.FindStringSubmatch(s)
	if m == nil {
		return version{}, false
	}
	var v version
	for i := range v.nums {
		v.nums[i], _ = strconv.Atoi(m[i+1])
	}
	v.pre = m[4]
	return v, true
}

// Newer says whether release (a plain version like 1.2.0) comes after
// current, the running build's version. A build from commits after a tag
// (1.2.0-3-gabc1234) counts as that tag; a test build of a version
// (1.2.0-test.1) comes before it. A current that isn't a version (a bare
// commit hash) is never behind.
func Newer(release, current string) bool {
	r, ok := parse(release)
	c, ok2 := parse(describeRe.ReplaceAllString(strings.TrimPrefix(current, "v"), ""))
	if !ok || !ok2 || r.pre != "" {
		return false
	}
	for i := range r.nums {
		if r.nums[i] != c.nums[i] {
			return r.nums[i] > c.nums[i]
		}
	}
	return c.pre != ""
}
