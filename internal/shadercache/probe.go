package shadercache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

const probeEnv = "RAM_SHADER_PROBE"
const probeSuccess = "RaysAndMagic shader validation complete"

func isProbe() bool { return os.Getenv(probeEnv) == "1" }

type shaderProbeCache struct{ executable, dir string }

// The cache is per user and available before bundle data setup runs.
func newShaderProbeCache() shaderProbeCache {
	executable, err := os.Executable()
	if err != nil {
		return shaderProbeCache{}
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return shaderProbeCache{}
	}
	return shaderProbeCache{executable: executable, dir: filepath.Join(dir, "RaysAndMagic", "shader-probes")}
}

// Cache only successful GPU admission. Hash the binary itself, not its timestamp
// or version label, so rebuilding either executable invalidates its own marker.
func probeWithCache(data []byte, executable, dir string, probe func() error) error {
	if dir == "" {
		return probe()
	}
	key, err := probeCacheKey(data, executable)
	if err != nil {
		return probe()
	}
	marker := filepath.Join(dir, key+".ok")
	if saved, err := os.ReadFile(marker); err == nil && string(saved) == probeSuccess {
		return nil
	}
	if err := probe(); err != nil {
		return err
	}
	if os.MkdirAll(dir, 0755) != nil {
		return nil // Optional cache failure must not reject a validated archive.
	}
	f, err := os.CreateTemp(dir, ".shader-probe-*")
	if err != nil {
		return nil
	}
	defer os.Remove(f.Name())
	_, writeErr := io.WriteString(f, probeSuccess)
	closeErr := f.Close()
	if writeErr == nil && closeErr == nil {
		_ = os.Rename(f.Name(), marker)
	}
	return nil
}

func probeCacheKey(data []byte, executable string) (string, error) {
	f, err := os.Open(executable)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	archiveHash := sha256.Sum256(data)
	h.Write(archiveHash[:])
	fmt.Fprintf(h, "%s\x00%s\x00%s\x00", executable, runtime.GOOS, runtime.GOARCH)
	for _, name := range []string{"EBITENGINE_GRAPHICS_LIBRARY", "EBITEN_GRAPHICS_LIBRARY", "EBITENGINE_DIRECTX", "EBITEN_DIRECTX", "EBITENGINE_DIRECTX_FEATURE_LEVEL"} {
		fmt.Fprintf(h, "%s=%s\x00", name, os.Getenv(name))
	}
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func probeProcess() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable)
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, probeEnv+"=") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, probeEnv+"=1")
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return fmt.Errorf("shader validation: %w", ctx.Err())
	}
	if err != nil {
		return fmt.Errorf("shader validation failed: %w", err)
	}
	if !strings.Contains(string(output), probeSuccess) {
		return fmt.Errorf("shader validation did not complete")
	}
	return nil
}

func exitProbe(data []byte, readErr error) {
	if readErr != nil {
		fmt.Fprintln(os.Stderr, readErr)
		os.Exit(1)
	}
	shaders, err := decodeArchive(data)
	if err == nil {
		err = validateOnGPU(shaders)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(probeSuccess)
	os.Exit(0)
}

type shaderProbe struct {
	artifacts []shaderArtifact
	programs  []*ebiten.Shader
	drawn     bool
}

func (p *shaderProbe) Layout(int, int) (int, int) { return 1, 1 }
func (p *shaderProbe) Update() error {
	if p.drawn {
		return ebiten.Termination
	}
	if p.programs != nil {
		return nil
	}
	p.programs = make([]*ebiten.Shader, 0, len(p.artifacts))
	for _, s := range p.artifacts {
		shader, err := ebiten.NewShader([]byte(s.Source))
		if err != nil {
			return err
		}
		p.programs = append(p.programs, shader)
	}
	return nil
}
func (p *shaderProbe) Draw(screen *ebiten.Image) {
	if p.drawn {
		return
	}
	source := ebiten.NewImage(1, 1)
	defer source.Deallocate()
	target := ebiten.NewImage(1, 1)
	defer target.Deallocate()
	opts := &ebiten.DrawRectShaderOptions{}
	for i := range opts.Images {
		opts.Images[i] = source
	}
	for _, shader := range p.programs {
		target.DrawRectShader(1, 1, shader, opts)
	}
	// This readback is confined to the startup child. It forces backend shader
	// creation and pipeline submission; NewShader alone only builds Kage IR.
	var pixel [4]byte
	target.ReadPixels(pixel[:])
	screen.DrawImage(target, nil)
	p.drawn = true
}
func validateOnGPU(shaders []shaderArtifact) error {
	register(shaders)
	ebiten.SetWindowVisible(false)
	ebiten.SetRunnableOnUnfocused(true)
	ebiten.SetVsyncEnabled(false)
	return ebiten.RunGame(&shaderProbe{artifacts: shaders})
}
