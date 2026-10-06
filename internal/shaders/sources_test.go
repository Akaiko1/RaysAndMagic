package shaders

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

// Shader cache IDs hash the full expanded bytes, including line endings.
func TestSourcesHaveStableBytesAcrossCheckoutLineEndings(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		programCRLF, includeCRLF bool
	}{
		{"LF", false, false},
		{"CRLF", true, true},
		{"CRLF_program", true, false},
		{"CRLF_includes", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checkout := fstest.MapFS{}
			entries, err := files.ReadDir(".")
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				data, err := fs.ReadFile(files, entry.Name())
				if err != nil {
					t.Fatal(err)
				}
				source := strings.ReplaceAll(string(data), "\r\n", "\n")
				crlf := tc.programCRLF
				if strings.HasSuffix(entry.Name(), ".kageinc") {
					crlf = tc.includeCRLF
				}
				if crlf {
					source = strings.ReplaceAll(source, "\n", "\r\n")
				}
				checkout[entry.Name()] = &fstest.MapFile{Data: []byte(source)}
			}
			for _, name := range Names() {
				got := expand(checkout, name, make(map[string]bool))
				if got != Source(name) || strings.ContainsRune(got, '\r') {
					t.Errorf("%s changed expanded bytes with %s checkout", name, tc.name)
				}
			}
		})
	}
}
