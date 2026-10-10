package game

import (
	"fmt"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/collision"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/spells"
	"ugataima/internal/threading/entities"
	"ugataima/internal/world"
)

// A party blast spell bursts wherever its shot ends: on the target it hits,
// around a target that dodges or is sealed, against a wall, and at the end
// of its range. Firebolt has no blast and is the control row.
func TestSpellBlastBurstsWhereverTheShotEnds(t *testing.T) {
	for _, spell := range []string{"fireball", "deadly_swarm", "starburst", "firebolt"} {
		for _, end := range []string{"hit", "dodge", "sealed", "wall", "range"} {
			t.Run(fmt.Sprintf("%s/%s", spell, end), func(t *testing.T) {
				g := newTestCombatSystemWithConfig(t).game
				def, err := spells.GetSpellDefinitionByID(spells.SpellID(spell))
				if err != nil {
					t.Fatal(err)
				}
				ts := float64(g.config.GetTileSize())
				target := mkTestMonster("Target", 1000)
				target.ID, target.X, target.Y = "target", 5*ts, 5*ts
				near := mkTestMonster("Near", 1000)
				near.ID, near.X, near.Y = "near", 6*ts, 5*ts
				far := mkTestMonster("Far", 1000)
				far.ID, far.X, far.Y = "far", 14*ts, 5*ts
				g.world.Monsters = []*monsterPkg.Monster3D{target, near, far}
				for _, m := range g.world.Monsters {
					g.collisionSystem.RegisterEntity(collision.NewEntity(m.ID, m.X, m.Y, 16, 16, collision.CollisionTypeMonster, false))
				}
				switch end {
				case "dodge":
					target.PerfectDodge = 100
				case "sealed":
					target.BossDormant = true
				}
				shot := MagicProjectile{ID: g.GenerateProjectileID(spell), X: target.X, Y: target.Y, VelX: 1,
					Damage: 30, LifeTime: 50, Active: true, SpellType: spell, Owner: ProjectileOwnerPlayer,
					Attacker: g.party.Members[0], AoeTiles: def.AoeRadiusTiles}
				if end == "wall" || end == "range" {
					// Clear of every monster's box, next to the near one.
					shot.X, shot.Y = near.X+ts/2, near.Y+ts/2
				}
				if end == "range" {
					shot.LifeTime = 1
				}
				g.magicProjectiles = []MagicProjectile{shot}
				g.collisionSystem.RegisterEntity(collision.NewEntity(shot.ID, shot.X, shot.Y, 8, 8, collision.CollisionTypeProjectile, false))
				switch end {
				case "hit", "dodge", "sealed":
					g.combat.CheckProjectileMonsterCollisions()
				default:
					updater := entities.NewEntityUpdaterWithWorkers(1)
					defer updater.Stop()
					open := end == "range"
					updater.UpdateProjectilesParallel(g.ConvertProjectilesToWrappers(), func(float64, float64) bool { return open })
				}
				blast := def.AoeRadiusTiles > 0
				if got := near.HitPoints < 1000; got != blast {
					t.Fatalf("near monster hit=%v, want %v (hp %d)", got, blast, near.HitPoints)
				}
				if far.HitPoints != 1000 {
					t.Fatalf("far monster outside the blast was hit (hp %d)", far.HitPoints)
				}
				if end == "dodge" || end == "sealed" {
					if target.HitPoints != 1000 {
						t.Fatalf("the spared target took the blast (hp %d)", target.HitPoints)
					}
				}
				if end == "hit" && target.HitPoints >= 1000 {
					t.Fatal("direct hit dealt no damage")
				}
				if p := g.magicProjectiles[0]; p.Active && p.LifeTime > 0 {
					t.Fatal("the shot is still in flight")
				}
			})
		}
	}
}

// Resurrect lifts the eradicated first (nothing else can), then any other
// fallen ally in slot order; Raise Dead keeps its first-fallen, never-eradicated
// rule. With no one fallen the SP is kept.
type fallenMember struct {
	slot int
	cond character.Condition
}

