package game

import (
	"log"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"

	"ugataima/internal/shaders"
)

// shaderFailures remembers each program that failed to compile.
var shaderFailures = struct {
	sync.Mutex
	errs map[string]error
}{errs: map[string]error{}}

// compileShader fills *dst with the named embedded Kage program on first use.
// A program that fails to compile is logged once and never retried: its
// callers skip their draw at no cost per frame.
func compileShader(dst **ebiten.Shader, name string) error {
	if *dst != nil {
		return nil
	}
	shaderFailures.Lock()
	defer shaderFailures.Unlock()
	if err := shaderFailures.errs[name]; err != nil {
		return err
	}
	shader, err := ebiten.NewShader([]byte(shaders.Source(name)))
	if err != nil {
		shaderFailures.errs[name] = err
		log.Printf("shader %s does not compile; its effect is skipped: %v", name, err)
		return err
	}
	*dst = shader
	return nil
}
