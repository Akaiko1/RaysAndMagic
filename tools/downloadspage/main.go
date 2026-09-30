// Downloadspage renders the static downloads site from GitHub release metadata.
// The Pages workflow feeds it `gh api --paginate --jq '.[]'` dumps of the
// releases and tags; the archives themselves stay on GitHub Releases.
package main

import (
	_ "embed"
	"flag"
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"time"
)

//go:embed page.html
var pageHTML string

var pageTemplate = template.Must(template.New("page").Funcs(template.FuncMap{
	"date": func(t time.Time) string { return t.UTC().Format("2006-01-02 15:04 UTC") },
}).Parse(pageHTML))

func main() {
	releasesPath := flag.String("releases", "", "releases dump (one JSON object per line)")
	tagsPath := flag.String("tags", "", "tags dump (one JSON object per line)")
	repo := flag.String("repo", "", "owner/name of the GitHub repository")
	runURL := flag.String("run-url", "", "workflow run that built the page (optional)")
	image := flag.String("image", "", "screenshot copied into the site (optional)")
	out := flag.String("out", "site", "output directory")
	checkTag := flag.String("check-tag", "", "only validate this release tag and exit")
	flag.Parse()
	if *checkTag != "" {
		if _, _, err := parseTag(*checkTag); err != nil {
			fmt.Fprintln(os.Stderr, "downloadspage:", err)
			os.Exit(1)
		}
		return
	}
	if err := generate(*releasesPath, *tagsPath, *repo, *runURL, *image, *out); err != nil {
		fmt.Fprintln(os.Stderr, "downloadspage:", err)
		os.Exit(1)
	}
}

func generate(releasesPath, tagsPath, repo, runURL, image, out string) error {
	if releasesPath == "" || tagsPath == "" || repo == "" {
		return fmt.Errorf("-releases, -tags and -repo are required")
	}
	releases, err := readStream[ghRelease](releasesPath)
	if err != nil {
		return err
	}
	tags, err := readStream[ghTag](tagsPath)
	if err != nil {
		return err
	}
	p, err := buildPage(releases, tags, repo)
	if err != nil {
		return err
	}
	p.RunURL, p.Built = runURL, time.Now().UTC()
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	if image != "" {
		p.Image = "screenshot" + filepath.Ext(image)
		if err := copyFile(image, filepath.Join(out, p.Image)); err != nil {
			return err
		}
	}
	f, err := os.Create(filepath.Join(out, "index.html"))
	if err != nil {
		return err
	}
	if err := pageTemplate.Execute(f, p); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func readStream[T any](path string) ([]T, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return decodeStream[T](f, path)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
