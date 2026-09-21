package game

import (
	"sort"
	"strings"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/graphics"
	"ugataima/internal/spells"
)

func statusHUDCatalog() []string {
	var keys []string
	for key, def := range config.GlobalSpells.Spells {
		if def.StatusIcon != "" {
			keys = append(keys, key)
		}
	}
	for key, def := range config.GlobalItems.Items {
		if def.StatusIcon != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

// All authored spell and consumable statuses share the compact HUD policy.
// Both first activation and reconstruction from active effects resolve dedicated
// status art; inventory migration metadata must never decorate those sources.
func TestStatusHUDCatalog(t *testing.T) {
	cfg := loadTestConfig(t)
	t.Chdir("../..")
	old := config.GlobalIconFrames
	t.Cleanup(func() { config.GlobalIconFrames = old })
	if err := config.LoadIconFrames("assets/icon_frames.yaml"); err != nil {
		t.Fatal(err)
	}
	g := &MMGame{config: cfg, sprites: graphics.NewSpriteManager()}
	for _, key := range statusHUDCatalog() {
		for _, restored := range []bool{false, true} {
			t.Run(key+"/restored="+map[bool]string{false: "false", true: "true"}[restored], func(t *testing.T) {
				g.utilitySpellStatuses = nil
				if restored {
					g.updateUtilityStatus(spells.SpellID(key), 120, true)
				} else {
					g.setUtilityStatus(spells.SpellID(key), 120)
				}
				status := g.utilitySpellStatuses[spells.SpellID(key)]
				if status == nil {
					t.Fatal("no active status")
				}
				if !strings.HasPrefix(status.Icon, "status_") || !g.sprites.HasSprite(status.Icon) {
					t.Fatalf("%s uses inventory/book art or missing HUD source: %s", key, status.Icon)
				}
				if config.IsUnframedIcon(status.Icon) {
					t.Fatalf("%s applies a decorative content frame", key)
				}
				if status.Duration != 120 || status.MaxDuration != 120 {
					t.Fatalf("duration changed: %+v", status)
				}
			})
		}
	}
}
