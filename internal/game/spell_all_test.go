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
	previousTiles := world.GlobalTileManager
	t.Cleanup(func() { world.GlobalTileManager = previousTiles })
	world.GlobalTileManager = world.NewTileManager(testTileSizeClasses())
	if err := world.GlobalTileManager.LoadTileConfig("../../assets/tiles.yaml"); err != nil {
		t.Fatalf("load tiles: %v", err)
	}
	// Summon and zone spells resolve their world through GetCurrentWorld, which
	// prefers the process-wide manager. Pin it to nil so the sweep always runs
	// against each subtest's own fixture instead of whatever ran before it.
	setTestWorldManager(t, nil)
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
			game.combat.controlSaveRoll = func() float64 { return 0.99 } // Exercise effects, not random saves.
			def, err := spells.GetSpellDefinitionByID(spells.SpellID(key))
			if err != nil {
				t.Fatalf("def: %v", err)
			}
			equipSpellAndPrepareCaster(t, game.combat, key, 200, 30)
			caster := game.party.Members[0]
			ally := game.party.Members[1]
			game.camera.Angle = 0 // Targeted spells and cones face the prepared enemy.

			// A tanky monster adjacent to the party for offensive spells.
			mon := monster.NewMonster3DFromConfig(game.camera.X+game.config.GetTileSize(), game.camera.Y, "goblin", game.config)
			mon.MaxHitPoints, mon.HitPoints = 500, 500
			game.world.Monsters = []*monster.Monster3D{mon}
			game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

			projBefore := len(game.magicProjectiles) + len(game.arrows)
			spBefore := caster.SpellPoints
			mechanic := ""
			if def.Mechanic != nil {
				mechanic = string(def.Mechanic.Kind)
			}
			switch mechanic {
			case "destroy_undead", "turn_undead":
				mon.MonsterType = monster.TypeUndead
			case "purify", "dispel_magic":
				ally.ApplyPoison(10 * game.config.GetTPS())
				if mechanic == "dispel_magic" {
					game.magicProjectiles = append(game.magicProjectiles, MagicProjectile{ID: "hostile", Active: true, LifeTime: 60, SpellType: "firebolt", Owner: ProjectileOwnerMonster, X: game.camera.X, Y: game.camera.Y})
				}
			case "shared_life", "souldrinker":
				ally.MaxHitPoints, ally.HitPoints = 100, 40
			case "vvoid_beacon":
				setTestWorldManager(t, &world.WorldManager{
					CurrentMapKey: "test",
					LoadedMaps:    map[string]*world.World3D{"test": game.world},
					MapConfigs:    map[string]*config.MapConfig{"test": {Name: "Test"}},
				})
			}

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
				setTestWorldManager(t, &world.WorldManager{
					MapConfigs:    map[string]*config.MapConfig{"arena": {SkyTexture: "arena_panorama"}},
					CurrentMapKey: "arena",
				})
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

			if def.TownPortal {
				// The cast only opens the destination picker; the confirm pays.
				if game.combat.CastEquippedSpell() || caster.SpellPoints != spBefore || !game.townPortalPickerOpen {
					t.Fatalf("portal cast must open the picker unpaid (sp %d -> %d, open %v)", spBefore, caster.SpellPoints, game.townPortalPickerOpen)
				}
				caster.ActionsRemaining = 1 // the confirm spends the caster's TB action
				game.confirmTownPortal("forest")
			} else if mechanic == "vvoid_beacon" {
				if game.combat.CastEquippedSpell() || caster.SpellPoints != spBefore || !game.beaconPicker {
					t.Fatal("beacon cast must open its picker without spending SP")
				}
				caster.ActionsRemaining = 1
				game.confirmBeacon(0, true)
			} else if def.TargetsAlly() {
				if game.combat.CastEquippedSpell() || caster.SpellPoints != spBefore || !game.allySpellSelecting {
					t.Fatal("ally buff must enter paused targeting without spending SP")
				}
				caster.ActionsRemaining = 1
				game.confirmAllySpell(0)
			} else if mechanic == "identify_monster" {
				if game.combat.CastEquippedSpell() || caster.SpellPoints != spBefore || !game.identifySelecting {
					t.Fatal("identify must enter paused targeting without spending SP")
				}
				caster.ActionsRemaining = 1
				game.confirmIdentify(mon)
			} else if !game.combat.CastEquippedSpell() {
				t.Fatalf("CastEquippedSpell returned false")
			}
			if caster.SpellPoints >= spBefore {
				t.Errorf("SP not consumed (%d -> %d)", spBefore, caster.SpellPoints)
			}

			// Class-correct effect assertion.
			switch {
			case def.Mechanic != nil && !def.IsProjectile:
				switch mechanic {
				case "fire_aura", "fate", "vampiric_weapon":
					if len(caster.SpellState.Effects) != 1 || caster.SpellState.Effects[0].SpellID != key {
						t.Fatal("personal spell effect missing")
					}
				case "haste", "shield", "preservation", "regeneration", "protection_from_magic", "celestial_armor", "pain_reflection":
					if game.advancedBuff(config.MechanicKind(mechanic)) == nil {
						t.Fatal("shared spell effect missing")
					}
				case "purify", "dispel_magic":
					if ally.Purifiable() || ally.PoisonFramesRemaining != 0 {
						t.Fatal("cleanse did not remove poison")
					}
					if mechanic == "dispel_magic" && game.magicProjectiles[0].Active {
						t.Fatal("dispel left hostile magic active")
					}
				case "shared_life":
					if ally.HitPoints <= 40 {
						t.Fatal("shared life did not redistribute HP")
					}
				case "vvoid_beacon":
					if len(caster.SpellState.Beacons) != 1 || caster.SpellState.Beacons[0].MapKey != "test" {
						t.Fatal("beacon confirmation did not bind a point")
					}
				case "identify_monster":
					if game.identifiedMonster != mon || !game.townPortalPickerOpen {
						t.Fatal("identify did not open the target report")
					}
				case "berserk":
					if mon.Berserk.Frames <= 0 {
						t.Fatal("berserk did not affect the living target")
					}
				case "frozen_ground":
					if mon.Root.Frames <= 0 {
						t.Fatal("frozen ground did not root the forward target")
					}
				case "slow":
					if mon.Slow.Frames <= 0 {
						t.Fatal("slow did not affect the target")
					}
				case "turn_undead", "mass_fear":
					if mon.Fear.Frames <= 0 {
						t.Fatal("fear did not affect the eligible target")
					}
				case "feeblemind":
					if mon.Silence.Frames <= 0 {
						t.Fatal("feeblemind did not silence the target")
					}
				case "destroy_undead", "prismatic_light", "souldrinker":
					if mon.HitPoints >= 500 {
						t.Fatal("direct spell did not damage the eligible target")
					}
				default:
					t.Fatalf("missing effect assertion for mechanic %q", mechanic)
				}
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
				if !mon.Stun.Active() {
					t.Errorf("monster not stunned")
				}
			case def.ZoneRadiusTiles > 0:
				if len(game.persistentDamageZones) == 0 {
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
