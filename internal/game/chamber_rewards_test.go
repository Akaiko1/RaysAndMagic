package game

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

func TestChamberRewardsLootAndCollection(t *testing.T) {
	loadTestConfig(t)
	before := config.GlobalLoots
	loots, err := config.LoadLootTables("../../assets/loots.yaml")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { config.GlobalLoots = before })
	if err := ValidateCardDrops(); err != nil {
		t.Fatal(err)
	}
	cases := []struct{ boss, card, relic string }{
		{"solstice_aureth", "solstice_aureth_card", "solstice_sun_crucible"},
		{"solstice_neressa", "solstice_neressa_card", "solstice_everflow_retort"},
		{"solstice_talura", "solstice_talura_card", "solstice_stone_script"},
		{"solstice_vaelith", "solstice_vaelith_card", "solstice_returning_gale"},
	}
	for _, tc := range cases {
		t.Run(tc.boss, func(t *testing.T) {
			g, _, _, tile := sniperFixture(t, false)
			var trophies []config.LootEntry
			for _, e := range loots.Loots[tc.boss] {
				if e.Key != tc.card && e.Key != tc.relic {
					continue
				}
				// The card's rate is ValidateCardDrops' contract, checked above.
				if e.Chance <= 0 || !e.KillOnly || e.RollCount() != 1 {
					t.Fatalf("bad boss roll: %+v", e)
				}
				e.Chance = 1 // Certain rolls expose independence without statistical flakiness.
				trophies = append(trophies, e)
			}
			if len(trophies) != 2 {
				t.Fatal("boss must have both independent trophies")
			}
			original := loots.Loots[tc.boss]
			loots.Loots[tc.boss] = trophies
			defer func() { loots.Loots[tc.boss] = original }()
			m := monster.NewMonster3DFromConfig(g.camera.X+3*tile, g.camera.Y, tc.boss, g.config)
			g.world.Monsters = []*monster.Monster3D{m}
			for _, it := range g.combat.pocketLoot(m) {
				if itemCardKey(it) == tc.card || legendaryStampKey(it) == tc.relic {
					t.Fatal("victory trophy stolen")
				}
			}
			m.HitPoints = 0
			g.combat.finishMonsterKillImmediately(m)
			var card, relic bool
			for _, bag := range g.groundContainers {
				for _, it := range bag.Items {
					card = card || itemCardKey(it) == tc.card
					relic = relic || legendaryStampKey(it) == tc.relic
				}
			}
			if !card || !relic {
				t.Fatal("a death failed to award both successful rolls")
			}
			count := len(g.groundContainers)
			g.combat.finishMonsterKillImmediately(m)
			if len(g.groundContainers) != count {
				t.Fatal("death settled twice")
			}
			if !slices.Contains(CardAlbumKeys(), tc.card) || !slices.Contains(LegendaryCollectionKeys(), tc.relic) {
				t.Fatal("missing collection place")
			}
		})
	}
}

func TestChamberCrumblingDirectHitContract(t *testing.T) {
	for _, tb := range []bool{false, true} {
		t.Run(fmt.Sprint(tb), func(t *testing.T) {
			g, _, ch, tile := sniperFixture(t, tb)
			isolateTrueDamageMember(ch, 0)
			g.cardSlots[0].key, g.cardSlots[1].key = "solstice_talura_card", "solstice_talura_card"
			it := items.CreateWeaponFromYAML("iron_sword")
			ch.Equipment[items.SlotMainHand] = it
			m, _ := partyDamageTargets(g, tile)
			m.ArmorClass = 101
			m.PerfectDodge = 100
			g.combat.ApplyDamageToMonster(m, 10, it.Name, false)
			if m.Relics.CrumbleArmor != 0 {
				t.Fatal("dodged hit eroded armor")
			}
			m.PerfectDodge = 0
			g.combat.ApplyDamageToMonster(m, 10, it.Name, false)
			if m.Relics.CrumbleArmor != 3 || m.EffectiveArmorClass() != 98 {
				t.Fatal("first direct hit or duplicate card rule failed")
			}
			g.combat.elementalWeaponBonus(m, 10, "earth", "Bonus")
			if m.Relics.CrumbleArmor != 3 {
				t.Fatal("secondary elemental bonus eroded armor")
			}
			for range 20 {
				g.combat.ApplyDamageToMonster(m, 10, it.Name, false)
			}
			if m.Relics.CrumbleArmor != 50 || m.EffectiveArmorClass() != 51 {
				t.Fatalf("cap failed: %+v", m.Relics)
			}
			// The cap follows the current armor: a switch that cuts it after the
			// hits leaves what cutting it first would have left.
			m.ArmorClass = 45
			if m.EffectiveArmorClass() != 23 {
				t.Fatalf("armor cut after erosion: %d, want 23", m.EffectiveArmorClass())
			}
			first, _ := partyDamageTargets(g, tile)
			first.ArmorClass, first.PerfectDodge = 45, 0
			for range 20 {
				g.combat.ApplyDamageToMonster(first, 10, it.Name, false)
			}
			if first.EffectiveArmorClass() != 23 {
				t.Fatalf("armor cut before erosion: %d, want 23", first.EffectiveArmorClass())
			}
			m.ArmorClass = 101
			m.ArmorShredPct = 25
			m.ArmorShredFramesRemaining = 600
			if m.EffectiveArmorClass() >= 51 {
				t.Fatal("erosion excluded ordinary armor debuffs")
			}
		})
	}
}

