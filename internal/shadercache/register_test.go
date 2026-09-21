package shadercache

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	if isProbe() {
		if fixture := os.Getenv("RAM_SHADER_TEST_ARCHIVE"); fixture != "" {
			data, err := os.ReadFile(fixture)
			exitProbe(data, err)
		}
		Initialize()
	}
	os.Exit(m.Run())
}
func archiveForTest(t *testing.T, shaders []shaderArtifact) []byte {
	t.Helper()
	var b bytes.Buffer
	z := gzip.NewWriter(&b)
	if err := json.NewEncoder(z).Encode(shaders); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func TestShaderArchiveFallback(t *testing.T) {
	good := shaderArtifact{SourceID: "aaaaaaaaaaaaaaaaaaaaaaaaaa", Source: "package main\nfunc Fragment(dst vec4, src vec2, c vec4) vec4 {return c}"}
	for _, tc := range []struct {
		name                         string
		entries                      []shaderArtifact
		corrupt, reject, wantPublish bool
	}{
		{name: "accepted", entries: []shaderArtifact{good}, wantPublish: true},
		{name: "empty"},
		{name: "corrupt_checksum", entries: []shaderArtifact{good}, corrupt: true},
		{name: "duplicate", entries: []shaderArtifact{good, good}},
		{name: "invalid_ID", entries: []shaderArtifact{{SourceID: "bad", Source: good.Source}}},
		{name: "legacy_without_source", entries: []shaderArtifact{{SourceID: good.SourceID}}},
		{name: "partial_DXBC", entries: []shaderArtifact{{SourceID: good.SourceID, Source: good.Source, DXBCVertex: []byte{1}}}},
		{name: "backend_rejection", entries: []shaderArtifact{good}, reject: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := archiveForTest(t, tc.entries)
			if tc.corrupt {
				data[len(data)-1] ^= 1
			}
			published, probed := false, false
			err := initializeArchive(data, func() error {
				probed = true
				if tc.reject {
					return errors.New("backend rejected binary")
				}
				return nil
			}, func([]shaderArtifact) { published = true }, shaderProbeCache{})
			if published != tc.wantPublish {
				t.Fatalf("published=%v error=%v", published, err)
			}
			if tc.name != "empty" && !tc.wantPublish && err == nil {
				t.Fatal("failure not reported")
			}
			if tc.name != "accepted" && tc.name != "backend_rejection" && probed {
				t.Fatal("invalid archive reached GPU")
			}
		})
	}
}

// Real child-process validation: corrupt native artifacts must fail before the
// parent's irreversible registration, while the same Kage works at runtime.
func TestShaderNativeValidationAndFallback(t *testing.T) {
	if os.Getenv("RAM_SHADER_GPU_TESTS") != "1" {
		t.Skip("opt-in live graphics subprocess")
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("native binary backends")
	}
	data, err := artifacts.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	originals, err := decodeArchive(data)
	if err != nil {
		t.Fatal(err)
	}
	modes := []string{"shipped", "runtime"}
	for _, s := range originals {
		modes = append(modes, "rejected/"+s.SourceID)
	}
	for _, mode := range modes {
		t.Run(mode, func(t *testing.T) {
			shaders := append([]shaderArtifact(nil), originals...)
			for i := range shaders {
				if mode == "runtime" {
					shaders[i].Metal = nil
					shaders[i].DXBCVertex = nil
					shaders[i].DXBCPixel = nil
					shaders[i].GLSL = nil
					shaders[i].GLSLES = nil
				}
				if mode == "rejected/"+shaders[i].SourceID {
					if runtime.GOOS == "darwin" {
						shaders[i].Metal = []byte("invalid Metal library")
					} else {
						shaders[i].DXBCVertex = []byte("invalid DXBC")
						shaders[i].DXBCPixel = []byte("invalid DXBC")
					}
				}
			}
			file := filepath.Join(t.TempDir(), "shaders.gz")
			payload := archiveForTest(t, shaders)
			if err := os.WriteFile(file, payload, 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("RAM_SHADER_TEST_ARCHIVE", file)
			published := false
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			cache := shaderProbeCache{executable: executable, dir: t.TempDir()}
			probes := 0
			probe := func() error { probes++; return probeProcess() }
			err = initializeArchive(payload, probe, func([]shaderArtifact) { published = true }, cache)
			if strings.HasPrefix(mode, "rejected/") {
				if err == nil || published {
					t.Fatal("rejected binaries reached parent")
				}
			} else if err != nil || !published {
				t.Fatalf("%s: %v", mode, err)
			}
			if !strings.HasPrefix(mode, "rejected/") {
				published = false
				if err := initializeArchive(payload, probe, func([]shaderArtifact) { published = true }, cache); err != nil || !published || probes != 1 {
					t.Fatalf("validated restart: published=%v probes=%d err=%v", published, probes, err)
				}
			}
		})
	}
}
