package game

import (
	"testing"

	"ugataima/internal/config"
	monsterPkg "ugataima/internal/monster"
)

// applyAoeSplash hits every living monster inside the radius once and nothing
// else: never the primary again, never a corpse, nobody when the radius is 0.
// A skipped victim stays untouched in HP, hit flash, combat log and rewards.
func TestAoESplash_VictimSelection(t *testing.T) {
	type victim struct {
		name    string
		dx, dy  float64 // tiles from the primary
		hp      int     // 0 = a corpse
		wantHit bool
	}
	for _, tc := range []struct {
		name    string
		radius  float64
		victims []victim
	}{
		{name: "inside_and_outside", radius: 2, victims: []victim{
			{name: "Near", dx: 1, hp: 100, wantHit: true},
			{name: "Edge", dy: 1.9, hp: 100, wantHit: true},
			{name: "Far", dx: 3, hp: 100},
		}},
		{name: "dead_bystander", radius: 2, victims: []victim{
			{name: "Corpse", dx: 1},
			{name: "Living", dy: 1, hp: 100, wantHit: true},
		}},
		{name: "zero_radius", radius: 0, victims: []victim{
			{name: "Bystander", dx: 0.5, hp: 100},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			g := cs.game
			ts := float64(g.config.GetTileSize())
			primary := &monsterPkg.Monster3D{Name: "Primary", HitPoints: 100, MaxHitPoints: 100}
			g.world.Monsters = []*monsterPkg.Monster3D{primary}
			mobs := make([]*monsterPkg.Monster3D, len(tc.victims))
			for i, v := range tc.victims {
				mobs[i] = &monsterPkg.Monster3D{Name: v.name, X: v.dx * ts, Y: v.dy * ts, HitPoints: v.hp, MaxHitPoints: 100}
				g.world.Monsters = append(g.world.Monsters, mobs[i])
			}
			xpBefore, goldBefore, logBefore := g.totalExperienceEarned, g.party.Gold, g.combatLogVersion

			const damage = 20
			attack := cs.newPartyMonsterAttack(damage, 0, "fire", 0, nil, "Fireball", false, true, false)
			cs.applyAoeSplash(primary, attack, tc.radius)

			if primary.HitPoints != primary.MaxHitPoints || primary.HitTintFrames != 0 {
				t.Errorf("primary re-damaged by its own splash: HP %d", primary.HitPoints)
			}
			hits := 0
			for i, v := range tc.victims {
				m := mobs[i]
				want := v.hp
				if v.wantHit {
					want -= damage
					hits++
				}
				if m.HitPoints != want || (m.HitTintFrames > 0) != v.wantHit {
					t.Errorf("%s: HP %d flash %d, want HP %d hit=%v", v.name, m.HitPoints, m.HitTintFrames, want, v.wantHit)
				}
			}
			if got := g.combatLogVersion - logBefore; got != hits {
				t.Errorf("combat log grew by %d lines, want one per hit victim (%d)", got, hits)
			}
			if g.totalExperienceEarned != xpBefore || g.party.Gold != goldBefore || len(g.deadMonsterIDs) != 0 {
				t.Error("splash on survivors and corpses paid rewards or queued a kill")
			}
		})
	}
}

// The melee AoE weapon splashes through the real melee damage path at its
// authored radius: just inside is hit, just outside is not.
func TestMeleeWeaponAoE_UsesWeaponRadiusOnRealMeleeDamagePath(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	tileSize := float64(cs.game.config.GetTileSize())
	def, ok := config.GetWeaponDefinition("tonbogiri")
	if !ok || def == nil || def.AoeRadiusTiles <= 0 {
		t.Fatal("tonbogiri missing or authors no AoE radius")
	}

	primary := &monsterPkg.Monster3D{
		ID: "primary", Name: "Primary", X: 0, Y: 0,
		HitPoints: 100, MaxHitPoints: 100, ArmorClass: 0,
	}
	near := &monsterPkg.Monster3D{
		ID: "near", Name: "Near", X: tileSize * (def.AoeRadiusTiles - 0.1), Y: 0,
		HitPoints: 100, MaxHitPoints: 100, ArmorClass: 0,
	}
	far := &monsterPkg.Monster3D{
		ID: "far", Name: "Far", X: tileSize * (def.AoeRadiusTiles + 0.1), Y: 0,
		HitPoints: 100, MaxHitPoints: 100, ArmorClass: 0,
	}
	cs.game.world.Monsters = []*monsterPkg.Monster3D{primary, near, far}

	cs.ApplyDamageToMonster(primary, 20, def.Name, false)

	if primary.HitPoints != 80 {
		t.Fatalf("primary HP = %d, want 80", primary.HitPoints)
	}
	if near.HitPoints != 80 {
		t.Fatalf("near monster should take %s splash, HP = %d, want 80", def.Name, near.HitPoints)
	}
	if far.HitPoints != 100 {
		t.Fatalf("far monster should be outside %s splash, HP = %d, want 100", def.Name, far.HitPoints)
	}
}