func TestChamberWildfireDeathChains(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, burningChild := range []bool{false, true} {
			t.Run(fmt.Sprintf("TB=%v/child=%v", tb, burningChild), func(t *testing.T) {
				g, _, _, tile := sniperFixture(t, tb)
				g.cardSlots[0].key, g.cardSlots[1].key = "solstice_aureth_card", "solstice_aureth_card"
				a, b := partyDamageTargets(g, 3*tile)
				a.MaxHitPoints, a.HitPoints, a.BurnFramesRemaining = 1000, 0, 600
				b.X, b.Y, b.MaxHitPoints, b.HitPoints = a.X+tile, a.Y, 1000, 100
				if burningChild {
					b.BurnFramesRemaining = 600
					b.NoKillRewards = true // Hostile boss adds still trigger combat cards.
				}
				c := mkTestMonster("Far", 1000)
				c.ID, c.X, c.Y = "far", a.X+2.5*tile, a.Y
				g.world.Monsters = append(g.world.Monsters, c)
				g.combat.finishMonsterKillImmediately(a)
				want := 0
				if burningChild {
					want = 150
				}
				if c.MaxHitPoints-c.HitPoints != want || b.IsAlive() {
					t.Fatalf("chain=%d want %d, childHP=%d", c.MaxHitPoints-c.HitPoints, want, b.HitPoints)
				}
				if burningChild && c.BurnFramesRemaining != 6*g.config.GetTPS() {
					t.Fatal("survivor burn missing")
				}
				before := c.HitPoints
				g.combat.finishMonsterKillImmediately(a)
				if c.HitPoints != before {
					t.Fatal("same death exploded twice")
				}
			})
		}
	}
}

func TestChamberOverflowPaymentAndExcess(t *testing.T) {
	for _, outcome := range []spellCastOutcome{castCommitted, castRejected} {
		for _, cost := range []int{0, 20} {
			t.Run(fmt.Sprintf("outcome=%d/cost=%d", outcome, cost), func(t *testing.T) {
				cs := newTestCombatSystemWithConfig(t)
				g := cs.game
				g.cardSlots[0].key, g.cardSlots[1].key = "solstice_neressa_card", "solstice_neressa_card"
				for _, c := range g.party.Members {
					c.HitPoints, c.MaxHitPoints, c.SpellPoints, c.MaxSpellPoints = 100, 100, 50, 100
				}
				c := g.party.Members[0]
				def, err := spells.GetSpellDefinitionByID("heal_other")
				if err != nil {
					t.Fatal(err)
				}
				cs.executeSpellCast(spellCastRequest{ID: "heal_other", Definition: def, Caster: c, Cost: cost}, func() spellCastOutcome { cs.healWholeParty(100); return outcome })
				total := 0
				for _, hero := range g.party.Members {
					total += hero.SpellPoints - 50
				}
				want := 0
				if outcome == castCommitted {
					want = -cost + cost/2
				}
				if total != want {
					t.Fatalf("SP delta %d want %d", total, want)
				}
				// Regeneration/free healing outside the paid cast has no refund.
				before := c.SpellPoints
				cs.healMember(0, 100)
				if c.SpellPoints != before {
					t.Fatal("free healing returned SP")
				}
				if !g.healingPotionHasEffect(c) {
					t.Fatal("healthy hero with missing SP cannot drink")
				}
				g.applyFlatHeal(0, 40, 1000000)
				if c.SpellPoints-before != 10 {
					t.Fatalf("potion excess returned %d want 10", c.SpellPoints-before)
				}
			})
		}
	}
}

func TestChamberOverflowSharedLife(t *testing.T) {
	for _, full := range []bool{false, true} {
		for _, cost := range []int{0, 12} {
			t.Run(fmt.Sprintf("full=%v/cost=%d", full, cost), func(t *testing.T) {
				cs := newTestCombatSystemWithConfig(t)
				g := cs.game
				g.cardSlots[0].key = "solstice_neressa_card"
				for _, ch := range g.party.Members {
					ch.MaxHitPoints, ch.HitPoints, ch.MaxSpellPoints, ch.SpellPoints = 100, 100, 100, 50
					if !full {
						ch.HitPoints = 10
					}
				}
				def, err := spells.GetSpellDefinitionByID("shared_life")
				if err != nil {
					t.Fatal(err)
				}
				if cs.castSpell(spellCastRequest{ID: "shared_life", Definition: def, Caster: g.party.Members[0], Cost: cost}) != castCommitted {
					t.Fatal("Shared Life rejected")
				}
				delta := 0
				for _, ch := range g.party.Members {
					delta += ch.SpellPoints - 50
				}
				want := -cost
				if full {
					want += cost / 2
				}
				if delta != want {
					t.Fatalf("Shared Life SP delta=%d want=%d", delta, want)
				}
			})
		}
	}
}

