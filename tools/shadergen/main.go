// Shadergen must run from the repository root with this module's Ebitengine.
// Regenerating also collects engine built-ins, avoiding a second shader list.
package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

type shader struct {
	Source                       string
	SourceID                     string
	GLSL, GLSLES                 *struct{ Vertex, Fragment string }
	HLSL                         *struct{ Vertex, Pixel string } `json:",omitempty"`
	MSL                          *struct{ Shader string }        `json:",omitempty"`
	Metal, DXBCVertex, DXBCPixel []byte                          `json:",omitempty"`
}

func main() {
	if err := generate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

const artifactPath = "internal/shadercache/data/shaders.json.gz"

// Keep other platforms' compiled artifacts only for identical engine/source
// IDs. CI prepares DXBC on Windows, then adds Metal on the macOS release host.
func mergePlatformArtifacts(shaders, previous []shader) {
	byID := make(map[string]shader, len(previous))
	for _, s := range previous {
		byID[s.SourceID] = s
	}
	for i := range shaders {
		old := byID[shaders[i].SourceID]
		shaders[i].Metal = old.Metal
		shaders[i].DXBCVertex, shaders[i].DXBCPixel = old.DXBCVertex, old.DXBCPixel
	}
}

func previousArtifacts() []shader {
	data, err := os.ReadFile(artifactPath)
	if err != nil {
		return nil
	}
	z, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	defer z.Close()
	var shaders []shader
	if json.NewDecoder(z).Decode(&shaders) != nil {
		return nil
	}
	return shaders
}

func generate() error {
	cmd := exec.Command("go", "run", "github.com/hajimehoshi/ebiten/v2/internal/shadercollector", "-target", "glsl,hlsl,msl", "./internal/game", "./assets/map_viewer")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("collect shaders: %w", err)
	}
	var shaders []shader
	if err := json.Unmarshal(out, &shaders); err != nil {
		return err
	}
	mergePlatformArtifacts(shaders, previousArtifacts())
	dir, err := os.MkdirTemp("", "raysandmagic-shaders-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	metal := runtime.GOOS == "darwin" && exec.Command("xcrun", "-sdk", "macosx", "metal", "--version").Run() == nil
	fxc, _ := exec.LookPath("fxc.exe")
	if runtime.GOOS == "darwin" && !metal {
		fmt.Fprintln(os.Stderr, "Metal compiler unavailable: generating GLSL only; Metal uses runtime compilation.")
	}
	if runtime.GOOS == "windows" && fxc == "" {
		fmt.Fprintln(os.Stderr, "fxc.exe unavailable: generating GLSL only; DirectX uses runtime compilation.")
	}
	for i := range shaders {
		s := &shaders[i]
		if metal && s.MSL != nil {
			src, ir, lib := filepath.Join(dir, "source.metal"), filepath.Join(dir, "shader.air"), filepath.Join(dir, "shader.metallib")
			if err := os.WriteFile(src, []byte(s.MSL.Shader), 0600); err != nil {
				return err
			}
			if err := run("xcrun", "-sdk", "macosx", "metal", "-mmacosx-version-min=10.15", "-c", src, "-o", ir); err != nil {
				return err
			}
			if err := run("xcrun", "-sdk", "macosx", "metallib", ir, "-o", lib); err != nil {
				return err
			}
			s.Metal, err = os.ReadFile(lib)
			if err != nil {
				return err
			}
		}
		if fxc != "" && s.HLSL != nil {
			for _, stage := range []struct {
				source, entry, profile string
				result                 *[]byte
			}{
				{s.HLSL.Vertex, "VSMain", "vs_4_0", &s.DXBCVertex},
				{s.HLSL.Pixel, "PSMain", "ps_4_0", &s.DXBCPixel},
			} {
				src, binary := filepath.Join(dir, "source.hlsl"), filepath.Join(dir, "shader.dxbc")
				if err := os.WriteFile(src, []byte(stage.source), 0600); err != nil {
					return err
				}
				if err := run(fxc, "/nologo", "/O3", "/T", stage.profile, "/E", stage.entry, "/Fo", binary, src); err != nil {
					return err
				}
				*stage.result, err = os.ReadFile(binary)
				if err != nil {
					return err
				}
			}
		}
		s.HLSL, s.MSL = nil, nil
	}
	var buf bytes.Buffer
	z := gzip.NewWriter(&buf)
	if err := json.NewEncoder(z).Encode(shaders); err != nil {
		return err
	}
	if err := z.Close(); err != nil {
		return err
	}
	// Publish only after every compiler succeeded. A failed build keeps the old
	// valid artifacts; plain go build also works with runtime compilation.
	const target = artifactPath
	if err := os.WriteFile(target+".tmp", buf.Bytes(), 0644); err != nil {
		return err
	}
	if err := os.Rename(target+".tmp", target); err != nil {
		return err
	}
	var metalCount, dxbcCount int
	for _, s := range shaders {
		if len(s.Metal) != 0 {
			metalCount++
		}
		if len(s.DXBCVertex) != 0 && len(s.DXBCPixel) != 0 {
			dxbcCount++
		}
	}
	fmt.Printf("Prepared %d shaders: GLSL=%d Metal=%d DXBC=%d\n", len(shaders), len(shaders), metalCount, dxbcCount)
	return nil
}
