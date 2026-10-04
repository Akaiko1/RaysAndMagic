package game

import (
	"testing"

	"ugataima/internal/graphics"
	"ugataima/internal/monster"
)

// Dedicated variants must resolve all animation states through the runtime
// loader, not fall back to a base monster or to a static placeholder.
func TestDedicatedMonsterSpriteFamilies(t *testing.T) {
	t.Chdir("../..")
	previous := monster.MonsterConfig
	t.Cleanup(func() { monster.MonsterConfig = previous })
	catalog, err := monster.LoadMonsterConfig("assets/monsters.yaml")
	if err != nil {
		t.Fatal(err)
	}
	sprites := graphics.NewSpriteManager()
	for _, tc := range []struct {
		key, direction string
	}{
		{"dragon_brood_mother", "r"},
		{"alien_enforcer", "l"},
		{"ancient_god_of_death", "r"},
		{"elder_dragon", "l"},
		{"elder_dragon_red", "r"},
		{"elder_dragon_green", "r"},
		{"elder_dragon_gold", "r"},
		{"ronin_marksman", "r"},
		{"vengeful_ningyo", "r"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			def, err := catalog.GetMonsterByKey(tc.key)
			if err != nil {
				t.Fatal(err)
			}
			if def.Sprite != tc.key {
				t.Fatalf("sprite = %q; want dedicated family %q", def.Sprite, tc.key)
			}
			if !sprites.HasSprite(def.Sprite) {
				t.Fatal("dedicated static sprite is missing")
			}
			static := sprites.GetSprite(def.Sprite)
			if static == nil {
				t.Fatal("dedicated static sprite did not load")
			}
			defer sprites.EvictResource(def.Sprite, "")
			for _, state := range []string{"walking", "attacking", "dying"} {
				name := state + "_" + tc.direction
				animation := sprites.GetAnimation(def.Sprite, name)
				if animation == nil || len(animation.Frames) != 4 {
					t.Fatalf("%s must load four frames", name)
				}
				defer sprites.EvictResource(def.Sprite, name)
				for i, frame := range animation.Frames {
					if frame == nil || frame.Bounds().Size() != static.Bounds().Size() {
						t.Fatalf("%s frame %d does not match the static frame size", name, i)
					}
				}
			}
		})
	}
}