func TestChamberStormReversalUsesDefensesWithoutSpendingThem(t *testing.T) {
	for _, delivery := range []string{"melee", "bolt", "spell bolt", "splash", "breath", "hazard", "warded melee"} {
		t.Run(delivery, func(t *testing.T) {
			g, _, ch, tile := sniperFixture(t, false)
			isolateTrueDamageMember(ch, 0)
			g.cardSlots[0].key, g.cardSlots[1].key = "solstice_vaelith_card", "solstice_vaelith_card"
			ch.Luck = 1000
			ch.ReactiveCombat.Shell = 20
			m, _ := partyDamageTargets(g, tile)
			m.Resistances = map[monster.DamageType]int{monster.DamageAir: 50}
			m.BossWarded = delivery == "warded melee" // an idol-warded source takes nothing back
			g.combatLogHistory = nil
			beforeHP, beforeState := ch.HitPoints, ch.ReactiveCombat
			hit := monsterCharacterHit{Parts: damagecalc.Parts{Normal: 100}, DamageType: "physical", IgnoresArmor: true, Melee: delivery == "melee" || delivery == "warded melee", Projectile: delivery == "bolt" || delivery == "spell bolt" || delivery == "splash", Spell: delivery == "spell bolt" || delivery == "breath", Hazard: delivery == "hazard"}
			if delivery == "splash" {
				g.combat.applyMonsterProjectileDamageAoE(m, m.Name, hit)
			} else {
				g.combat.monsterHitCharacter(m, ch, m.Name, hit)
			}
			want := 40
			if delivery == "hazard" || delivery == "splash" || delivery == "breath" || delivery == "warded melee" {
				want = 0
			}
			if m.MaxHitPoints-m.HitPoints != want || ch.HitPoints != beforeHP || !reflect.DeepEqual(ch.ReactiveCombat, beforeState) {
				t.Fatalf("reflection=%d state=%+v", m.MaxHitPoints-m.HitPoints, ch.ReactiveCombat)
			}
			if logged := countCombatLog(g, "Vaelith Card") > 0; logged != (want > 0) {
				t.Fatalf("Storm Reversal logged=%v for a %d reflection", logged, want)
			}
		})
	}
}

// A dodged physical bolt draws its armor pierce once: the graze and Storm
// Reversal's avoided-damage preview share it.
func TestChamberStormReversalSharesThePierceRoll(t *testing.T) {
	for _, tc := range []struct {
		name            string
		grazes, pierced bool
	}{{"graze-pierced", true, true}, {"graze-armored", true, false}, {"no-graze-pierced", false, true}, {"no-graze-armored", false, false}} {
		t.Run(tc.name, func(t *testing.T) {
			g, _, ch, tile := sniperFixture(t, false)
			isolateTrueDamageMember(ch, 0)
			g.cardSlots[0].key = "solstice_vaelith_card"
			ch.Luck = 1000
			ch.Equipment[items.SlotArmor] = items.CreateItemFromYAML("jade_tortoise_do")
			m, _ := partyDamageTargets(g, tile)
			draws := 0
			g.combat.projectilePierceRoll = func(int) int {
				draws++
				if tc.pierced {
					return 0
				}
				return 99
			}
			if g.combat.CalculateTotalArmorClass(ch) <= 0 {
				t.Fatal("fixture hero needs armor for a pierce roll")
			}
			hit := monsterCharacterHit{Parts: damagecalc.Parts{Normal: 100}, DamageType: "physical", Projectile: true, GrazesDodge: tc.grazes}
			g.combat.monsterHitCharacter(m, ch, m.Name, hit)
			if draws != 1 {
				t.Fatalf("pierce drawn %d times for one dodged bolt, want 1", draws)
			}
			if m.MaxHitPoints-m.HitPoints <= 0 {
				t.Fatal("Storm Reversal returned nothing")
			}
		})
	}
}

// Solar Flare's lane and Wildfire's burst pick victims like every party
// splash (partyAreaHurts): an enemy and a bound ally are hit, a pure party
// summon and an idol-warded monster are not.
func TestChamberAreaRidersHurtLikeASplash(t *testing.T) {
	for _, source := range []string{"solar flare", "wildfire"} {
		t.Run(source, func(t *testing.T) {
			g, _, ch, tile := sniperFixture(t, false)
			g.snapFacing(0)
			isolateTrueDamageMember(ch, 0)
			primary, _ := partyDamageTargets(g, tile)
			g.world.Monsters = []*monster.Monster3D{primary}
			victims := map[string]bool{"enemy": true, "bound ally": true, "pure summon": false, "warded": false}
			placed := map[string]*monster.Monster3D{}
			for i, kind := range []string{"enemy", "bound ally", "pure summon", "warded"} {
				m := mkTestMonster(kind, 10000)
				m.ID, m.X, m.Y = kind, primary.X+float64(i+1)*tile/2, primary.Y
				switch kind {
				case "bound ally":
					m.Bound, m.BoundFramesRemaining = true, 1000
				case "pure summon":
					m.SummonedBy = spellSummonOwnerPrefix + "summon_wolf"
				case "warded":
					m.BossWarded = true
				}
				placed[kind] = m
				g.world.Monsters = append(g.world.Monsters, m)
			}
			switch source {
			case "solar flare":
				ch.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("solstice_sun_crucible")
				staff := ch.Equipment[items.SlotMainHand]
				staff.Signature.Charges = 1
				ch.Equipment[items.SlotMainHand] = staff
				g.combat.createArrowAttack(100, items.SlotMainHand, "")
				g.combat.releaseSolarFlare(primary, &g.arrows[0])
			case "wildfire":
				g.cardSlots[0].key = "solstice_aureth_card"
				primary.HitPoints = 0
				primary.ApplyBurn(g.config.GetTPS())
				g.combat.triggerWildfire(primary)
			}
			for kind, want := range victims {
				if hurt := placed[kind].HitPoints < placed[kind].MaxHitPoints; hurt != want {
					t.Errorf("%s hurt=%v, want %v", kind, hurt, want)
				}
			}
		})
	}
}