func TestReviveSpellsPickTheirTarget(t *testing.T) {
	type state = fallenMember
	for _, tc := range []struct {
		name, spell string
		fallen      []state
		want        int // -1: nobody (SP kept)
	}{
		{"resurrect/eradicated beats unconscious", "resurrect", []state{{1, character.ConditionUnconscious}, {3, character.ConditionEradicated}}, 3},
		{"resurrect/first eradicated", "resurrect", []state{{0, character.ConditionEradicated}, {2, character.ConditionEradicated}}, 0},
		{"resurrect/unconscious only", "resurrect", []state{{2, character.ConditionUnconscious}}, 2},
		{"resurrect/dead only", "resurrect", []state{{1, character.ConditionDead}}, 1},
		{"resurrect/nobody", "resurrect", nil, -1},
		{"raise_dead/first fallen, not eradicated", "raise_dead", []state{{1, character.ConditionUnconscious}, {3, character.ConditionEradicated}}, 1},
		{"raise_dead/only eradicated", "raise_dead", []state{{3, character.ConditionEradicated}}, -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			g := cs.game
			def, err := spells.GetSpellDefinitionByID(spells.SpellID(tc.spell))
			if err != nil {
				t.Fatal(err)
			}
			for _, m := range g.party.Members {
				m.MaxHitPoints, m.HitPoints = 50, 50
			}
			for _, f := range tc.fallen {
				m := g.party.Members[f.slot]
				m.HitPoints = 0
				m.AddCondition(f.cond)
			}
			caster := g.party.Members[0]
			if tc.want == 0 || containsSlot(tc.fallen, 0) {
				caster = g.party.Members[len(g.party.Members)-1]
			}
			caster.SpellPoints = 100
			outcome := cs.castSpell(spellCastRequest{ID: def.ID, Definition: def, Caster: caster, Cost: def.SpellPointsCost})
			if tc.want < 0 {
				if outcome != castNoEffect || caster.SpellPoints != 100 {
					t.Fatalf("no target: outcome %v, SP %d", outcome, caster.SpellPoints)
				}
				return
			}
			for i, m := range g.party.Members {
				revived := m.HitPoints > 0
				wasFallen := containsSlot(tc.fallen, i)
				if wasFallen && revived != (i == tc.want) {
					t.Fatalf("slot %d revived=%v, want only slot %d", i, revived, tc.want)
				}
			}
		})
	}
}

func containsSlot(fallen []fallenMember, slot int) bool {
	for _, f := range fallen {
		if f.slot == slot {
			return true
		}
	}
	return false
}

// A heal cast from the spellbook has no pointer target: Heal Other lands on the
// most wounded ally that can be healed (the caster when nobody is hurt), while
// First Aid stays on the caster. The quick-slot F path keeps its pointer-else-
// selected-hero rule (Control Tips).
func TestBookHealTargets(t *testing.T) {
	for _, tc := range []struct {
		name, spell string
		hp          [4]int // percent of max; 0 = knocked out
		pointer     int    // F path only: card under the pointer (-1 none)
		entry       string // "book" or "f"
		want        int
	}{
		{"heal_other/book/most wounded ally", "heal_other", [4]int{80, 30, 100, 60}, -1, "book", 1},
		{"heal_other/book/skips 0 HP", "heal_other", [4]int{100, 0, 100, 50}, -1, "book", 3},
		{"heal_other/book/only the caster hurt", "heal_other", [4]int{40, 100, 100, 100}, -1, "book", 0},
		{"heal_other/f/no pointer heals the selected hero", "heal_other", [4]int{80, 30, 100, 100}, -1, "f", 0},
		{"heal_other/f/pointer wins", "heal_other", [4]int{80, 30, 90, 100}, 2, "f", 2},
		{"heal/book/self only", "heal", [4]int{80, 30, 100, 100}, -1, "book", 0},
		{"heal/f/self only", "heal", [4]int{80, 30, 100, 100}, 1, "f", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			g := cs.game
			g.turnBasedMode = false
			g.showPartyStats = true
			equipSpellAndPrepareCaster(t, cs, tc.spell, 100, 20)
			g.selectedChar = 0
			before := make([]int, len(g.party.Members))
			for i, m := range g.party.Members {
				m.MaxHitPoints, m.HitPoints = 1000, 10*tc.hp[i]
				if m.HitPoints == 0 {
					m.AddCondition(character.ConditionUnconscious)
				}
				before[i] = m.HitPoints
			}
			def, _ := spells.GetSpellDefinitionByID(spells.SpellID(tc.spell))
			switch tc.entry {
			case "book":
				if !cs.castPlayerSpell(def.ID, def, g.party.Members[0], true) {
					t.Fatal("book cast refused")
				}
			case "f":
				prev := rawCursorPosition
				defer func() { rawCursorPosition = prev }()
				x, y := -1, -1
				if tc.pointer >= 0 {
					w, h, left, top := partyPortraitLayout(g)
					x, y = left+tc.pointer*w+w/2, top+h/2
				}
				rawCursorPosition = func() (int, int) { return x, y }
				if fired, _ := NewInputHandler(g).castSlottedSpellResolved(g.party.Members[0]); !fired {
					t.Fatal("F cast refused")
				}
			}
			for i, m := range g.party.Members {
				if healed := m.HitPoints > before[i]; healed != (i == tc.want) {
					t.Fatalf("member %d healed=%v, want only member %d (hp %v -> %d)", i, healed, tc.want, before, m.HitPoints)
				}
			}
		})
	}
}

