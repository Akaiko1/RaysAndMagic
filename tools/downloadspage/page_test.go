package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

const testRepo = "owner/game"

func testDigest(seed string) string { return "sha256:" + strings.Repeat(seed, 64)[:64] }

func testRelease(tag string, day int, assets ...string) ghRelease {
	r := ghRelease{
		Tag:         tag,
		PublishedAt: time.Date(2026, 9, day, 12, 0, 0, 0, time.UTC),
		HTMLURL:     "https://github.com/" + testRepo + "/releases/tag/" + tag,
	}
	for i, name := range assets {
		r.Assets = append(r.Assets, ghAsset{
			Name: name, Size: int64(i+1) << 30, Digest: testDigest(fmt.Sprint(i)),
			URL: "https://github.com/" + testRepo + "/releases/download/" + tag + "/" + name,
		})
	}
	return r
}

func tagsFor(releases ...ghRelease) []ghTag {
	var tags []ghTag
	for i, r := range releases {
		t := ghTag{Name: r.Tag}
		t.Commit.SHA = fmt.Sprintf("%040d", i+1)
		tags = append(tags, t)
	}
	return tags
}

var (
	allArchives    = []string{"RaysAndMagic-windows-amd64.zip", "RaysAndMagic-mac-universal.zip"}
	legacyArchives = []string{"RaysAndMagic-windows-amd64.zip", "RaysAndMagic-mac-amd64.zip", "RaysAndMagic-mac-arm64.zip"}
)

func TestBuildPage(t *testing.T) {
	oldStable := testRelease("v0.9.8.stable", 10, legacyArchives...)
	stable := testRelease("v0.9.9.stable", 20, allArchives...)
	nightly := testRelease("v0.9.9.1.nightly", 25, allArchives...)
	draft := testRelease("v1.0.stable", 28, allArchives...)
	draft.Draft = true
	empty := testRelease("v1.0.nightly", 27)
	cases := []struct {
		name       string
		releases   []ghRelease
		tags       []ghTag // nil: every release's tag resolves
		wantErr    string
		wantLatest []string
		wantOrder  []string
		wantAssets []string // download order on every latest card; nil: current archives
	}{
		{name: "latest per channel in channel order", releases: []ghRelease{nightly, oldStable, stable},
			wantLatest: []string{"v0.9.9.stable", "v0.9.9.1.nightly"},
			wantOrder:  []string{"v0.9.9.1.nightly", "v0.9.9.stable", "v0.9.8.stable"}},
		{name: "draft never listed", releases: []ghRelease{stable, draft},
			wantLatest: []string{"v0.9.9.stable"}, wantOrder: []string{"v0.9.9.stable"}},
		{name: "release without archives stays out of latest", releases: []ghRelease{stable, nightly, empty},
			wantLatest: []string{"v0.9.9.stable", "v0.9.9.1.nightly"},
			wantOrder:  []string{"v1.0.nightly", "v0.9.9.1.nightly", "v0.9.9.stable"}},
		{name: "single channel only", releases: []ghRelease{nightly},
			wantLatest: []string{"v0.9.9.1.nightly"}, wantOrder: []string{"v0.9.9.1.nightly"}},
		{name: "legacy per-arch archives stay listed", releases: []ghRelease{oldStable},
			wantLatest: []string{"v0.9.8.stable"}, wantOrder: []string{"v0.9.8.stable"},
			wantAssets: []string{"RaysAndMagic-windows-amd64.zip", "RaysAndMagic-mac-arm64.zip", "RaysAndMagic-mac-amd64.zip"}},
		{name: "unknown channel", releases: []ghRelease{testRelease("v1.0.beta", 1, allArchives...)}, wantErr: `unknown channel "beta"`},
		{name: "tag without channel", releases: []ghRelease{testRelease("v1.0", 1, allArchives...)}, wantErr: "v<version>.<channel>"},
		{name: "unknown archive", releases: []ghRelease{testRelease("v1.0.stable", 1, "RaysAndMagic-linux.zip")}, wantErr: `unknown archive "RaysAndMagic-linux.zip"`},
		{name: "tag missing from tag list", releases: []ghRelease{stable}, tags: []ghTag{}, wantErr: "tag missing"},
		{name: "nothing downloadable", releases: []ghRelease{empty}, wantErr: "no published release"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tags := tc.tags
			if tags == nil {
				tags = tagsFor(tc.releases...)
			}
			p, err := buildPage(tc.releases, tags, testRepo)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error %v, want it to mention %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var latest, order []string
			for _, r := range p.Latest {
				latest = append(latest, r.Tag)
			}
			for _, r := range p.History {
				order = append(order, r.Tag)
			}
			if !slices.Equal(latest, tc.wantLatest) || !slices.Equal(order, tc.wantOrder) {
				t.Fatalf("latest %v history %v, want %v and %v", latest, order, tc.wantLatest, tc.wantOrder)
			}
			want := tc.wantAssets
			if want == nil {
				want = []string{"RaysAndMagic-mac-universal.zip", "RaysAndMagic-windows-amd64.zip"}
			}
			for _, r := range p.Latest {
				var got []string
				for _, d := range r.Downloads {
					got = append(got, d.Platform.Asset)
				}
				if !slices.Equal(got, want) {
					t.Fatalf("%s downloads %v, want platform order %v", r.Tag, got, want)
				}
			}
		})
	}
}