func TestChamberSunChargesAndLandedSnapshot(t *testing.T) {
	for _, trigger := range []string{"walk", "forced movement", "spell", "free spell", "rejected spell", "teleport", "blocked teleport"} {
		t.Run(trigger, func(t *testing.T) {
			g, ch := rareClassGame(t, character.ClassWayfarer, false)
			ch.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("solstice_sun_crucible")
			def, _ := config.GetWeaponDefinition("solstice_sun_crucible")
			want := 1
			switch trigger {
			case "walk":
				(&InputHandler{game: g}).movePlayer(g.config.GetTileSize(), 0)
				(&InputHandler{game: g}).movePlayer(g.config.GetTileSize(), 0)
			case "forced movement":
				g.camera.X += 3 * g.config.GetTileSize()
				g.partyMoved(g.camera.X-3*g.config.GetTileSize(), g.camera.Y)
			case "teleport", "blocked teleport":
				if trigger == "blocked teleport" {
					ch.SpellPoints = 0
					want = 0
				}
				g.useTechnique(0, "fold_step", false, false)
			default:
				spell, _ := spells.GetSpellDefinitionByID("firebolt")
				cost, outcome := 4, castCommitted
				if trigger == "free spell" {
					cost, want = 0, 0
				}
				if trigger == "rejected spell" {
					outcome, want = castRejected, 0
				}
				g.combat.executeSpellCast(spellCastRequest{ID: "firebolt", Definition: spell, Caster: ch, Cost: cost}, func() spellCastOutcome { return outcome })
			}
			if ch.Equipment[items.SlotMainHand].Signature.Charges != want {
				t.Fatalf("charge trigger %s failed", trigger)
			}
			if want == 0 {
				return
			}
			ch.Intellect = 60
			primary, behind := partyDamageTargets(g, g.config.GetTileSize())
			g.combat.createArrowAttack(100, items.SlotMainHand, "")
			shot := &g.arrows[0]
			payload := shot.Signature
			if payload.Damage != 50 {
				t.Fatalf("staff flare INT scaling: %d want 50", payload.Damage)
			}
			if !payload.Flare || ch.Equipment[items.SlotMainHand].Signature.Charges != 1 {
				t.Fatal("launch spent the charge instead of reserving it")
			}
			original := *def.Signature
			def.Signature.Damage = 900
			defer func() { *def.Signature = original }()
			ch.Intellect = 900
			before := behind.HitPoints
			g.combat.applyProjectileDamage(shot, primary, shot.ID)
			if before-behind.HitPoints != payload.Damage || ch.Equipment[items.SlotMainHand].Signature.Charges != 0 || countWorldVisuals(g, worldSolarFlare) != 1 {
				t.Fatalf("landed snapshot=%d want %d", before-behind.HitPoints, payload.Damage)
			}
		})
	}
}

// resolveArrowHit resolves one arrow-on-monster contact the way the collision
// pass does: on a detached copy committed back after any continuation spawn.
func resolveArrowHit(g *MMGame, index int, m *monster.Monster3D) {
	shot := g.arrows[index]
	g.combat.applyProjectileDamage(&shot, m, shot.ID)
	g.arrows[index] = shot
}

// TestChamberReturningGalePiercesBothWays: thrown down a line of two foes, the
// blade pierces both to its full range, turns, pierces both again on the way
// back for the return share, and is caught at the party: each foe takes one
// outward and one return hit, through the real projectile tick.
func TestChamberReturningGalePiercesBothWays(t *testing.T) {
	g, gl, ch, tile := sniperFixture(t, false)
	g.snapFacing(0)
	isolateTrueDamageMember(ch, 0)
	ch.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("solstice_returning_gale")
	near, far := partyDamageTargets(g, 2*tile)
	far.X = g.camera.X + 4*tile
	g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
	g.combat.createArrowAttack(100, items.SlotMainHand, "")
	out := g.arrows[0].Damage
	back := out * g.arrows[0].Signature.Rule.ReturnDamagePercent / 100
	flyShots(t, g, gl)
	for _, m := range []*monster.Monster3D{near, far} {
		if taken := m.MaxHitPoints - m.HitPoints; taken != out+back {
			t.Fatalf("%s took %d, want %d out and %d back", m.Name, taken, out, back)
		}
	}
	if g.projectileCount() != 0 {
		t.Fatal("the blade was never caught")
	}
}

