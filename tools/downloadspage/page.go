package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"
)

// GitHub API shapes, read from `gh api --paginate --jq '.[]'` (one object per line).
type ghAsset struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"`
	URL    string `json:"browser_download_url"`
}

type ghRelease struct {
	Tag         string    `json:"tag_name"`
	Draft       bool      `json:"draft"`
	PublishedAt time.Time `json:"published_at"`
	HTMLURL     string    `json:"html_url"`
	Assets      []ghAsset `json:"assets"`
}

type ghTag struct {
	Name   string `json:"name"`
	Commit struct {
		SHA string `json:"sha"`
	} `json:"commit"`
}

// channel is one release track, named by the tag suffix (v0.9.9.1.stable).
type channel struct {
	Key, Title, Blurb string
}

var channels = []channel{
	{Key: "stable", Title: "Stable", Blurb: "Recommended build."},
	{Key: "nightly", Title: "Nightly", Blurb: "Newest development build. Expect rough edges."},
}

// platform maps one release archive to its download card. Current archives
// are exactly the ones release.yml builds; legacy ones only appear in older
// releases and stay known so the history keeps its links.
type platform struct {
	Asset, Title, Detail string
	Legacy               bool
}

var platforms = []platform{
	{Asset: "RaysAndMagic-mac-universal.zip", Title: "macOS", Detail: "Apple Silicon and Intel"},
	{Asset: "RaysAndMagic-windows-amd64.zip", Title: "Windows", Detail: "64-bit"},
	{Asset: "RaysAndMagic-mac-arm64.zip", Title: "macOS", Detail: "Apple Silicon", Legacy: true},
	{Asset: "RaysAndMagic-mac-amd64.zip", Title: "macOS", Detail: "Intel", Legacy: true},
}

var tagPattern = regexp.MustCompile(`^v([0-9]+(?:\.[0-9]+)*)\.([a-z]+)$`)

type download struct {
	Platform platform
	URL      string
	Size     string
	SHA256   string
}

type release struct {
	Tag, Version, URL string
	Channel           channel
	Published         time.Time
	Commit            string
	Downloads         []download
}

func (r release) ShortCommit() string { return r.Commit[:min(8, len(r.Commit))] }

type page struct {
	Repo    string
	RunURL  string
	Built   time.Time
	Latest  []release // newest downloadable release per channel, in channel order
	History []release // every published release, newest first
	Image   string    // screenshot file name inside the site, "" for none
}

func decodeStream[T any](r io.Reader, what string) ([]T, error) {
	var out []T
	dec := json.NewDecoder(bufio.NewReader(r))
	for {
		var v T
		if err := dec.Decode(&v); errors.Is(err, io.EOF) {
			return out, nil
		} else if err != nil {
			return nil, fmt.Errorf("%s: %w", what, err)
		}
		out = append(out, v)
	}
}

// buildPage validates the release metadata and arranges it for the template.
// Unknown channels or archives fail instead of silently vanishing from the page.
func buildPage(releases []ghRelease, tags []ghTag, repo string) (page, error) {
	commits := make(map[string]string, len(tags))
	for _, t := range tags {
		commits[t.Name] = t.Commit.SHA
	}
	p := page{Repo: repo}
	for _, gr := range releases {
		if gr.Draft {
			continue
		}
		r, err := convertRelease(gr, commits)
		if err != nil {
			return page{}, err
		}
		p.History = append(p.History, r)
	}
	sort.SliceStable(p.History, func(i, j int) bool {
		if !p.History[i].Published.Equal(p.History[j].Published) {
			return p.History[i].Published.After(p.History[j].Published)
		}
		return p.History[i].Tag > p.History[j].Tag
	})
	for _, ch := range channels {
		for _, r := range p.History {
			if r.Channel.Key == ch.Key && len(r.Downloads) > 0 {
				p.Latest = append(p.Latest, r)
				break
			}
		}
	}
	if len(p.Latest) == 0 {
		return page{}, errors.New("no published release has downloadable archives")
	}
	return p, nil
}

func convertRelease(gr ghRelease, commits map[string]string) (release, error) {
	m := tagPattern.FindStringSubmatch(gr.Tag)
	if m == nil {
		return release{}, fmt.Errorf("release %q: tag must look like v<version>.<channel>", gr.Tag)
	}
	r := release{Tag: gr.Tag, Version: m[1], URL: gr.HTMLURL, Published: gr.PublishedAt.UTC()}
	found := false
	for _, ch := range channels {
		if ch.Key == m[2] {
			r.Channel, found = ch, true
		}
	}
	if !found {
		return release{}, fmt.Errorf("release %q: unknown channel %q (known: %s)", gr.Tag, m[2], channelKeys())
	}
	sha, ok := commits[gr.Tag]
	if !ok || sha == "" {
		return release{}, fmt.Errorf("release %q: tag missing from the tag list", gr.Tag)
	}
	r.Commit = sha
	byName := make(map[string]ghAsset, len(gr.Assets))
	for _, a := range gr.Assets {
		if platformFor(a.Name) == nil {
			return release{}, fmt.Errorf("release %q: unknown archive %q; add it to platforms", gr.Tag, a.Name)
		}
		byName[a.Name] = a
	}
	for _, pl := range platforms {
		a, ok := byName[pl.Asset]
		if !ok {
			continue
		}
		r.Downloads = append(r.Downloads, download{Platform: pl, URL: a.URL, Size: humanSize(a.Size), SHA256: sha256Of(a.Digest)})
	}
	return r, nil
}

func platformFor(asset string) *platform {
	for i := range platforms {
		if platforms[i].Asset == asset {
			return &platforms[i]
		}
	}
	return nil
}

func channelKeys() string {
	keys := make([]string, len(channels))
	for i, ch := range channels {
		keys[i] = ch.Key
	}
	return strings.Join(keys, ", ")
}

// sha256Of returns the hex part of a GitHub "sha256:<hex>" digest, "" otherwise
// (assets uploaded before GitHub recorded digests have none).
func sha256Of(digest string) string {
	hex, ok := strings.CutPrefix(digest, "sha256:")
	if !ok || len(hex) != 64 {
		return ""
	}
	return hex
}

func humanSize(n int64) string {
	const mb, gb = 1 << 20, 1 << 30
	if n >= gb {
		return fmt.Sprintf("%.2f GB", float64(n)/gb)
	}
	return fmt.Sprintf("%.0f MB", float64(n)/mb)
}