// Town Portal charges nothing until a destination is chosen: the cast only
// opens the picker, cancelling costs nothing, and the confirm pays the SP,
// the action and the cooldown before the party goes.
func TestTownPortalChargesOnConfirm(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, entry := range []string{"quick", "book"} {
			for _, choice := range []string{"cancel", "confirm"} {
				t.Run(fmt.Sprintf("tb=%v/%s/%s", tb, entry, choice), func(t *testing.T) {
					g, wm, _ := travelFixture(t)
					g.turnBasedMode = tb
					equipSpellAndPrepareCaster(t, g.combat, "town_portal", 100, 20)
					g.selectedChar = 0
					g.visitedTavernMaps = map[string]bool{"other": true}
					caster := g.party.Members[0]
					caster.RTCooldown, caster.ActionsRemaining = 0, 1
					g.currentTurn = 0
					ih := NewInputHandler(g)
					switch entry {
					case "quick":
						ih.castSlottedSpell(caster)
					case "book":
						g.menuOpen, g.currentTab = true, TabSpellbook
						schools := spellbookSchoolsWithSpells(caster)
						for i, s := range schools {
							for j, id := range caster.GetSpellsForSchool(s) {
								if id == "town_portal" {
									g.selectedSchool, g.selectedSpell = i, j
								}
							}
						}
						ih.useSelectedBookEntryFromHub()
					}
					if !g.townPortalPickerOpen || caster.SpellPoints != 100 || caster.RTCooldown != 0 || caster.ActionsRemaining != 1 {
						t.Fatalf("opening the picker charged: open=%v sp=%d cd=%d actions=%d", g.townPortalPickerOpen, caster.SpellPoints, caster.RTCooldown, caster.ActionsRemaining)
					}
					def, _ := spells.GetSpellDefinitionByID("town_portal")
					if choice == "cancel" {
						g.cancelTownPortalPicker()
						if g.townPortalPickerOpen || caster.SpellPoints != 100 || caster.RTCooldown != 0 || caster.ActionsRemaining != 1 || wm.CurrentMapKey != "forest" {
							t.Fatal("cancel charged or moved the party")
						}
						return
					}
					g.confirmTownPortal("other")
					if caster.SpellPoints != 100-g.combat.effectiveSpellCost(caster, def.SpellPointsCost) {
						t.Fatalf("confirm paid %d SP", 100-caster.SpellPoints)
					}
					if tb && caster.ActionsRemaining != 0 {
						t.Fatalf("TB confirm left %d actions", caster.ActionsRemaining)
					}
					if !tb && caster.RTCooldown <= 0 {
						t.Fatal("RT confirm set no cooldown")
					}
					if wm.CurrentMapKey != "other" || g.townPortalPickerOpen {
						t.Fatalf("confirm did not travel: map %s", wm.CurrentMapKey)
					}
				})
			}
		}
	}
}