// Both legs are the shot's school: an air-immune foe takes neither, a
// fire-immune one takes both, and every leg's log line names air.
func TestChamberReturningGaleLegsKeepTheirSchool(t *testing.T) {
	g, gl, ch, tile := sniperFixture(t, false)
	g.snapFacing(0)
	isolateTrueDamageMember(ch, 0)
	ch.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("solstice_returning_gale")
	airImmune, fireImmune := partyDamageTargets(g, 2*tile)
	fireImmune.X = g.camera.X + 4*tile
	airImmune.Resistances = map[monster.DamageType]int{monster.DamageAir: 100}
	fireImmune.Resistances = map[monster.DamageType]int{monster.DamageFire: 100}
	g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
	g.combatLogHistory = nil
	g.combat.createArrowAttack(100, items.SlotMainHand, "")
	out := g.arrows[0].Damage
	back := out * g.arrows[0].Signature.Rule.ReturnDamagePercent / 100
	flyShots(t, g, gl)
	if airImmune.HitPoints != airImmune.MaxHitPoints || fireImmune.MaxHitPoints-fireImmune.HitPoints != out+back {
		t.Fatalf("air-immune took %d (want 0), fire-immune took %d (want %d)",
			airImmune.MaxHitPoints-airImmune.HitPoints, fireImmune.MaxHitPoints-fireImmune.HitPoints, out+back)
	}
	legs := 0
	for _, line := range g.combatLogHistory {
		if strings.Contains(line.Text, "Returning Gale") && strings.Contains(line.Text, "(HP:") {
			legs++
			if !strings.Contains(line.Text, " air damage") {
				t.Fatalf("leg reported without its school: %q", line.Text)
			}
		}
	}
	if legs != 4 {
		t.Fatalf("%d leg reports, want 2 foes x 2 legs", legs)
	}
}

// A dodged return leg reads like a dodged outward shot: the hero lands what
// gets through.
func TestChamberReturningGaleDodgedReturnNamesTheHero(t *testing.T) {
	g, _, ch, tile := sniperFixture(t, false)
	isolateTrueDamageMember(ch, 0)
	ch.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("solstice_returning_gale")
	m, _ := partyDamageTargets(g, 3*tile)
	g.world.Monsters = g.world.Monsters[:1]
	g.combat.createArrowAttack(100, items.SlotMainHand, "")
	g.arrows[0].X, g.arrows[0].Y, g.arrows[0].Active = m.X, m.Y, false
	g.combat.turnSpentBlades()
	g.arrows[0].TrueDamage = 10
	m.PerfectDodge = 100
	g.combatLogHistory = nil
	resolveArrowHit(g, 0, m)
	if countCombatLog(g, "dodges, but "+ch.Name+" lands") != 1 {
		t.Fatalf("dodged return logged %+v, want the hero landing the true damage", g.combatLogHistory)
	}
}

// A blade that stops without a victim - on a wall, at the end of its range or
// on its landing spot - turns there, halves its damage, and on the way back
// pierces a foe once.
func TestChamberReturningGaleTurnsWhereItStops(t *testing.T) {
	for _, turn := range []string{"wall", "range", "landing"} {
		t.Run(turn, func(t *testing.T) {
			g, _, ch, tile := sniperFixture(t, false)
			isolateTrueDamageMember(ch, 0)
			ch.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("solstice_returning_gale")
			m, _ := partyDamageTargets(g, 3*tile)
			g.world.Monsters = g.world.Monsters[:1]
			g.combat.createArrowAttack(100, items.SlotMainHand, "")
			shot := &g.arrows[0]
			shot.X, shot.Y = m.X, m.Y
			damage := shot.Damage
			switch turn {
			case "wall":
				w := ArrowWrapper{Arrow: shot, game: g}
				w.OnCollision(shot.X, shot.Y)
				w.ApplyCollisionEffects()
			case "range":
				w := ArrowWrapper{Arrow: shot, game: g}
				w.SetLifetime(0)
				w.ApplyCollisionEffects()
			case "landing":
				shot.Active = false // landShots ends a missed shot on its spot
			}
			bursts := len(g.spellHitEffects)
			g.combat.turnSpentBlades() // the projectile tick's serial turn
			if turn == "range" && bursts != 0 {
				t.Fatal("range end burst in mid-air")
			}
			if !shot.Active || !shot.Signature.ReturnLeg || shot.Damage != damage/2 || g.collisionSystem.GetEntityByID(shot.ID) == nil {
				t.Fatalf("bad return transition: %+v", shot)
			}
			before := m.HitPoints
			resolveArrowHit(g, 0, m)
			shot = &g.arrows[0]
			if before-m.HitPoints != damage/2 || !shot.Active {
				t.Fatal("return hit lost half damage or stopped flight")
			}
			// Collision dispatch can call a resolver again; a foe is pierced once a leg.
			before = m.HitPoints
			resolveArrowHit(g, 0, m)
			g.combat.CheckProjectileMonsterCollisions()
			if m.HitPoints != before {
				t.Fatal("the return pierced the same foe twice")
			}
			shot = &g.arrows[0]
			g.camera.Y += tile
			g.combat.aimReturningBlade(shot)
			if shot.VelY <= 0 {
				t.Fatal("return did not follow the moved party")
			}
			shot.X, shot.Y = g.camera.X, g.camera.Y
			g.combat.aimReturningBlade(shot)
			if shot.Active || g.collisionSystem.GetEntityByID(shot.ID) != nil {
				t.Fatal("caught blade left a projectile")
			}
		})
	}
}

