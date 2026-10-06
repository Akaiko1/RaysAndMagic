package game

import (
	"encoding/json"
	"fmt"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

// Every party buff family crosses RT, TB idle/initial/resolved turns, mode
// switching and a serialized remainder. Legacy CombatClock=false is deliberate.
func TestPartyBuffModeClockAndSave(t *testing.T) {
	for _, kind := range []string{"heroism", "flame_ward_draught", "solstice_fire_advanced_ward", "bless", "torch_light", "wizard_eye", "walk_on_water", "fly", "water_breathing"} {
		t.Run(kind, func(t *testing.T) {
			cfg := loadTestConfig(t)
			g := newTestGame(cfg, newTestWorldSized(cfg, 12, 12))
			setTestWorldManager(t, nil)
			g.combat = NewCombatSystem(g)
			gl := &GameLoop{game: g}
			g.gameLoop = gl
			duration := TurnBasedPeriodicEffectSeconds*cfg.GetTPS() + 17
			expired := 0
			switch kind {
			case "heroism":
				g.addCombatBuff(TimedCombatBuff{SpellID: kind, Frames: duration, OutBonus: 10})
			case "flame_ward_draught", "solstice_fire_advanced_ward":
				def, _ := config.GetItemDefinition(kind)
				b, ok := timedCombatBuffFromItem(kind, def, duration)
				if !ok {
					t.Fatal("draught unavailable")
				}
				g.addCombatBuff(b)
			case "bless":
				g.addStatBuff(TimedStatBuff{SpellID: kind, Frames: duration, Bonuses: character.UniformStatBonuses(3)})
			default:
				gl.timedBuffRegistry = g.buildTimedBuffs()
				for i := range gl.timedBuffRegistry {
					b := &gl.timedBuffRegistry[i]
					if string(b.id) == kind {
						original := b.onExpire
						b.onExpire = func() {
							expired++
							if original != nil {
								original()
							}
						}
					}
				}
				if g.activateTimedBuffFrames(spells.SpellID(kind), duration, false) != timedBuffApplied {
					t.Fatal("utility unavailable")
				}
			}
			left := func() int {
				if b, ok := g.combatBuffByID(kind); ok {
					return b.Frames
				}
				if b, ok := g.statBuffByID(kind); ok {
					return b.Frames
				}
				for _, b := range g.timedBuffs() {
					if string(b.id) == kind && *b.active {
						return *b.duration
					}
				}
				return 0
			}
			check := func(want int) {
				t.Helper()
				if got := left(); got != want {
					t.Fatalf("remaining %d, want %d", got, want)
				}
				if want > 0 {
					s := g.utilitySpellStatuses[spells.SpellID(kind)]
					if s == nil || s.Duration != want {
						t.Fatal("HUD does not match timer")
					}
				}
			}
			gl.updateSpecialEffects()
			check(duration - 1)
			g.ToggleTurnBasedMode()
			check(duration - 1)
			for range 20 {
				gl.updateSpecialEffects()
			}
			check(duration - 1)
			g.startPartyTurn()
			check(16)
			wm := world.NewWorldManager(cfg)
			wm.CurrentMapKey = "clock_fixture"
			wm.LoadedMaps = map[string]*world.World3D{"clock_fixture": g.world}
			wm.MapConfigs = map[string]*config.MapConfig{"clock_fixture": {SkyTexture: "forest_panorama"}}
			setTestWorldManager(t, wm)
			t.Chdir("../..")
			save := g.buildSave(wm)
			raw, err := json.Marshal(save)
			if err != nil {
				t.Fatal(err)
			}
			var decoded GameSave
			if err := json.Unmarshal(raw, &decoded); err != nil {
				t.Fatal(err)
			}
			g.restoreSavedEffects(&decoded)
			check(16)
			g.ToggleTurnBasedMode()
			check(16)
			for range 16 {
				gl.updateSpecialEffects()
			}
			check(0)
			for range 4 {
				gl.updateSpecialEffects()
			}
			if kind == "bless" && g.statBonuses != (character.StatBonuses{}) {
				t.Fatal("expired stat bonus remains")
			}
			if _, ok := g.utilitySpellStatuses[spells.SpellID(kind)]; ok {
				t.Fatal("expired HUD remains")
			}
			if kind != "heroism" && kind != "flame_ward_draught" && kind != "solstice_fire_advanced_ward" && kind != "bless" && expired != 1 {
				t.Fatalf("expiry callbacks=%d", expired)
			}
		})
	}
}

func TestPartyBuffExpiresOnResolvedRound(t *testing.T) {
	for _, left := range []int{1, 17, 180} {
		t.Run(fmt.Sprint(left), func(t *testing.T) {
			g, gl, _ := tbBehaviorGame(t, 12, 12)
			g.gameLoop = gl
			g.addCombatBuff(TimedCombatBuff{SpellID: "heroism", Frames: left, OutBonus: 4})
			g.addStatBuff(TimedStatBuff{SpellID: "bless", Frames: left, Bonuses: character.UniformStatBonuses(4)})
			g.torchLightActive, g.torchLightDuration = true, left
			gl.updateSpecialEffects()
			if len(g.combatBuffs) != 1 || g.torchLightDuration != left {
				t.Fatal("thinking drained a buff")
			}
			g.startPartyTurn()
			if len(g.combatBuffs) != 0 || len(g.statBuffs) != 0 || g.torchLightActive || g.torchLightDuration != 0 {
				t.Fatal("round failed to expire all buff families")
			}
		})
	}
}
