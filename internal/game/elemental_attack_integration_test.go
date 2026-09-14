package game

import (
	"fmt"
	"strings"
	"testing"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/monster"
)

func TestElementalCommitClocksAndInvalidActions(t *testing.T) {
	for _, cadence := range []monsterAttackCadence{monsterAttackRealtime, monsterAttackTurn, monsterAttackPounce} {
		for _, state := range []string{"valid", "dead", "stunned", "idol", "dead_target"} {
			t.Run(fmt.Sprintf("%d/%s", cadence, state), func(t *testing.T) {
				cs := newTestCombatSystemWithConfig(t)
				elementalTestBiome(t, cs, "earth")
				g := cs.game
				g.party.Members = g.party.Members[:1]
				ch := g.party.Members[0]
				isolateTrueDamageMember(ch, 0)
				ch.HitPoints, ch.MaxHitPoints = 10000, 10000
				m := mkTestMonster("Swing", 1000)
				m.X, m.Y = g.camera.X+g.config.GetTileSize(), g.camera.Y
				m.AttackRadius = g.config.GetTileSize()
				m.DamageMin, m.DamageMax = 20, 20
				m.AttacksPerRound = 3
				m.State = monster.StateAttacking
				m.StateTimer = 1
				m.IsEngagingPlayer = true
				m.WasAttacked = true
				g.world.Monsters = []*monster.Monster3D{m}
				if state == "dead" {
					m.HitPoints = 0
				}
				if state == "stunned" {
					m.StunFramesRemaining = 100
					m.StunTurnsRemaining = 2
				}
				if state == "idol" {
					m.WarlordIdol = true
				}
				if state == "dead_target" {
					ch.HitPoints = 0
				}
				rolls := 0
				cs.elementalAttackRoll = func() float64 { rolls++; return 0 }
				committed := cs.commitMonsterAttack(m, monsterAttackDestination{}, cadence)
				want := 0
				if state == "valid" {
					want = 1
					if cadence == monsterAttackTurn {
						want = m.GetTurnBasedAttackCount()
					}
				}
				if committed != (want > 0) || rolls != want {
					t.Fatalf("committed=%t rolls=%d want %d", committed, rolls, want)
				}
				if state == "valid" && ch.HitPoints != 10000-want*40 {
					t.Fatalf("damage=%d want %d", 10000-ch.HitPoints, want*40)
				}
				if cadence == monsterAttackRealtime && state == "valid" {
					if cs.commitMonsterAttack(m, monsterAttackDestination{}, cadence) || rolls != want {
						t.Fatal("cooldown retried elemental roll")
					}
				}
			})
		}
	}
}

func TestElementalSpecialsAndChampionsStayExcluded(t *testing.T) {
	for _, special := range []string{"breath", "fireburst", "piercing", "inferno"} {
		t.Run(special, func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			elementalTestBiome(t, cs, "earth")
			cs.elementalAttackRoll = func() float64 { t.Fatal("special action rolled melee proc"); return 0 }
			m := mkTestMonster("Special", 1000)
			m.DamageMin, m.DamageMax = 10, 10
			switch special {
			case "breath":
				m.DragonBreathChance = 1
				m.DragonBreathDamageType = "fire"
				cs.applyMonsterMeleeDamage(m)
			case "fireburst":
				m.FireburstChance = 1
				cs.applyMonsterMeleeDamage(m)
			case "piercing":
				m.PiercingShotChance = 1
				cs.applyMonsterMeleeDamage(m)
			case "inferno":
				m.InfernoDamage = 20
				cs.applyMonsterInferno(m)
			}
			if len(cs.game.elementalAttackEffects) > 0 {
				t.Fatal("special action acquired elemental FX")
			}
		})
	}
	for _, key := range []string{"weapon_master", "hobbit_archer", "dark_elf_sorceress", "wild_druid"} {
		for _, tier := range []string{"easy", "normal", "impossible"} {
			t.Run(key+"/"+tier, func(t *testing.T) {
				cs := newTestCombatSystemWithConfig(t)
				primeTestChampions(t, cs.game)
				elementalTestBiome(t, cs, "light")
				m := monster.NewMonster3DFromConfig(cs.game.camera.X+cs.game.config.GetTileSize(), cs.game.camera.Y, key, cs.game.config)
				m.ChampionTier = tier
				cs.game.mirrorChampionStats(m)
				for _, c := range cs.game.party.Members {
					c.HitPoints, c.MaxHitPoints = 100000, 100000
				}
				cs.elementalAttackRoll = func() float64 { t.Fatal("champion consumed elemental RNG"); return 0 }
				before := m.ProjectileWeapon
				cs.performMonsterAttackAgainstParty(m)
				if m.ProjectileWeapon != before || len(cs.game.elementalAttackEffects) > 0 {
					t.Fatal("champion changed profile or emitted elemental FX")
				}
				for _, line := range cs.game.MonsterCombatEffectLines(m) {
					if strings.Contains(line.Text, "Elemental Attack") {
						t.Fatal("champion acquired proc tooltip")
					}
				}
			})
		}
	}
}