// Both legs of a returning blade are party hits under one rule: Dark Elf
// Binding can claim the victim instead of the hit, and a dodged hit still
// ends Charm. The outward leg is the control.
func TestChamberReturningGaleLegsShareHitRules(t *testing.T) {
	for _, leg := range []string{"outward", "return"} {
		for _, rule := range []string{"binding", "charm through a dodge"} {
			t.Run(leg+"/"+rule, func(t *testing.T) {
				g, _, ch, tile := sniperFixture(t, false)
				isolateTrueDamageMember(ch, 0)
				ch.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("solstice_returning_gale")
				m, _ := partyDamageTargets(g, 3*tile)
				g.world.Monsters = g.world.Monsters[:1]
				g.combat.racialProcRoll = func(int) bool { return true }
				g.combat.createArrowAttack(100, items.SlotMainHand, "")
				g.arrows[0].X, g.arrows[0].Y = m.X, m.Y
				index := 0
				if leg == "return" {
					// The outward hit pierces; its continuation turns at range end.
					resolveArrowHit(g, 0, m)
					index = len(g.arrows) - 1
					g.arrows[index].Active = false
					g.combat.turnSpentBlades()
					if !g.arrows[index].Signature.ReturnLeg {
						t.Fatal("no return leg")
					}
				}
				switch rule {
				case "binding":
					ch.Skills[character.SkillDarkElfBinding] = &character.Skill{Mastery: character.MasteryNovice}
				default:
					m.Pacified, m.PacifiedFramesRemaining, m.PerfectDodge = true, 1000, 100
				}
				before := m.HitPoints
				resolveArrowHit(g, index, m)
				switch rule {
				case "binding":
					if !m.Bound || m.HitPoints != before {
						t.Fatalf("binding did not replace the hit: bound=%v damage=%d", m.Bound, before-m.HitPoints)
					}
				default:
					if m.Pacified || m.HitPoints != before {
						t.Fatalf("dodged hit: pacified=%v damage=%d", m.Pacified, before-m.HitPoints)
					}
				}
			})
		}
	}
}

func TestChamberRetortRechargeCatalysisAndOwnership(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, stock := range []string{"own", "shared"} { // a quick slot only holds its shortcut
			t.Run(fmt.Sprintf("TB=%v/%s", tb, stock), func(t *testing.T) {
				g, ch := rareClassGame(t, character.ClassAlchemist, tb)
				ch.Skills[character.SkillAlchemy].Mastery = character.MasteryGrandMaster
				ch.Skills[character.SkillBombThrowing].Mastery = character.MasteryNovice
				ch.Intellect = 60
				item := items.CreateItemFromYAML("solstice_everflow_retort")
				g.party.Inventory = nil
				ch.Inventory = nil
				switch stock {
				case "own":
					ch.Inventory = []items.Item{item}
				case "shared":
					g.party.Inventory = []items.Item{item}
				}
				if !g.equipFlask(0, "solstice_everflow_retort") || !g.throwFlask(0, "solstice_everflow_retort", false) {
					t.Fatal("retort failed to launch")
				}
				shot := g.flasks[0]
				if shot.Damage != 110 || shot.CatalysisPercent != 50 || g.flaskStock(ch, shot.Key) != 1 || g.flaskReady(ch, shot.Key) {
					t.Fatal("wrong mastery, consumption or recharge")
				}
				m := zoneVictim(t, g)
				m.ArmorClass = 0
				g.combat.detonateFlask(&shot, m.X, m.Y)
				mark := m.Relics.Catalysis[shot.ActorID]
				if mark.Percent != 50 || mark.Frames != 6*g.config.GetTPS() {
					t.Fatal("missing owner mark")
				}
				packet := g.combat.newPartyMonsterDamagePacket(100, 0, "fire", 0, false)
				other := shot
				other.Reusable = false
				other.ActorID = items.NewInstanceID()
				if catalyzedFlaskPacket(m, &other, packet).Components[0].Parts.Normal != 100 {
					t.Fatal("another hero consumed Catalysis")
				}
				if catalyzedFlaskPacket(m, &shot, packet).Components[0].Parts.Normal != 100 {
					t.Fatal("retort consumed its own mark")
				}
				other.ActorID = shot.ActorID
				boosted := catalyzedFlaskPacket(m, &other, packet)
				if boosted.Components[0].Parts.Normal != 150 || packet.Components[0].Parts.Normal != 100 || len(m.Relics.Catalysis) != 0 {
					t.Fatal("Catalysis did not consume exactly once, or mutated splash packet")
				}
				m.Relics.Catalysis = map[uint64]monster.CatalysisMark{shot.ActorID: mark}
				craft := FlaskProjectile{ID: "crafted", Key: "fire_flask", ActorID: shot.ActorID, Attacker: ch, Damage: 100, Radius: g.config.GetTileSize() * 2, BurnFrames: 360, LifeTime: 1, Active: true}
				beforeHP := m.HitPoints
				g.combat.detonateFlask(&craft, m.X, m.Y)
				if beforeHP-m.HitPoints != 150 || m.BurnFramesRemaining != 360 || len(m.Relics.Catalysis) != 0 {
					t.Fatal("crafted impact did not consume the mark or changed its burn")
				}
				ch.Eradicate()
				if relicActorID(ch) != shot.ActorID {
					t.Fatal("eradication replaced the Catalysis owner identity")
				}
				g.tickDeviceCooldowns(6*g.config.GetTPS() - 1)
				if g.flaskReady(ch, shot.Key) {
					t.Fatal("early recharge")
				}
				g.tickDeviceCooldowns(1)
				if !g.flaskReady(ch, shot.Key) {
					t.Fatal("physical item never recharged")
				}
				text := GetItemTooltipRows(item, ch, g.combat, true).String()
				d, _ := config.GetItemDefinition(shot.Key)
				if !strings.Contains(text, "Alchemy") || !strings.Contains(text, "Catalysis") || !strings.Contains(text, config.DurationText(d.Flask.CatalysisSeconds)) || !strings.Contains(text, d.DeviceRechargeLine()) {
					t.Fatalf("incomplete flask tooltip: %s", text)
				}
			})
		}
	}
}

