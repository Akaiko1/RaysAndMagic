//go:build debug

package game

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/spells"
	"ugataima/internal/storage"
)

func TestDebugSim_AllTooltipGroupingGallery(t *testing.T) {
	requireStandeeGPU(t)
	out := os.Getenv("RAM_TOOLTIP_GROUPING_OUTPUT")
	if out == "" {
		t.Skip("set RAM_TOOLTIP_GROUPING_OUTPUT for gallery output")
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	storage.SetDataRootForTesting(t.TempDir())
	defer storage.SetDataRootForTesting("")
	g, _ := bootFxGalleryGame(t)
	defer g.Shutdown()
	ch := gmReferenceChar(g.config)
	g.party.Members[0] = ch
	ui := g.gameLoop.ui
	catalog := map[string]string{}
	for _, full := range []bool{false, true} {
		for _, base := range []bool{false, true} {
			bearer := ch
			if base {
				bearer = nil
			}
			add := func(key, text string) { catalog[fmt.Sprintf("%s/full%v/base%v", key, full, base)] = text }
			for key := range config.GlobalWeapons.Weapons {
				add("weapon/"+key, GetItemTooltip(items.CreateWeaponFromYAML(key), bearer, g.combat, full))
			}
			for key := range config.GlobalItems.Items {
				add("item/"+key, GetItemTooltip(items.CreateItemFromYAML(key), bearer, g.combat, full))
			}
			for key := range config.GlobalSpells.Spells {
				add("spell/"+key, GetSpellTooltip(spells.SpellID(key), bearer, g.combat, full))
			}
			for _, key := range config.TrapKeysOrdered() {
				def, _ := config.GetTrapDefinition(key)
				add("trap/"+key, buildTrapTooltipUnified(key, def, bearer, g.combat, full))
			}
		}
	}
	for _, skill := range character.AllSkills {
		catalog["skill/"+skill.String()] = masteryTooltipTextForSkill(skill)
	}
	for _, school := range character.AllMagicSchools {
		catalog["school/"+school.DisplayName()] = magicMasteryTooltipText(school)
	}
	data, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "catalog.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct{ kind, key string }{
		{"spell", "fireball"}, {"spell", "firewall"}, {"spell", "heal_other"}, {"spell", "hour_of_power"},
		{"item", "golden_armor"}, {"item", "health_potion"}, {"item", "onmyoji_talisman"}, {"item", "troll_card"},
		{"trap", config.TrapKeysOrdered()[0]}, {"skill", "Overwatch"}, {"school", "Fire"}, {"stat", "Speed"},
	} {
		for _, full := range []bool{false, true} {
			for _, size := range [][2]int{{800, 600}, {1024, 768}} {
				var text string
				var shot *image.RGBA
				runOnDrawFrame(func(*ebiten.Image) {
					dst := ebiten.NewImage(size[0], size[1])
					defer dst.Deallocate()
					dst.Fill(color.RGBA{18, 20, 25, 255})
					ui.tooltipCompareLines = nil
					switch fixture.kind {
					case "spell":
						id := spells.SpellID(fixture.key)
						text = GetSpellTooltip(id, ch, g.combat, full)
						def, _ := spells.GetSpellDefinitionByID(id)
						ui.queueTitledTooltipIcon(strings.Split(text, "\n"), nil, schoolPlateColor(def.School), nil, spellTooltipIconName(id), 16, 16)
					case "item":
						it := items.CreateItemFromYAML(fixture.key)
						text = GetItemTooltip(it, ch, g.combat, full)
						ui.queueItemTooltip(strings.Split(text, "\n"), it, ch, 16, 16)
					case "trap":
						def, _ := config.GetTrapDefinition(fixture.key)
						text = buildTrapTooltipUnified(fixture.key, def, ch, g.combat, full)
						ui.queueTitledTooltipIcon(strings.Split(text, "\n"), nil, woodPlateColor, nil, def.Icon, 16, 16)
					case "skill":
						text = masteryTooltipTextForSkill(character.SkillOverwatch)
						ui.queueTooltip(strings.Split(text, "\n"), 16, 16)
					case "school":
						text = magicMasteryTooltipText(character.MagicSchoolFire)
						ui.queueTooltip(strings.Split(text, "\n"), 16, 16)
					case "stat":
						text = statTooltipText(fixture.key)
						ui.queueTooltip(strings.Split(text, "\n"), 16, 16)
					}
					ui.drawQueuedTooltips(dst)
					shot = snapshotUIImage(dst)
				})
				name := fmt.Sprintf("%s-%s-full%v-%dx%d", fixture.kind, fixture.key, full, size[0], size[1])
				f, err := os.Create(filepath.Join(out, name+".png"))
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(f, shot)
				closeErr := f.Close()
				if err != nil || closeErr != nil {
					t.Fatalf("write PNG: %v; close: %v", err, closeErr)
				}
				if err := os.WriteFile(filepath.Join(out, name+".txt"), []byte(text), 0644); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}
