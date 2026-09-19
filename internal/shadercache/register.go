// Package shadercache registers optional build-time artifacts only after a
// separate graphics process has accepted them. Failure leaves runtime compilation.
package shadercache

import (
	"bytes"
	"compress/gzip"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"sync"

	"github.com/hajimehoshi/ebiten/v2/exp/shaderprecomp"
)

//go:embed data
var artifacts embed.FS

const archivePath = "data/shaders.json.gz"
const maxArchiveBytes = 32 << 20

type shaderArtifact struct {
	SourceID                     string
	Source                       string
	GLSL, GLSLES                 *struct{ Vertex, Fragment string }
	Metal, DXBCVertex, DXBCPixel []byte
}

// Decode the entire archive before registering anything: a bad later entry
// must not leave an irreversible partial registration in Ebitengine.
func decodeArchive(data []byte) ([]shaderArtifact, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	plain, err := io.ReadAll(io.LimitReader(r, maxArchiveBytes+1))
	if err != nil {
		return nil, err
	}
	if len(plain) > maxArchiveBytes {
		return nil, fmt.Errorf("shader archive too large")
	}
	var shaders []shaderArtifact
	if err := json.Unmarshal(plain, &shaders); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, s := range shaders {
		if _, err := shaderprecomp.ParseShaderSourceID(s.SourceID); err != nil {
			return nil, err
		}
		if seen[s.SourceID] || s.Source == "" {
			return nil, fmt.Errorf("duplicate shader or missing validation source: %s", s.SourceID)
		}
		seen[s.SourceID] = true
		if (len(s.DXBCVertex) == 0) != (len(s.DXBCPixel) == 0) {
			return nil, fmt.Errorf("incomplete DirectX shader: %s", s.SourceID)
		}
	}
	return shaders, nil
}

func register(shaders []shaderArtifact) {
	for _, s := range shaders {
		id := shaderprecomp.MustParseShaderSourceID(s.SourceID)
		if s.GLSL != nil && s.GLSLES != nil {
			shaderprecomp.RegisterGLSL(id, []byte(s.GLSL.Vertex), []byte(s.GLSL.Fragment), []byte(s.GLSLES.Vertex), []byte(s.GLSLES.Fragment))
		}
		if len(s.Metal) != 0 {
			shaderprecomp.RegisterMetalLibraryForMacOS(id, s.Metal)
		}
		if len(s.DXBCVertex) != 0 && len(s.DXBCPixel) != 0 {
			shaderprecomp.RegisterDXBCsForWindows(id, s.DXBCVertex, s.DXBCPixel)
		}
	}
}

var initializeOnce sync.Once

// Initialize must precede content loading and RunGame in every executable.
// Ebitengine cannot unregister a rejected binary or restart RunGame, so the
// child validates the exact embedded archive before the parent registers it.
func Initialize() {
	initializeOnce.Do(func() {
		data, err := artifacts.ReadFile(archivePath)
		if isProbe() {
			exitProbe(data, err)
			return
		}
		if err != nil {
			return
		}
		if err := initializeArchive(data, probeProcess, register); err != nil {
			log.Printf("Precompiled shaders unavailable; using runtime compilation: %v", err)
		}
	})
}

func initializeArchive(data []byte, probe func() error, publish func([]shaderArtifact)) error {
	shaders, err := decodeArchive(data)
	if err != nil {
		return err
	}
	if len(shaders) == 0 {
		return nil
	}
	if err := probe(); err != nil {
		return err
	}
	publish(shaders)
	return nil
}