func TestChamberRuneDischargePrimaryOnly(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, sp := range []int{0, 2, 3, 20} {
			for _, lethal := range []bool{false, true} {
				t.Run(fmt.Sprintf("TB=%v/SP=%d/lethal=%v", tb, sp, lethal), func(t *testing.T) {
					g, _, ch, tile := sniperFixture(t, tb)
					g.snapFacing(0)
					isolateTrueDamageMember(ch, 0)
					ch.Intellect, ch.SpellPoints = 60, sp
					it := items.CreateWeaponFromYAML("solstice_stone_script")
					ch.Equipment[items.SlotMainHand] = it
					primary, flank := partyDamageTargets(g, tile)
					flank.X, flank.Y = primary.X, g.camera.Y+tile
					g.world.Monsters = []*monster.Monster3D{flank, primary} // World order is not the primary hit.
					if lethal {
						primary.HitPoints = 1
					}
					def, _ := config.GetWeaponDefinition("solstice_stone_script")
					g.combat.performMeleeHitDetection(it, 100, def.Melee, false)
					// A killing swing discharges nothing and pays nothing.
					wantSP, wantDamage := sp, 100
					if sp >= 3 && !lethal {
						wantSP -= 3
						wantDamage += 60
					}
					if ch.SpellPoints != wantSP || flank.MaxHitPoints-flank.HitPoints != 100 {
						t.Fatalf("SP %d, want %d; flank took %d, want 100: rune spent on a kill, twice or on a flank", ch.SpellPoints, wantSP, flank.MaxHitPoints-flank.HitPoints)
					}
					if !lethal && primary.MaxHitPoints-primary.HitPoints != wantDamage {
						t.Fatalf("primary=%d want %d", primary.MaxHitPoints-primary.HitPoints, wantDamage)
					}
				})
			}
		}
	}
}

// Rune Discharge strikes a monster this swing actually hits, whatever the
// arc: a diagonal the narrow arc assists to, or a flank the wide arc reaches.
func TestChamberRuneDischargeFollowsTheSwing(t *testing.T) {
	for _, tc := range []struct {
		name    string
		arc     int
		targets [][2]float64 // offsets in tiles from the party, facing east
	}{
		{"narrow-arc-diagonal-assist", 1, [][2]float64{{1, -1}, {1, 1}}},
		{"wide-arc-flank-only", 4, [][2]float64{{0, 1}}},
	} {
		for round := range 4 { // the narrow arc picks its diagonal at random
			t.Run(fmt.Sprintf("%s/%d", tc.name, round), func(t *testing.T) {
				g, _, ch, tile := sniperFixture(t, false)
				g.snapFacing(0)
				isolateTrueDamageMember(ch, 0)
				ch.Intellect, ch.SpellPoints = 60, 20
				it := items.CreateWeaponFromYAML("solstice_stone_script")
				ch.Equipment[items.SlotMainHand] = it
				g.world.Monsters = nil
				for i, off := range tc.targets {
					m := mkTestMonster(fmt.Sprint("T", i), 10000)
					m.ID, m.X, m.Y = fmt.Sprint("t", i), g.camera.X+off[0]*tile, g.camera.Y+off[1]*tile
					m.ArmorClass, m.PerfectDodge, m.Resistances = 0, 0, nil
					g.world.Monsters = append(g.world.Monsters, m)
				}
				def, _ := config.GetWeaponDefinition("solstice_stone_script")
				arc := *def.Melee
				arc.ArcType = tc.arc
				if hits := g.combat.performMeleeHitDetection(it, 100, &arc, false); hits != 1 {
					t.Fatalf("swing hit %d monsters, want 1", hits)
				}
				for _, m := range g.world.Monsters {
					if taken := m.MaxHitPoints - m.HitPoints; taken != 0 && taken != 160 {
						t.Fatalf("%s took %d, want the 100 hit plus the 60 discharge", m.Name, taken)
					}
				}
				if ch.SpellPoints != 17 {
					t.Fatalf("SP %d, want one 3 SP discharge", ch.SpellPoints)
				}
			})
		}
	}
}

