package game

import (
	"testing"
)

func TestGenerateProjectileID(t *testing.T) {
	g := &MMGame{}
	id := g.GenerateProjectileID("arrow")
	if id == "" {
		t.Error("GenerateProjectileID should return a non-empty string")
	}
}
