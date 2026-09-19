package shadercache

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

const probeEnv = "RAM_SHADER_PROBE"
const probeSuccess = "RaysAndMagic shader validation complete"

func isProbe() bool { return os.Getenv(probeEnv) == "1" }

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
