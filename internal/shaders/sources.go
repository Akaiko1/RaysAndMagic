// Package shaders owns the embedded Kage sources used by the game and tools.
package shaders

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"
)

//go:embed *.kage *.kageinc
var files embed.FS

var sources = loadSources()

// Names lists complete shaders. Shared .kageinc fragments are not programs.
func Names() []string {
	entries, err := files.ReadDir(".")
	if err != nil {
		panic(err)
	}
	var names []string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".kage") {
			names = append(names, entry.Name())
		}
	}
	return names
}

// Source returns one complete source with its shared fragments expanded.
// Missing files and invalid includes are build defects, not optional content.
func Source(name string) string {
	source, ok := sources[name]
	if !ok {
		panic(fmt.Sprintf("unknown shader %q", name))
	}
	return source
}

func loadSources() map[string]string {
	result := make(map[string]string)
	for _, name := range Names() {
		result[name] = expand(files, name, make(map[string]bool))
	}
	return result
}

func expand(sourceFS fs.FS, name string, active map[string]bool) string {
	if active[name] {
		panic(fmt.Sprintf("cyclic shader include %q", name))
	}
	data, err := fs.ReadFile(sourceFS, name)
	if err != nil {
		panic(err)
	}
	active[name] = true
	defer delete(active, name)
	var source strings.Builder
	// Source IDs include source bytes. Normalize Windows checkouts before both
	// runtime compilation and cross-platform precompiled-cache generation.
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	for _, line := range strings.SplitAfter(text, "\n") {
		if include, ok := strings.CutPrefix(strings.TrimSpace(line), "//include "); ok {
			if !strings.HasSuffix(include, ".kageinc") {
				panic(fmt.Sprintf("invalid shader include %q in %s", include, name))
			}
			source.WriteString(expand(sourceFS, include, active))
		} else {
			source.WriteString(line)
		}
	}
	return source.String()
}
