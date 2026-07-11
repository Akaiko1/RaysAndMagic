package game

import (
	"sort"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

// TestEverySpell_CastsAndApplies casts EVERY spell in spells.yaml through the
// real CastEquippedSpell path on a prepared scene and asserts a class-correct
// effect. It guarantees no spell silently fails to cast / mis-dispatches, and
// that each effect family (damage projectile, heal, party-heal, revive, awaken,
// AoE-stun, zone, party-nova, buff, utility) actually fires. Projectile on-hit
// riders (charm/disintegrate/psychic-shock) are covered by asserting the
// projectile spawns - the hit itself resolves on collision elsewhere.
func TestEverySpell_CastsAndApplies(t *testing.T) {
	// tbBehaviorGame loads the YAML configs (populating config.GlobalSpells).
	tbBehaviorGame(t, 5, 5)
	if config.GlobalSpells == nil || len(config.GlobalSpells.Spells) == 0 {
		t.Fatal("spell config not loaded")
	}
	keys := make([]string, 0, len(config.GlobalSpells.Spells))
	for k := range config.GlobalSpells.Spells {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) < 30 {
		t.Fatalf("expected the full spell roster, got %d", len(keys))
	}

	for _, key := range keys {
		key := key
		t.Run(key, func(t *testing.T) {
			game, _, _ := tbBehaviorGame(t, 9, 9)
			def, err := spells.GetSpellDefinitionByID(spells.SpellID(key))
			if err != nil {
				t.Fatalf("def: %v", err)
			}
			equipSpellAndPrepareCaster(t, game.combat, key, 200, 30)
			caster := game.party.Members[0]
			ally := game.party.Members[1]

			// A tanky monster adjacent to the party for offensive spells.
			mon := monster.NewMonster3DFromConfig(game.camera.X+32, game.camera.Y, "goblin", game.config)
			mon.MaxHitPoints, mon.HitPoints = 500, 500
			game.world.Monsters = []*monster.Monster3D{mon}
			game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

			projBefore := len(game.magicProjectiles) + len(game.arrows)
			spBefore := caster.SpellPoints

			// Scene prep per effect class.
			switch {
			case def.TownPortal:
				// A destination must be known or the cast refunds itself.
				game.visitedTavernMaps = map[string]bool{"forest": true}
			case def.OutdoorOnly:
				// The sky-variant check stats assets/ relative to the repo root.
				t.Chdir("../..")
				// Fly is gated to open-sky maps: point the world manager at a
				// map whose sky ships day/night variants (arena_panorama_*).
				prev := world.GlobalWorldManager
				world.GlobalWorldManager = &world.WorldManager{
					MapConfigs:    map[string]*config.MapConfig{"arena": {SkyTexture: "arena_panorama"}},
					CurrentMapKey: "arena",
				}
				t.Cleanup(func() { world.GlobalWorldManager = prev })
			case def.ReviveHpPct > 0 || def.Revive: // raise_dead / resurrect
				ally.MaxHitPoints, ally.HitPoints = 40, 0
				ally.AddCondition(character.ConditionDead)
			case def.Awaken:
				ally.MaxHitPoints, ally.HitPoints = 40, 0
				ally.AddCondition(character.ConditionUnconscious)
			case def.HealParty:
				for _, m := range game.party.Members {
					m.MaxHitPoints, m.HitPoints = 100, 40
				}
			case def.IsHeal():
				caster.MaxHitPoints, caster.HitPoints = 100, 40
			}

			if !game.combat.CastEquippedSpell() {
				t.Fatalf("CastEquippedSpell returned false")
			}
			if caster.SpellPoints >= spBefore {
				t.Errorf("SP not consumed (%d -> %d)", spBefore, caster.SpellPoints)
			}

			// Class-correct effect assertion.
			switch {
			case def.ReviveHpPct > 0 || def.Revive:
				if ally.HasCondition(character.ConditionDead) || ally.HitPoints <= 0 {
					t.Errorf("ally not revived (hp=%d, dead=%v)", ally.HitPoints, ally.HasCondition(character.ConditionDead))
				}
			case def.Awaken:
				if ally.HasCondition(character.ConditionUnconscious) || ally.HitPoints <= 0 {
					t.Errorf("ally not awakened (hp=%d)", ally.HitPoints)
				}
			case def.HealParty:
				for i, m := range game.party.Members {
					if m.HitPoints <= 40 {
						t.Errorf("party member %d not healed (%d)", i, m.HitPoints)
					}
				}
			case def.IsHeal():
				if caster.HitPoints <= 40 {
					t.Errorf("caster not healed (%d)", caster.HitPoints)
				}
			case def.StunRadiusTiles > 0:
				if mon.StunFramesRemaining <= 0 && mon.StunTurnsRemaining <= 0 {
					t.Errorf("monster not stunned")
				}
			case def.ZoneRadiusTiles > 0:
				if len(game.steamZones) == 0 {
					t.Errorf("no damage zone created")
				}
			case def.PartyAoeRadiusTiles > 0 || def.MapWide:
				if mon.HitPoints >= 500 {
					t.Errorf("party-nova did not damage the monster")
				}
			case def.ResistBuffPct > 0 || def.OutgoingDamageBonus > 0 || def.IncomingDamageReduction > 0 || def.StatBonus > 0:
				if len(game.combatBuffs) == 0 && game.statBonuses.IsZero() {
					t.Errorf("no buff applied (combatBuffs=%d statBonuses=%+v)", len(game.combatBuffs), game.statBonuses)
				}
			case def.IsProjectile:
				if len(game.magicProjectiles)+len(game.arrows) <= projBefore {
					t.Errorf("projectile spell spawned no projectile")
				}
				if !def.DealsNoDamage {
					if d, _, _ := game.combat.CalculateSpellDamage(def.ID, caster); d <= 0 {
						t.Errorf("damage spell computes 0 base damage")
					}
				}
			default:
				// Pure utility (torch_light, wizard_eye, walk_on_water,
				// water_breathing): the cast firing + SP consumption above is the
				// guarantee - they have no combat-state effect to probe here.
			}
		})
	}
}