func TestElementalBiomeContextAndSaveRoundTrip(t *testing.T) {
	t.Chdir("../..")
	g, wm, cfg := bootOpenWorldGame(t, true)
	fx, fy, ok := wm.OpenWorldRegionStart("forest")
	if !ok {
		t.Fatal("forest start")
	}
	hx, hy, ok := wm.OpenWorldRegionStart("highlands")
	if !ok {
		t.Fatal("highlands start")
	}
	g.camera.X, g.camera.Y = fx, fy
	g.syncOpenWorldRegion()
	m := monster.NewMonster3DFromConfig(hx, hy, "goblin", cfg)
	if got := g.monsterEffectContext(m).ElementalSchool; got != wm.Biomes["highlands"].ElementalAttackSchool {
		t.Fatalf("attacker used party region: %s", got)
	}
	m.X, m.Y = fx, fy
	if got := g.monsterEffectContext(m).ElementalSchool; got != wm.Biomes["forest"].ElementalAttackSchool {
		t.Fatalf("crossed region remained stale: %s", got)
	}
	// Every standalone map resolves its authored biome, including cities/dungeons.
	oldWorld := g.world
	g.world = newTestWorldSized(cfg, 4, 4)
	oldKey := wm.CurrentMapKey
	seen := map[string]bool{}
	for key, mc := range wm.MapConfigs {
		wm.CurrentMapKey = key
		ctx := g.monsterEffectContext(m)
		seen[mc.Biome] = true
		if ctx.ElementalSchool == "" || ctx.ElementalSchool != wm.Biomes[mc.Biome].ElementalAttackSchool {
			t.Fatalf("map %s unresolved: %+v", key, ctx)
		}
	}
	if len(seen) != 19 {
		t.Fatalf("biome coverage=%d", len(seen))
	}
	g.world = oldWorld
	wm.CurrentMapKey = oldKey
	g.world.Monsters = append(g.world.Monsters, m)
	save := g.buildSave(wm)
	cfg.MonsterCombat.ElementalAttack = config.ElementalAttackConfig{Chance: .37, DamageMultiplier: 3}
	g.addMonsterElementalAttackFX(m, "earth")
	if err := g.applySave(wm, &save); err != nil {
		t.Fatal(err)
	}
	if len(g.elementalAttackEffects) != 0 {
		t.Fatal("save restored transient FX")
	}
	found := false
	for _, restored := range g.world.Monsters {
		if restored.ID != m.ID {
			continue
		}
		found = true
		ctx := g.monsterEffectContext(restored)
		p := restored.MeleeProfile(ctx.ElementalAttack, ctx.ElementalSchool)
		if p.School != "physical" || p.ElementalAttack.Chance != .37 || p.ElementalAttack.DamageMultiplier != 3 || p.ElementalSchool != "earth" {
			t.Fatalf("save restored stale profile: %+v", p)
		}
	}
	if !found {
		t.Fatal("saved test monster missing")
	}
}

func TestElementalFXLifetimeBoundAndSnapshot(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.config.Graphics.ElementalAttack.MaxActive = 2
	member := &character.MMCharacter{}
	for i := 0; i < 5; i++ {
		g.addElementalAttackFX(float64(i), 0, member, "water")
	}
	if len(g.elementalAttackEffects) != 2 || g.elementalAttackEffects[0].X != 3 {
		t.Fatal("effect cap failed")
	}
	g.config.Graphics.ElementalAttack.MaxActive = 0
	g.addMonsterElementalAttackFX(mkTestMonster("Disabled", 1), "earth")
	if g.elementalAttackEffects[1].MonsterTarget != nil {
		t.Fatal("disabled FX retargeted an existing effect")
	}
	g.config.Graphics.ElementalAttack.MaxActive = 2
	maxAge := g.elementalAttackEffects[0].Frames
	g.config.Graphics.ElementalAttack.DurationSeconds = 99
	for i := 0; i < maxAge; i++ {
		g.tickElementalAttackFX()
	}
	if len(g.elementalAttackEffects) != 0 {
		t.Fatal("effect retained after snapshotted lifetime")
	}
	g.addElementalAttackFX(0, 0, member, "water")
	g.clearTransientCombatState()
	if len(g.elementalAttackEffects) != 0 {
		t.Fatal("map reset retained effects")
	}
}