func TestChamberWildfireLastBurnTick(t *testing.T) {
	for _, tb := range []bool{false, true} {
		t.Run(fmt.Sprint(tb), func(t *testing.T) {
			g, _, _, tile := sniperFixture(t, tb)
			g.cardSlots[0].key = "solstice_aureth_card"
			m, n := partyDamageTargets(g, tile)
			m.MaxHitPoints, m.HitPoints = 1000, 1
			m.ApplyBurn(1)
			m.RestoreDoTTickTimers(0, g.config.GetTPS()-1)
			if tb {
				g.tickMonsterTurnStatuses(m, true)
			} else {
				m.TickBurn()
			}
			if m.IsAlive() || m.BurnFramesRemaining != 0 {
				t.Fatal("last burn tick fixture did not kill")
			}
			g.combat.finishMonsterKillImmediately(m)
			if n.MaxHitPoints-n.HitPoints != 150 {
				t.Fatal("last burning tick lost the death trigger")
			}
		})
	}
}

func TestChamberRewardStateSaveRestores(t *testing.T) {
	cfg := loadTestConfig(t)
	w := newTestWorldSized(cfg, 8, 8)
	g := newTestGame(cfg, w)
	g.combat = NewCombatSystem(g)
	m := monster.NewMonster3DFromConfig(192, 192, "solstice_talura", cfg)
	baseArmor := m.ArmorClass
	actor := relicActorID(g.party.Members[0])
	m.Relics = monster.RelicState{CrumbleCapPct: 50, CrumbleArmor: 30, Catalysis: map[uint64]monster.CatalysisMark{actor: {Percent: 50, Frames: 500}}}
	w.Monsters = []*monster.Monster3D{m}
	staff := items.CreateWeaponFromYAML("solstice_sun_crucible")
	staff.Signature = items.WeaponSignatureState{Charges: 1, MoveDistance: 1.25}
	retort := items.CreateItemFromYAML("solstice_everflow_retort")
	retort.DeviceCooldownFrames = 400
	g.party.Members[0].Equipment[items.SlotMainHand] = staff
	g.party.Members[0].Inventory = []items.Item{retort}
	wm := world.NewWorldManager(cfg)
	wm.CurrentMapKey = "forest"
	wm.LoadedMaps = map[string]*world.World3D{"forest": w}
	setTestWorldManager(t, wm)
	data, err := json.Marshal(g.buildSave(wm))
	if err != nil {
		t.Fatal(err)
	}
	var saved GameSave
	if err = json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	loaded := newTestGame(cfg, newTestWorldSized(cfg, 8, 8))
	wm.LoadedMaps["forest"] = loaded.world
	loaded.restoreSavedMonsters(wm, &saved)
	ch := restoreCharacterSave(saved.Party.Members[0])
	if ch.ActorID != actor || ch.Inventory[0].DeviceCooldownFrames != 400 || ch.Equipment[items.SlotMainHand].Signature != staff.Signature {
		t.Fatal("hero, item or charge identity lost on load")
	}
	if len(loaded.world.Monsters) != 1 || !reflect.DeepEqual(loaded.world.Monsters[0].Relics, m.Relics) {
		t.Fatalf("monster relic state lost: %+v want %+v", loaded.world.Monsters[0].Relics, m.Relics)
	}
	if loaded.world.Monsters[0].EffectiveArmorClass() != baseArmor-30 {
		t.Fatal("restored armor ignored erosion")
	}
	loaded.world.Monsters[0].Relics.Tick(500)
	if len(loaded.world.Monsters[0].Relics.Catalysis) != 0 || m.Relics.Catalysis[actor].Frames != 500 {
		t.Fatal("snapshot aliases live mark state")
	}
	bad := staff
	bad.Signature.MoveDistance = math.NaN()
	bad.Signature.Charges = 999
	normalizeItemFromConfig(&bad)
	if bad.Signature.Charges != 1 || bad.Signature.MoveDistance != 0 {
		t.Fatal("saved charge normalization failed")
	}
	capPercent := config.CrumblingArmorCapPercent()
	state := monster.RelicState{CrumbleCapPct: 100, CrumbleArmor: baseArmor}.Normalized(capPercent)
	if capPercent != 50 || state.CrumbleCapPct != 50 || state.Erosion(baseArmor) != baseArmor/2 {
		t.Fatalf("cap %d%%, state %+v: saved erosion bypassed the authored cap", capPercent, state)
	}
	// Valid erosion survives a reload as is.
	eroded := monster.RelicState{CrumbleCapPct: 50, CrumbleArmor: 50}
	if got := eroded.Normalized(capPercent); got.CrumbleCapPct != 50 || got.CrumbleArmor != 50 {
		t.Fatalf("reload changed erosion to %+v", got)
	}
}

func countWorldVisuals(g *MMGame, kind string) int {
	n := 0
	for _, v := range g.spellWorldVisuals {
		if v.kind == kind {
			n++
		}
	}
	return n
}