// The dive records where to surface; recasting Water Breathing in the depths
// keeps that point, also across a save, and the party surfaces there when the
// spell ends.
func TestWaterBreathingRecastKeepsTheSurfacePoint(t *testing.T) {
	for _, saved := range []bool{false, true} {
		t.Run(fmt.Sprintf("saved=%v", saved), func(t *testing.T) {
			g, wm, _ := travelFixture(t)
			g.gameLoop = &GameLoop{game: g}
			def, err := spells.GetSpellDefinitionByID("water_breathing")
			if err != nil {
				t.Fatal(err)
			}
			caster := g.party.Members[0]
			cast := func() {
				if !g.combat.castResolvedSpell(def.ID, def, caster, 0, false, false) {
					t.Fatal("cast refused")
				}
			}
			cast()
			g.world.Tiles[10][8] = world.TileDeepWater
			NewInputHandler(g).checkDeepWater()
			if wm.CurrentMapKey != "water" || g.underwaterReturnMap != "forest" {
				t.Fatalf("dive failed: map %s return %s", wm.CurrentMapKey, g.underwaterReturnMap)
			}
			surfaceX, surfaceY := g.underwaterReturnX, g.underwaterReturnY
			cast()
			if saved {
				save := g.buildSave(wm)
				if err := g.applySave(wm, &save); err != nil {
					t.Fatal(err)
				}
			}
			if g.underwaterReturnMap != "forest" || g.underwaterReturnX != surfaceX || g.underwaterReturnY != surfaceY {
				t.Fatalf("recast moved the surface point to %s (%v,%v)", g.underwaterReturnMap, g.underwaterReturnX, g.underwaterReturnY)
			}
			g.advanceUtilityBuffs(g.waterBreathingDuration + 1)
			if wm.CurrentMapKey != "forest" || g.camera.X != surfaceX || g.camera.Y != surfaceY {
				t.Fatalf("spell ended on %s at (%v,%v), want forest (%v,%v)", wm.CurrentMapKey, g.camera.X, g.camera.Y, surfaceX, surfaceY)
			}
		})
	}
}

// Zone ticks are ordinary spell damage: target armor cuts them exactly as it
// cuts a projectile of the same school, in both modes.
func TestZoneTicksMeetArmorLikeProjectiles(t *testing.T) {
	for _, spell := range []string{"firewall", "hot_steam"} {
		for _, tb := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/tb=%v", spell, tb), func(t *testing.T) {
				g := newTestCombatSystemWithConfig(t).game
				g.turnBasedMode = tb
				ts := float64(g.config.GetTileSize())
				bare, armored, shot := mkTestMonster("Bare", 10000), mkTestMonster("Armored", 10000), mkTestMonster("Shot", 10000)
				bare.ID, armored.ID, shot.ID = "bare", "armored", "shot"
				bare.X, bare.Y = 5*ts, 5*ts
				armored.X, armored.Y = 5*ts, 5*ts
				shot.X, shot.Y = 20*ts, 20*ts
				armored.ArmorClass, shot.ArmorClass = 100, 100
				g.world.Monsters = []*monsterPkg.Monster3D{bare, armored, shot}
				z := &PersistentDamageZone{SpellID: spell, FieldID: 1, X: 5 * ts, Y: 5 * ts, Radius: 2 * ts, FramesLeft: 60, TickDamage: 100}
				g.combat.damageZoneMonsters(spell, []*PersistentDamageZone{z}, []*PersistentDamageZone{z})
				bolt := &MagicProjectile{ID: "bolt", Active: true, LifeTime: 60, Damage: 100, SpellType: spell, Attacker: g.party.Members[0]}
				g.combat.applyProjectileDamage(bolt, "magic_projectile", shot, bolt.ID)
				if got := 10000 - bare.HitPoints; got != 100 {
					t.Fatalf("unarmored tick %d, want 100", got)
				}
				zone, projectile := 10000-armored.HitPoints, 10000-shot.HitPoints
				if zone >= 100 || zone != projectile {
					t.Fatalf("armored zone tick %d, projectile %d: armor must cut both alike", zone, projectile)
				}
			})
		}
	}
}

// The Quickening stage appears in a cooldown breakdown only while the buff is
// up and actually shortens the cooldown.
func TestCooldownShowsQuickeningOnlyWhileActive(t *testing.T) {
	for _, active := range []bool{false, true} {
		t.Run(fmt.Sprintf("active=%v", active), func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			char := cs.game.party.Members[0]
			if active {
				cs.game.addCombatBuff(TimedCombatBuff{SpellID: "quickening", Frames: 600, RecoveryPct: 20})
			}
			card := GetSpellTooltipRows("fireball", char, cs, true).String()
			if got := strings.Contains(card, "Quickening: -20% recovery"); got != active {
				t.Fatalf("Quickening line shown=%v with the buff active=%v:\n%s", got, active, card)
			}
			if strings.Contains(card, "Cooldown limit") {
				t.Fatalf("an unclamped cooldown shows the limit stage:\n%s", card)
			}
		})
	}
}