func TestSHA256Of(t *testing.T) {
	hex := strings.Repeat("ab", 32)
	for _, tc := range []struct{ in, want string }{
		{"sha256:" + hex, hex},
		{"", ""},
		{"sha512:" + hex, ""},
		{"sha256:abc", ""},
	} {
		if got := sha256Of(tc.in); got != tc.want {
			t.Errorf("sha256Of(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// The page's current platforms and the archives release.yml publishes are one
// list: a renamed or added archive must fail here, not vanish from the page.
func TestReleaseWorkflowArchivesMatchPlatforms(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/release.yml")
	if err != nil {
		t.Fatal(err)
	}
	var built []string
	for _, m := range regexp.MustCompile(`zip -r (\S+\.zip)`).FindAllStringSubmatch(string(data), -1) {
		built = append(built, m[1])
	}
	var known []string
	for _, p := range platforms {
		if !p.Legacy {
			known = append(known, p.Asset)
		}
	}
	slices.Sort(built)
	slices.Sort(known)
	if !slices.Equal(built, known) {
		t.Fatalf("release.yml archives %v, page platforms %v", built, known)
	}
	if !strings.Contains(string(data), "uses: ./.github/workflows/pages.yml") {
		t.Fatal("release.yml no longer republishes the downloads page")
	}
}

func TestGenerateWritesSite(t *testing.T) {
	dir := t.TempDir()
	stable := testRelease("v0.9.9.stable", 20, allArchives...)
	stable.HTMLURL += "?a=1&b=2"
	writeStream := func(name string, lines ...string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	asset := func(a ghAsset) string {
		return fmt.Sprintf(`{"name":%q,"size":%d,"digest":%q,"browser_download_url":%q}`, a.Name, a.Size, a.Digest, a.URL)
	}
	var assets []string
	for _, a := range stable.Assets {
		assets = append(assets, asset(a))
	}
	releases := writeStream("releases.ndjson",
		fmt.Sprintf(`{"tag_name":%q,"draft":false,"published_at":"2026-09-20T12:00:00Z","html_url":%q,"assets":[%s]}`,
			stable.Tag, stable.HTMLURL, strings.Join(assets, ",")))
	tags := writeStream("tags.ndjson", `{"name":"v0.9.9.stable","commit":{"sha":"0123456789abcdef0123456789abcdef01234567"}}`)
	image := writeStream("shot.png", "png")
	out := filepath.Join(dir, "site")
	if err := generate(releases, tags, testRepo, "https://example.test/run/1", image, out); err != nil {
		t.Fatal(err)
	}
	html, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	page := string(html)
	for _, want := range []string{
		stable.Assets[0].URL,
		strings.Repeat("0", 64),
		"https://github.com/owner/game/commit/0123456789abcdef0123456789abcdef01234567",
		">01234567<",
		"?a=1&amp;b=2",
		`src="screenshot.png"`,
		"https://example.test/run/1",
		"ZIP, 1.00 GB",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page is missing %q", want)
		}
	}
	for i, r := range page {
		if r > 127 {
			t.Fatalf("non-ASCII rune %q at byte %d", r, i)
		}
	}
	if _, err := os.Stat(filepath.Join(out, "screenshot.png")); err != nil {
		t.Fatal("screenshot not copied into the site")
	}
}

// The rule the release workflow checks a pushed tag against before building.
func TestParseTag(t *testing.T) {
	for _, tc := range []struct {
		tag, version, channel, wantErr string
	}{
		{"v0.9.9.3.nightly", "0.9.9.3", "nightly", ""},
		{"v1.0.stable", "1.0", "stable", ""},
		{"v0.9.9.3.nighly", "", "", `unknown channel "nighly"`},
		{"v1.0", "", "", "v<version>.<channel>"},
		{"0.9.9.3.nightly", "", "", "v<version>.<channel>"},
		{"v0.9.9.3.Nightly", "", "", "v<version>.<channel>"},
	} {
		t.Run(tc.tag, func(t *testing.T) {
			version, ch, err := parseTag(tc.tag)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error %v, want it to mention %q", err, tc.wantErr)
				}
				return
			}
			if err != nil || version != tc.version || ch.Key != tc.channel {
				t.Fatalf("got %q %q %v, want %q %q", version, ch.Key, err, tc.version, tc.channel)
			}
		})
	}
}
