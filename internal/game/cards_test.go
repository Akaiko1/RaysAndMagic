package game

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/stash"
	"ugataima/internal/world"
)

// mkTestMonster builds a bare, alive monster with no resistances - the common
// case for card-proc tests that don't care about mitigation.
func mkTestMonster(name string, hp int) *monster.Monster3D {
	return &monster.Monster3D{
		Name: name, HitPoints: hp, MaxHitPoints: hp,
		Resistances: map[monster.DamageType]int{},
	}
}

// The collection aggregates per-card effects, place/remove move cards between the
// party inventory and the 8 slots, and only true cards (items.ItemCard) qualify.
func TestCardCollection_EffectsAndPlacement(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game

	// Effects aggregate straight from the card defs; each card adds only its own field.
	thiefBug, pumaDef := cardDef("thief_bug_card"), cardDef("puma_card")
	if thiefBug.CardMoveSpeedPct <= 0 || pumaDef.CardBonusActions <= 0 {
		t.Fatal("fixture: thief_bug_card must grant move speed and puma_card actions")
	}
	g.cardSlots = [MaxCardSlots]cardSlot{}
	g.cardSlots[0].key = "thief_bug_card"
	g.cardSlots[1].key = "puma_card"
	if got, want := g.cardMoveSpeedPct(), thiefBug.CardMoveSpeedPct+pumaDef.CardMoveSpeedPct; got != want {
		t.Errorf("cardMoveSpeedPct = %d, want %d", got, want)
	}
	if got, want := g.cardBonusActions(), thiefBug.CardBonusActions+pumaDef.CardBonusActions; got != want {
		t.Errorf("cardBonusActions = %d, want %d", got, want)
	}

	// Card type + gating.
	puma := items.CreateItemFromYAML("puma_card")
	if puma.Type != items.ItemCard {
		t.Fatalf("puma_card type = %v, want ItemCard", puma.Type)
	}
	if itemCardKey(puma) != "puma_card" {
		t.Errorf("itemCardKey(puma) = %q, want puma_card", itemCardKey(puma))
	}
	if k := itemCardKey(items.CreateItemFromYAML("granite")); k != "" {
		t.Errorf("granite is a curio, not a card: itemCardKey = %q", k)
	}

	// Place a loose card from inventory into the collection, then take it back.
	g.cardSlots = [MaxCardSlots]cardSlot{}
	g.party.Inventory = append(g.party.Inventory, puma)
	invN := len(g.party.Inventory)
	idxs := g.inventoryCardIndices()
	if len(idxs) == 0 {
		t.Fatal("expected the puma card in inventory")
	}
	if !g.placeCardFromInventory(idxs[len(idxs)-1]) {
		t.Fatal("placeCardFromInventory failed")
	}
	if g.cardSlots[0].key != "puma_card" {
		t.Errorf("slot 0 = %q, want puma_card", g.cardSlots[0].key)
	}
	if len(g.party.Inventory) != invN-1 {
		t.Errorf("inventory should shrink by 1 on place (%d -> %d)", invN, len(g.party.Inventory))
	}
	if g.cardBonusActions() != pumaDef.CardBonusActions {
		t.Errorf("placed puma should grant +%d action, got %d", pumaDef.CardBonusActions, g.cardBonusActions())
	}

	if !g.removeCardToInventory(0) {
		t.Fatal("removeCardToInventory failed")
	}
	if g.cardSlots[0].key != "" {
		t.Errorf("slot 0 should be empty after removal, got %q", g.cardSlots[0].key)
	}
	if g.cardBonusActions() != 0 {
		t.Errorf("no bonus expected after removal, got %d", g.cardBonusActions())
	}
	if len(g.party.Inventory) != invN {
		t.Errorf("inventory should return to %d after take-back, got %d", invN, len(g.party.Inventory))
	}
}

// Batch-A passive effects: aggregation, additive stacking, the Ocelot Speed
// reaching party stats through recomputeStatBonuses, walk-on-water, effect text.
func TestCardEffects_AggregateApplyAndText(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	ocelot, huntress, samurai := cardDef("ocelot_card"), cardDef("masked_huntress_card"), cardDef("samurai_card")
	speed := ocelot.CardStatBonuses["speed"]
	if speed <= 0 || huntress.CardRangedDmgPct <= 0 || samurai.CardMeleeTrueDmg <= 0 || !cardDef("medusa_card").CardWalkOnWater {
		t.Fatal("fixture: ocelot/huntress/samurai/medusa cards lost their effect fields")
	}
	g.cardSlots = [MaxCardSlots]cardSlot{}
	g.cardSlots[0].key = "ocelot_card"          // Speed
	g.cardSlots[1].key = "masked_huntress_card" // ranged %
	g.cardSlots[2].key = "samurai_card"         // true melee
	g.cardSlots[3].key = "medusa_card"          // walk on water

	if g.cardStatBonuses().Speed != speed || g.cardRangedDmgPct() != huntress.CardRangedDmgPct || g.cardMeleeTrueDmg() != samurai.CardMeleeTrueDmg {
		t.Fatalf("aggregates: speed=%d ranged=%d true=%d", g.cardStatBonuses().Speed, g.cardRangedDmgPct(), g.cardMeleeTrueDmg())
	}
	if !g.hasCardWalkOnWater() {
		t.Error("medusa card should grant walk-on-water")
	}

	// Stacking is additive.
	g.cardSlots[4].key = "ocelot_card"
	if g.cardStatBonuses().Speed != 2*speed {
		t.Errorf("two ocelot cards should stack to +%d Speed, got %d", 2*speed, g.cardStatBonuses().Speed)
	}

	// Ocelot Speed reaches the party through the stat-bonus pipeline.
	g.recomputeStatBonuses()
	if g.statBonuses.Speed != 2*speed {
		t.Errorf("party stat Speed bonus = %d, want %d", g.statBonuses.Speed, 2*speed)
	}
	if len(g.party.Members) > 0 && g.party.Members[0].BuffBonuses.Speed != 2*speed {
		t.Errorf("member BuffBonuses.Speed = %d, want %d", g.party.Members[0].BuffBonuses.Speed, 2*speed)
	}

	// Effect text is derived from the card's fields.
	for key, want := range map[string]string{
		"ocelot_card":          fmt.Sprintf("%+d Speed", speed),
		"medusa_card":          "Walk on water",
		"samurai_card":         fmt.Sprintf("+%d true melee damage", samurai.CardMeleeTrueDmg),
		"masked_huntress_card": fmt.Sprintf("+%d%% ranged damage", huntress.CardRangedDmgPct),
	} {
		if got := strings.Join(cardDef(key).CardCollectionLines(), ", "); got != want {
			t.Errorf("collection lines of %s = %q, want %q", key, got, want)
		}
	}

	// One card for every view: the bag/shop card and the collector's card are
	// the same text, the collector only adding its double-click hint, and both
	// list each collection effect on its own line.
	bag := GetItemTooltip(items.CreateItemFromYAML("ocelot_card"), nil, nil, false)
	collector := strings.Join(cardItemTooltipLines("ocelot_card", "Double-click to remove"), "\n")
	if !strings.Contains(bag, fmt.Sprintf("EFFECTS\n%+d Speed", speed)) || strings.Replace(collector, "\nDouble-click to remove", "", 1) != bag {
		t.Errorf("card views differ:\nbag:\n%s\ncollector:\n%s", bag, collector)
	}
	// A card whose effect is not built yet says so in every view.
	stub := &config.ItemDefinitionConfig{Type: "card"}
	if lines := stub.CardCollectionLines(); len(lines) != 1 || lines[0] != "Currently not implemented" {
		t.Errorf("an effectless card shows %v", lines)
	}
}

// Batch-B procs: aggregation, pure helpers (split / reviveHalf), effect text.
func TestCardEffects_BatchB(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	archmage, ningyo, lich, gorilla := cardDef("archmage_card"), cardDef("ningyo_card"), cardDef("lich_card"), cardDef("gorilla_titan_card")
	if archmage.CardPhysToFirePct <= 0 || ningyo.CardHealOnAtkPct <= 0 || ningyo.CardHealAmount <= 0 ||
		lich.CardLethalSavePct <= 0 || gorilla.CardMoveAoePct <= 0 || gorilla.CardMoveAoeDmg <= 0 {
		t.Fatal("fixture: archmage/ningyo/lich/gorilla cards lost their effect fields")
	}
	g.cardSlots = [MaxCardSlots]cardSlot{}
	g.cardSlots[0].key = "archmage_card"
	g.cardSlots[1].key = "ningyo_card"
	g.cardSlots[2].key = "lich_card"
	g.cardSlots[3].key = "gorilla_titan_card"

	if g.cardPhysToFirePct() != archmage.CardPhysToFirePct || g.cardHealOnAttackPct() != ningyo.CardHealOnAtkPct || g.cardHealAmount() != ningyo.CardHealAmount ||
		g.cardLethalSavePct() != lich.CardLethalSavePct || g.cardMoveAoePct() != gorilla.CardMoveAoePct || g.cardMoveAoeDmg() != gorilla.CardMoveAoeDmg {
		t.Fatalf("aggregates wrong: fire=%d heal%%=%d healAmt=%d lethal=%d aoe%%=%d aoeDmg=%d",
			g.cardPhysToFirePct(), g.cardHealOnAttackPct(), g.cardHealAmount(),
			g.cardLethalSavePct(), g.cardMoveAoePct(), g.cardMoveAoeDmg())
	}

	// Archmage split: 25% of physical becomes fire.
	if p, f := splitPhysToFire(100, 25); p != 75 || f != 25 {
		t.Errorf("splitPhysToFire(100,25) = %d/%d, want 75/25", p, f)
	}
	if p, f := splitPhysToFire(50, 0); p != 50 || f != 0 {
		t.Errorf("splitPhysToFire(50,0) = %d/%d, want 50/0", p, f)
	}

	// Lich save restores half HP+SP.
	m := g.party.Members[0]
	m.HitPoints, m.SpellPoints = 0, 0
	reviveHalf(m)
	if m.HitPoints != m.MaxHitPoints/2 || m.SpellPoints != m.MaxSpellPoints/2 {
		t.Errorf("reviveHalf: hp=%d/%d sp=%d/%d", m.HitPoints, m.MaxHitPoints, m.SpellPoints, m.MaxSpellPoints)
	}

	for key, want := range map[string]string{
		"archmage_card":      fmt.Sprintf("%d%% of physical damage dealt as fire", archmage.CardPhysToFirePct),
		"ningyo_card":        fmt.Sprintf("%d%% to self-heal %d on weapon attack", ningyo.CardHealOnAtkPct, ningyo.CardHealAmount),
		"lich_card":          fmt.Sprintf("%d%% to cheat death (half HP+SP)", lich.CardLethalSavePct),
		"gorilla_titan_card": fmt.Sprintf("%d%% on move: %d physical true damage within %g tiles", gorilla.CardMoveAoePct, gorilla.CardMoveAoeDmg, gorilla.CardMoveAoeRadiusTiles),
	} {
		if got := strings.Join(cardDef(key).CardCollectionLines(), ", "); got != want {
			t.Errorf("collection lines of %s = %q, want %q", key, got, want)
		}
	}
}

// AoE lethal damage routes through knockOut, so the Lich Card can cheat death on
// it too - not just plain melee. Fireburst stands in for the AoE/Inferno branches
// that previously set ConditionUnconscious directly. Statistical: with a partial
// save chance over 400 lethal hits, both outcomes must appear (a direct KO would
// never save).
func TestLichCard_SavesOnAoEFireburst(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.cardSlots = [MaxCardSlots]cardSlot{}
	g.cardSlots[0].key = "lich_card"
	if pct := cardDef("lich_card").CardLethalSavePct; g.cardLethalSavePct() != pct || pct <= 0 || pct >= 100 {
		t.Fatalf("lich save pct = %d, want the authored partial chance %d", g.cardLethalSavePct(), pct)
	}
	mon := &monster.Monster3D{Name: "Dragon", FireburstDamageMin: 9999, FireburstDamageMax: 9999}
	member := g.party.Members[0]

	saves := 0
	const trials = 400
	for i := 0; i < trials; i++ {
		member.HitPoints = 1 // fresh, conscious, on the brink each trial
		member.SpellPoints = 0
		member.Conditions = nil
		cs.applyMonsterFireburst(mon)
		if member.HitPoints > 0 { // reviveHalf left HP up -> cheated death (direct KO would be 0)
			saves++
		}
	}
	if saves == 0 {
		t.Fatalf("Lich Card never cheated death over %d AoE Fireburst hits - the AoE branch bypasses knockOut", trials)
	}
	if saves == trials {
		t.Fatalf("every AoE hit cheated death (%d/%d) - the save roll isn't being applied", saves, trials)
	}
}

// Archmage Card splits melee into phys + fire on the PRIMARY; the AoE splash must
// carry the SAME split (full magnitude), not just the physical remainder (the bug
// dropped the fire share, splashing 75 instead of 100).
func TestArchmageCard_SplashGetsFullSplit(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.cardSlots = [MaxCardSlots]cardSlot{}
	g.cardSlots[0].key = "archmage_card"
	if pct := cardDef("archmage_card").CardPhysToFirePct; g.cardPhysToFirePct() != pct || pct <= 0 {
		t.Fatalf("archmage pct = %d, want the authored %d", g.cardPhysToFirePct(), pct)
	}
	ts := float64(g.config.GetTileSize())
	primary := &monster.Monster3D{Name: "Primary", X: 0, Y: 0, HitPoints: 1000, MaxHitPoints: 1000}
	near := &monster.Monster3D{Name: "Near", X: ts, Y: 0, HitPoints: 1000, MaxHitPoints: 1000} // 1 tile, inside 1.5
	g.world.Monsters = []*monster.Monster3D{primary, near}

	// Idol-Breaker: physical mace, aoe_radius_tiles 1.5. No armor/resist on targets,
	// so the splash should land the full 100 (physical remainder + converted fire).
	cs.ApplyDamageToMonster(primary, 100, "Idol-Breaker, the Warlord's Maul", false)

	if got := 1000 - near.HitPoints; got != 100 {
		t.Fatalf("splash dealt %d, want 100 - the fire share is dropped if this is the physical remainder", got)
	}
}

// The Gorilla move-burst follows the party auto-target policy: foes and bound
// former enemies take the hit; summons, charmed monsters and the caravan do not.
func TestCardMoveBurst_FollowsAutoTargetPolicy(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	mk := func(id string, mod func(*monster.Monster3D)) *monster.Monster3D {
		m := &monster.Monster3D{ID: id, Name: id, HitPoints: 100, MaxHitPoints: 100,
			X: g.camera.X, Y: g.camera.Y, Resistances: map[monster.DamageType]int{}}
		mod(m)
		return m
	}
	foe := mk("foe", func(m *monster.Monster3D) {})
	bound := mk("bound", func(m *monster.Monster3D) { m.Bound = true })
	charmed := mk("charmed", func(m *monster.Monster3D) { m.Pacified = true })
	caravan := mk("caravan", func(m *monster.Monster3D) { m.Disposition = monster.DispositionCaravan })
	warded := mk("warded", func(m *monster.Monster3D) { m.BossWarded = true })
	g.world.Monsters = []*monster.Monster3D{foe, bound, charmed, caravan, warded}
	var summons []*monster.Monster3D
	for _, s := range partySummonKinds(g) {
		m := mk(s.kind, func(m *monster.Monster3D) { markPurePartySummon(m, s.owner) })
		if !isPurePartySummon(m) {
			t.Fatalf("%s fixture is not a pure summon", s.kind)
		}
		summons = append(summons, m)
		g.world.Monsters = append(g.world.Monsters, m)
	}

	if !cs.cardMoveBurstApply(50, 5) {
		t.Fatal("burst should report a hit on the foe")
	}
	if foe.HitPoints != 50 {
		t.Errorf("foe should take 50 pure (hp=%d, want 50)", foe.HitPoints)
	}
	if bound.HitPoints != 50 {
		t.Errorf("bound former enemy should take the burst (hp=%d, want 50)", bound.HitPoints)
	}
	for _, m := range summons {
		if m.HitPoints != 100 || m.WasAttacked {
			t.Errorf("%s must be transparent to the burst (hp=%d attacked=%v)", m.Name, m.HitPoints, m.WasAttacked)
		}
	}
	if charmed.HitPoints != 100 || !charmed.Pacified || charmed.WasAttacked {
		t.Errorf("burst must preserve Charm (hp=%d pacified=%v attacked=%v)",
			charmed.HitPoints, charmed.Pacified, charmed.WasAttacked)
	}
	if caravan.HitPoints != 100 || caravan.WasAttacked {
		t.Errorf("burst must not pick the caravan (hp=%d attacked=%v)", caravan.HitPoints, caravan.WasAttacked)
	}
	// Invulnerable boss is skipped entirely - no flash/hit/message, not just 0 damage.
	if warded.HitPoints != 100 || warded.HitTintFrames != 0 {
		t.Errorf("warded boss must be skipped (hp=%d hitTint=%d)", warded.HitPoints, warded.HitTintFrames)
	}
}

// The Gorilla move-burst is physical true damage: it bypasses armor/flat soak,
// but the physical resistance carried by its element still applies.
func TestCardMoveBurst_TrueDamageUsesPhysicalResist(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	resistant := &monster.Monster3D{ID: "res", HitPoints: 100, MaxHitPoints: 100, X: g.camera.X, Y: g.camera.Y,
		Resistances: map[monster.DamageType]int{monster.DamagePhysical: 50}}
	immune := &monster.Monster3D{ID: "imm", HitPoints: 100, MaxHitPoints: 100, X: g.camera.X, Y: g.camera.Y,
		Resistances: map[monster.DamageType]int{monster.DamagePhysical: 100}}
	g.world.Monsters = append(g.world.Monsters, resistant, immune)

	if !cs.cardMoveBurstApply(50, 5) {
		t.Fatal("expected the burst to hit")
	}
	if resistant.HitPoints != 75 {
		t.Errorf("50%% physical-resist mob hp = %d, want 75", resistant.HitPoints)
	}
	if immune.HitPoints != 100 {
		t.Errorf("physical-immune mob hp = %d, want 100", immune.HitPoints)
	}
}

// Every physical summon card owns an independent roll, creature pool, live cap
// and cooldown. Drive the real action proc with 100% authored chances so every
// case is deterministic.
func TestCardSummonSourcesContractTable(t *testing.T) {
	type slottedCard struct {
		key string
		id  uint64
	}
	// The first card may be cooling (spawns nothing) or already at its cap (its
	// live allies stay); every other card summons its own authored limit.
	tests := []struct {
		name      string
		cards     []slottedCard
		coolFirst bool
		capFirst  bool
	}{
		{name: "orc alone", cards: []slottedCard{{"orc_warlord_card", 101}}},
		{name: "lich alone", cards: []slottedCard{{"lich_king_card", 201}}},
		{name: "orc then lich", cards: []slottedCard{{"orc_warlord_card", 101}, {"lich_king_card", 201}}},
		{name: "lich then orc", cards: []slottedCard{{"lich_king_card", 201}, {"orc_warlord_card", 101}}},
		{name: "one cooldown does not block the other", cards: []slottedCard{{"orc_warlord_card", 101}, {"lich_king_card", 201}}, coolFirst: true},
		{name: "one full cap does not block the other", cards: []slottedCard{{"orc_warlord_card", 101}, {"lich_king_card", 201}}, capFirst: true},
		{name: "duplicate physical cards stay independent", cards: []slottedCard{{"orc_warlord_card", 101}, {"orc_warlord_card", 102}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			g := cs.game
			for _, key := range []string{"orc_warlord_card", "lich_king_card"} {
				def := cardDef(key)
				if def == nil {
					t.Fatalf("%s definition missing", key)
				}
				old := def.CardSummonChance
				def.CardSummonChance = 100
				t.Cleanup(func() { def.CardSummonChance = old })
			}
			setupSummonableWorld(t, cs)
			tile := float64(g.config.GetTileSize())
			if _, _, ok := cs.findNearestSummonTile(g.camera.X+2*tile, g.camera.Y, 10); !ok {
				t.Fatal("summonable-world fixture has no free spawn tile")
			}
			g.cardSlots = [MaxCardSlots]cardSlot{}
			for slot, spec := range tt.cards {
				card := items.CreateItemFromYAML(spec.key)
				card.InstanceID = spec.id
				if !g.setCardCollectionSlot(slot, card) {
					t.Fatalf("slot %d rejected %s", slot, spec.key)
				}
			}
			sources := g.cardSummonSources()
			if len(sources) != len(tt.cards) {
				t.Fatalf("sources = %d, want %d", len(sources), len(tt.cards))
			}
			wantByMonster := map[string]int{}
			for i, spec := range tt.cards {
				def := cardDef(spec.key)
				if def.CardSummonMonster == "" || def.CardSummonLimit <= 0 {
					t.Fatalf("fixture: %s is not a summon card", spec.key)
				}
				want := def.CardSummonLimit
				if tt.coolFirst && i == 0 {
					want = 0
				}
				wantByMonster[def.CardSummonMonster] += want
			}
			if tt.coolFirst {
				g.armCardSummonCooldown(sources[0].Owner, 10)
			}
			if tt.capFirst {
				source := sources[0]
				for i := 0; i < source.Limit; i++ {
					m := &monster.Monster3D{Key: source.MonsterKey, ID: source.Owner + fmt.Sprint(i), HitPoints: 1, MaxHitPoints: 1}
					markPurePartySummon(m, source.Owner)
					g.world.Monsters = append(g.world.Monsters, m)
				}
			}

			cs.tryCardSummonOnAction()

			gotByMonster := map[string]int{}
			gotByOwner := map[string]int{}
			for _, m := range g.world.Monsters {
				if m != nil && m.IsAlive() && isCardAlly(m) {
					gotByMonster[m.Key]++
					gotByOwner[m.SummonedBy]++
				}
			}
			for key, want := range wantByMonster {
				if got := gotByMonster[key]; got != want {
					t.Errorf("%s summons = %d, want %d (all=%v)", key, got, want, gotByMonster)
				}
			}
			var logText strings.Builder
			for _, entry := range g.combatLogHistory {
				logText.WriteString(entry.Text)
				logText.WriteByte('\n')
			}
			for i, source := range sources {
				blocked := tt.coolFirst && i == 0
				capped := tt.capFirst && i == 0
				if blocked || capped {
					if gotByOwner[source.Owner] != 0 {
						if blocked {
							t.Errorf("cooling card %s spawned %d allies", source.CardKey, gotByOwner[source.Owner])
						}
					}
					if capped && g.cardSummonCooldown(source.Owner) != 0 {
						t.Errorf("capped card %s spent a cooldown", source.CardKey)
					}
					continue
				}
				if got := gotByOwner[source.Owner]; got != source.Limit {
					t.Errorf("owner %s controls %d allies, want its own limit %d", source.Owner, got, source.Limit)
				}
				if g.cardSummonCooldown(source.Owner) <= 0 {
					t.Errorf("successful %s proc did not arm its cooldown", source.CardKey)
				}
				if !strings.Contains(logText.String(), "The "+source.CardName+" rallies") {
					t.Errorf("successful %s proc did not name its own card in the log: %q", source.CardKey, logText.String())
				}
			}
		})
	}
}

func TestCardSummonNoSpawnSpaceDoesNotSpendCooldown(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	setupSummonableWorld(t, cs)
	def := cardDef("orc_warlord_card")
	if def == nil {
		t.Fatal("orc_warlord_card definition missing")
	}
	oldChance := def.CardSummonChance
	def.CardSummonChance = 100
	t.Cleanup(func() { def.CardSummonChance = oldChance })
	card := items.CreateItemFromYAML("orc_warlord_card")
	card.InstanceID = 301
	if !g.setCardCollectionSlot(0, card) {
		t.Fatal("could not slot Orc Warlord Card")
	}
	for y := range g.world.Tiles {
		for x := range g.world.Tiles[y] {
			g.world.Tiles[y][x] = world.TileWall
		}
	}
	source := g.cardSummonSources()[0]

	cs.tryCardSummonOnAction()

	if got := cs.countLiveSummonsByOwner(source.Owner); got != 0 {
		t.Fatalf("blocked spawn created %d allies, want 0", got)
	}
	if got := g.cardSummonCooldown(source.Owner); got != 0 {
		t.Fatalf("blocked spawn spent cooldown %d, want 0", got)
	}
}

func TestCardSummonCooldownsTickAndExpireIndependently(t *testing.T) {
	g := &MMGame{cardSummonCooldowns: map[string]int{"orc": 2, "lich": 1}}
	g.tickCardSummonCooldowns()
	if got := g.cardSummonCooldown("orc"); got != 1 {
		t.Errorf("orc cooldown = %d, want 1", got)
	}
	if got := g.cardSummonCooldown("lich"); got != 0 {
		t.Errorf("lich cooldown = %d, want expired", got)
	}
	if _, exists := g.cardSummonCooldowns["lich"]; exists {
		t.Error("expired cooldown entry was not removed")
	}

	g = &MMGame{cardSummonCooldowns: map[string]int{cardSummonOwner: 2}}
	g.tickCardSummonCooldowns()
	if got := g.cardSummonCooldown("late-card"); got != 1 {
		t.Errorf("legacy shared cooldown after one tick = %d, want 1", got)
	}
	g.tickCardSummonCooldowns()
	if got := g.cardSummonCooldown("late-card"); got != 0 {
		t.Errorf("expired legacy shared cooldown = %d, want 0", got)
	}
}

func TestCardSummonCollectionSummaryKeepsSourcesSeparate(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	for slot, key := range []string{"orc_warlord_card", "lich_king_card"} {
		card := items.CreateItemFromYAML(key)
		card.InstanceID = uint64(400 + slot)
		if !g.setCardCollectionSlot(slot, card) {
			t.Fatalf("could not slot %s", key)
		}
	}
	lines := g.cardCollectionEffectLines()
	summon := func(chance, limit, cd int) string {
		return fmt.Sprintf("%d%% on action: summon allies (max %d), %ds cooldown", chance, limit, cd)
	}
	want := map[string]bool{}
	var chance, limit, cd int
	for _, key := range []string{"orc_warlord_card", "lich_king_card"} {
		def := cardDef(key)
		want[def.Name+": "+summon(def.CardSummonChance, def.CardSummonLimit, def.CardSummonCDSeconds)] = false
		chance, limit, cd = chance+def.CardSummonChance, limit+def.CardSummonLimit, cd+def.CardSummonCDSeconds
	}
	merged := summon(chance, limit, cd)
	for _, line := range lines {
		if _, ok := want[line]; ok {
			want[line] = true
		}
		if strings.HasSuffix(line, merged) {
			t.Fatalf("summary merged independent summon cards: %q", line)
		}
	}
	for line, seen := range want {
		if !seen {
			t.Errorf("summary missing %q; got %v", line, lines)
		}
	}
}

func TestCardSummonSaveContractTable(t *testing.T) {
	for _, tt := range []struct {
		name           string
		legacy         bool
		orcCooldown    int
		lichCooldown   int
		legacyCooldown int
	}{
		{name: "per-card state", orcCooldown: 11, lichCooldown: 17},
		{name: "legacy shared state migrates conservatively", legacy: true, legacyCooldown: 23},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := loadTestConfig(t)
			wSave := newTestWorld(cfg)
			wmSave := world.NewWorldManager(cfg)
			wmSave.LoadedMaps = map[string]*world.World3D{"forest": wSave}
			wmSave.CurrentMapKey = "forest"
			gSave := newTestGame(cfg, wSave)
			for slot, spec := range []struct {
				key string
				id  uint64
			}{{"orc_warlord_card", 501}, {"lich_king_card", 502}} {
				card := items.CreateItemFromYAML(spec.key)
				card.InstanceID = spec.id
				if !gSave.setCardCollectionSlot(slot, card) {
					t.Fatalf("could not slot %s", spec.key)
				}
			}
			sources := gSave.cardSummonSources()
			owners := map[string]string{}
			for _, source := range sources {
				owners[source.MonsterKey] = source.Owner
			}
			for i, key := range []string{"masked_huntress", "revenant"} {
				m := monster.NewMonster3DFromConfig(float64(64+i*64), 64, key, cfg)
				if m == nil {
					t.Fatalf("%s config missing", key)
				}
				owner := owners[key]
				if tt.legacy {
					owner = cardSummonOwner
				}
				markPurePartySummon(m, owner)
				wSave.Monsters = append(wSave.Monsters, m)
			}
			if !tt.legacy {
				gSave.armCardSummonCooldown(owners["masked_huntress"], tt.orcCooldown)
				gSave.armCardSummonCooldown(owners["revenant"], tt.lichCooldown)
			}
			save := gSave.buildSave(wmSave)
			save.MapKey = "forest"
			if tt.legacy {
				save.CardSummonCooldowns = nil
				save.CardSummonCDFrames = tt.legacyCooldown
			}

			wLoad := newTestWorld(cfg)
			wmLoad := world.NewWorldManager(cfg)
			wmLoad.LoadedMaps = map[string]*world.World3D{"forest": wLoad}
			wmLoad.CurrentMapKey = "forest"
			gLoad := newTestGame(cfg, wLoad)
			if err := gLoad.applySave(wmLoad, &save); err != nil {
				t.Fatalf("apply save: %v", err)
			}
			loadedSources := gLoad.cardSummonSources()
			if len(loadedSources) != 2 {
				t.Fatalf("loaded sources = %d, want 2", len(loadedSources))
			}
			loadedByMonster := map[string]cardSummonSource{}
			for _, source := range loadedSources {
				loadedByMonster[source.MonsterKey] = source
			}
			loadedAllies := map[string]int{}
			for _, m := range wLoad.Monsters {
				if !isCardAlly(m) {
					continue
				}
				loadedAllies[m.Key]++
				source, ok := loadedByMonster[m.Key]
				if !ok {
					t.Fatalf("loaded unexpected card ally %s", m.Key)
				}
				if m.SummonedBy != source.Owner {
					t.Errorf("%s owner = %q, want %q", m.Key, m.SummonedBy, source.Owner)
				}
			}
			for monsterKey, source := range loadedByMonster {
				if got := loadedAllies[monsterKey]; got != 1 {
					t.Errorf("loaded %d %s allies, want 1", got, monsterKey)
				}
				wantCooldown := tt.orcCooldown
				if monsterKey == "revenant" {
					wantCooldown = tt.lichCooldown
				}
				if tt.legacy {
					wantCooldown = tt.legacyCooldown
				}
				if got := gLoad.cardSummonCooldown(source.Owner); got != wantCooldown {
					t.Errorf("%s cooldown = %d, want %d", monsterKey, got, wantCooldown)
				}
			}
			if tt.legacy && !gLoad.loadNeedsResave {
				t.Error("legacy summon state migration did not request a resave")
			}
		})
	}
}

func TestCardSummonCooldownFollowsRemovedCardThroughSave(t *testing.T) {
	cfg := loadTestConfig(t)
	wSave := newTestWorld(cfg)
	wmSave := world.NewWorldManager(cfg)
	wmSave.LoadedMaps = map[string]*world.World3D{"forest": wSave}
	wmSave.CurrentMapKey = "forest"
	gSave := newTestGame(cfg, wSave)
	card := items.CreateItemFromYAML("orc_warlord_card")
	card.InstanceID = 601
	if !gSave.setCardCollectionSlot(0, card) {
		t.Fatal("could not slot Orc Warlord Card")
	}
	owner := gSave.cardSummonSources()[0].Owner
	gSave.armCardSummonCooldown(owner, 19)
	if !gSave.removeCardToInventory(0) {
		t.Fatal("could not remove cooling summon card")
	}
	save := gSave.buildSave(wmSave)
	save.MapKey = "forest"

	wLoad := newTestWorld(cfg)
	wmLoad := world.NewWorldManager(cfg)
	wmLoad.LoadedMaps = map[string]*world.World3D{"forest": wLoad}
	wmLoad.CurrentMapKey = "forest"
	gLoad := newTestGame(cfg, wLoad)
	if err := gLoad.applySave(wmLoad, &save); err != nil {
		t.Fatalf("apply save: %v", err)
	}
	cardIndex := -1
	for i := range gLoad.party.Inventory {
		if gLoad.party.Inventory[i].InstanceID == card.InstanceID {
			cardIndex = i
			break
		}
	}
	if cardIndex < 0 || !gLoad.placeCardFromInventory(cardIndex) {
		t.Fatal("removed physical card did not survive for re-slotting")
	}
	loadedSource := gLoad.cardSummonSources()[0]
	if loadedSource.Owner != owner {
		t.Fatalf("re-slotted card owner = %q, want stable %q", loadedSource.Owner, owner)
	}
	if got := gLoad.cardSummonCooldown(owner); got != 19 {
		t.Fatalf("re-slotted card cooldown = %d, want 19", got)
	}
}

func TestLegacyCardSummonStateMigrationByStorage(t *testing.T) {
	for _, tt := range []struct {
		name     string
		inBag    bool
		inStash  bool
		fullLoad bool
		instance uint64
	}{
		{name: "active collection card", fullLoad: true, instance: 701},
		{name: "removed inventory card", inBag: true, fullLoad: true, instance: 702},
		{name: "shared stash card", inStash: true, instance: 703},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := loadTestConfig(t)
			w := newTestWorld(cfg)
			g := newTestGame(cfg, w)
			card := items.CreateItemFromYAML("orc_warlord_card")
			card.InstanceID = tt.instance
			owner := fmt.Sprintf("%s%d", cardSummonOwnerPrefix, card.InstanceID)
			if tt.inBag {
				g.party.Inventory = append(g.party.Inventory, card)
			} else if tt.inStash {
				g.stash = &stash.Stash{}
				g.stash.CardSlots[0] = card
			} else if !g.setCardCollectionSlot(0, card) {
				t.Fatal("could not slot summon card")
			}
			ally := monster.NewMonster3DFromConfig(64, 64, "masked_huntress", cfg)
			if ally == nil {
				t.Fatal("masked_huntress config missing")
			}
			markPurePartySummon(ally, cardSummonOwner)
			w.Monsters = append(w.Monsters, ally)

			if tt.fullLoad {
				wmSave := world.NewWorldManager(cfg)
				wmSave.LoadedMaps = map[string]*world.World3D{"forest": w}
				wmSave.CurrentMapKey = "forest"
				save := g.buildSave(wmSave)
				save.MapKey = "forest"
				save.CardSummonCooldowns = nil
				save.CardSummonCDFrames = 23

				wLoad := newTestWorld(cfg)
				wmLoad := world.NewWorldManager(cfg)
				wmLoad.LoadedMaps = map[string]*world.World3D{"forest": wLoad}
				wmLoad.CurrentMapKey = "forest"
				g = newTestGame(cfg, wLoad)
				if err := g.applySave(wmLoad, &save); err != nil {
					t.Fatalf("apply legacy save: %v", err)
				}
				w = wLoad
				if len(w.Monsters) != 1 {
					t.Fatalf("loaded monsters = %d, want 1 legacy ally", len(w.Monsters))
				}
				ally = w.Monsters[0]
			} else {
				g.restoreCardSummonState(nil, 23)
			}

			if ally.SummonedBy != owner {
				t.Fatalf("%s legacy ally owner = %q, want physical owner %q", tt.name, ally.SummonedBy, owner)
			}
			if got := g.cardSummonCooldown(owner); got != 23 {
				t.Fatalf("%s migrated cooldown = %d, want 23", tt.name, got)
			}

			if tt.inBag {
				cardIndex := -1
				for i := range g.party.Inventory {
					if g.party.Inventory[i].InstanceID == card.InstanceID {
						cardIndex = i
						break
					}
				}
				if cardIndex < 0 || !g.placeCardFromInventory(cardIndex) {
					t.Fatal("could not re-slot migrated inventory card")
				}
			} else if tt.inStash {
				g.stash.CardSlots[0] = items.Item{}
				g.party.Inventory = append(g.party.Inventory, card)
				if !g.placeCardFromInventory(len(g.party.Inventory) - 1) {
					t.Fatal("could not re-slot migrated stash card")
				}
			}

			def := cardDef("orc_warlord_card")
			oldChance := def.CardSummonChance
			def.CardSummonChance = 100
			t.Cleanup(func() { def.CardSummonChance = oldChance })
			g.combat = NewCombatSystem(g)
			setupSummonableWorld(t, g.combat)
			g.world.Monsters = append(g.world.Monsters, ally)
			g.combat.tryCardSummonOnAction()
			if got := g.combat.countLiveSummonsByOwner(owner); got != 1 {
				t.Fatalf("%s action during migrated cooldown left %d allies, want 1", tt.name, got)
			}
		})
	}
}

func TestLegacyCardSummonCooldownSurvivesWithoutOwnedCard(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorld(cfg))
	g.restoreCardSummonState(nil, 31)
	saved := g.snapshotCardSummonCooldowns()
	if saved[cardSummonOwner] != 31 {
		t.Fatalf("legacy fallback snapshot = %d, want 31", saved[cardSummonOwner])
	}

	loaded := newTestGame(cfg, newTestWorld(cfg))
	loaded.restoreCardSummonState(saved, 0)
	card := items.CreateItemFromYAML("orc_warlord_card")
	card.InstanceID = 704
	if !loaded.setCardCollectionSlot(0, card) {
		t.Fatal("could not activate card after legacy load")
	}
	source := loaded.cardSummonSources()[0]
	if got := loaded.cardSummonCooldown(source.Owner); got != 31 {
		t.Fatalf("late-activated card cooldown = %d, want legacy fallback 31", got)
	}
	def := cardDef("orc_warlord_card")
	oldChance := def.CardSummonChance
	def.CardSummonChance = 100
	t.Cleanup(func() { def.CardSummonChance = oldChance })
	loaded.combat = NewCombatSystem(loaded)
	setupSummonableWorld(t, loaded.combat)
	loaded.combat.tryCardSummonOnAction()
	if got := loaded.combat.countLiveSummonsByOwner(source.Owner); got != 0 {
		t.Fatalf("late-activated card spawned %d allies during legacy fallback", got)
	}
}

func TestLegacyCardSummonAlliesDistributeAcrossDuplicatePhysicalCards(t *testing.T) {
	cfg := loadTestConfig(t)
	w := newTestWorld(cfg)
	g := newTestGame(cfg, w)
	for i, id := range []uint64{705, 706} {
		card := items.CreateItemFromYAML("orc_warlord_card")
		card.InstanceID = id
		g.party.Inventory = append(g.party.Inventory, card)
		for n := 0; n < 2; n++ {
			ally := monster.NewMonster3DFromConfig(float64(64+i*32+n), 64, "masked_huntress", cfg)
			markPurePartySummon(ally, cardSummonOwner)
			w.Monsters = append(w.Monsters, ally)
		}
	}
	g.restoreCardSummonState(nil, 0)

	got := map[string]int{}
	for _, ally := range w.Monsters {
		got[ally.SummonedBy]++
	}
	for _, id := range []uint64{705, 706} {
		owner := fmt.Sprintf("%s%d", cardSummonOwnerPrefix, id)
		if got[owner] != 2 {
			t.Errorf("duplicate physical owner %q received %d allies, want 2", owner, got[owner])
		}
	}
}

// resetCardCollection (called from startNewGameWithParty) clears the collection
// and recomputes stats, so a fresh party never inherits the old run's effects.
func TestResetCardCollection_ClearsEffects(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.cardSlots[0].key = "thief_bug_card"
	g.cardSlots[1].key = "ocelot_card"
	g.recomputeStatBonuses()
	if g.cardMoveSpeedPct() == 0 || g.cardStatBonuses().Speed == 0 || g.statBonuses.Speed == 0 {
		t.Fatal("precondition: cards should be active first")
	}

	g.resetCardCollection()

	for i := 0; i < MaxCardSlots; i++ {
		if g.cardCollectionKey(i) != "" {
			t.Errorf("collection slot %d should be empty, got %q", i, g.cardCollectionKey(i))
		}
	}
	if g.cardMoveSpeedPct() != 0 || g.cardStatBonuses().Speed != 0 {
		t.Errorf("card effects should be gone: move=%d speed=%d", g.cardMoveSpeedPct(), g.cardStatBonuses().Speed)
	}
	if g.statBonuses.Speed != 0 {
		t.Errorf("party stat bonus should recompute to 0, got %d", g.statBonuses.Speed)
	}
}

// All loose cards are enumerated (even past one collector page) so pagination can
// reach every one - the fixture holds more cards than cardInvMaxShown.
func TestInventoryCardIndices_EnumeratesPastOnePage(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.party.Inventory = nil
	keys := []string{
		"medusa_card", "puma_card", "archmage_card", "lich_card", "thief_bug_card",
		"samurai_card", "ningyo_card", "ocelot_card", "gorilla_titan_card",
		"masked_huntress_card", "orc_warlord_card",
	}
	for _, k := range keys {
		g.party.Inventory = append(g.party.Inventory, items.CreateItemFromYAML(k))
	}
	if got := len(g.inventoryCardIndices()); got != len(keys) {
		t.Errorf("inventoryCardIndices = %d, want %d (all loose cards across pages)", got, len(keys))
	}
	if len(keys) <= cardInvMaxShown {
		t.Fatalf("test premise broken: %d cards must exceed one page (%d)", len(keys), cardInvMaxShown)
	}
}

// The 8-slot collection survives a save round-trip (and stale slots clear).
func TestSaveLoad_PersistsCardCollection(t *testing.T) {
	cfg := loadTestConfig(t)
	wm := world.NewWorldManager(cfg)
	w := newTestWorld(cfg)
	wm.LoadedMaps = map[string]*world.World3D{"forest": w}
	wm.CurrentMapKey = "forest"

	game := newTestGame(cfg, w)
	game.cardSlots[0].key = "thief_bug_card"
	game.cardSlots[3].key = "puma_card"

	save := game.buildSave(wm)

	loaded := newTestGame(cfg, w)
	loaded.cardSlots[1].key = "lich_card" // should be wiped by restore
	if err := loaded.applySave(wm, &save); err != nil {
		t.Fatalf("apply save: %v", err)
	}
	if loaded.cardSlots[0].key != "thief_bug_card" || loaded.cardSlots[3].key != "puma_card" {
		t.Fatalf("collection not restored: %v", loaded.cardSlots)
	}
	if loaded.cardSlots[1].key != "" {
		t.Errorf("stale slot should clear on restore, got %q", loaded.cardSlots[1].key)
	}
	if loaded.cardMoveSpeedPct() != cardDef("thief_bug_card").CardMoveSpeedPct || loaded.cardBonusActions() != cardDef("puma_card").CardBonusActions {
		t.Errorf("restored effects wrong: speed=%d actions=%d", loaded.cardMoveSpeedPct(), loaded.cardBonusActions())
	}
}

func TestCardCollection_PreservesInstanceIDWhenRemoved(t *testing.T) {
	loadTestConfig(t)
	g := &MMGame{party: &character.Party{}}
	card := items.CreateItemFromYAML("puma_card")
	card.InstanceID = 123
	g.party.Inventory = []items.Item{card}

	if !g.placeCardFromInventory(0) {
		t.Fatal("placeCardFromInventory failed")
	}
	if len(g.party.Inventory) != 0 {
		t.Fatalf("card should leave inventory, got %+v", g.party.Inventory)
	}
	if got := g.cardSlots[0].item.InstanceID; got != 123 {
		t.Fatalf("collection card id = %d, want 123", got)
	}

	if !g.removeCardToInventory(0) {
		t.Fatal("removeCardToInventory failed")
	}
	if len(g.party.Inventory) != 1 || g.party.Inventory[0].InstanceID != 123 {
		t.Fatalf("removed card should keep original id, got %+v", g.party.Inventory)
	}
}

func TestSaveLoad_LegacyCardCollectionClearsStashOwnedKey(t *testing.T) {
	cfg := loadTestConfig(t)
	wm := world.NewWorldManager(cfg)
	w := newTestWorld(cfg)
	wm.LoadedMaps = map[string]*world.World3D{"forest": w}
	wm.CurrentMapKey = "forest"

	source := newTestGame(cfg, w)
	source.cardSlots[0].key = "puma_card"
	save := source.buildSave(wm)
	save.Party.CardCollection = []string{"puma_card"}
	save.Party.CardCollectionItems = nil // pre-InstanceID collection save

	owned := items.CreateItemFromYAML("puma_card")
	owned.InstanceID = 777
	loaded := newTestGame(cfg, w)
	loaded.stash = &stash.Stash{}
	loaded.stash.CardSlots[0] = owned

	if err := loaded.applySave(wm, &save); err != nil {
		t.Fatalf("apply save: %v", err)
	}
	if key := loaded.cardCollectionKey(0); key != "" {
		t.Fatalf("legacy card collection should clear key already owned by stash, got %q", key)
	}
	if loaded.cardMoveSpeedPct() != 0 {
		t.Fatalf("cleared legacy card should not grant speed, got %d", loaded.cardMoveSpeedPct())
	}
}

// Every authored card must parse into a real collection effect - catches a
// typo'd card_* field name or a key mismatch.
func TestEveryCardHasARealEffect(t *testing.T) {
	newTestCombatSystemWithConfig(t)
	checked := 0
	for _, key := range slices.Sorted(maps.Keys(config.GlobalItems.Items)) {
		if config.GlobalItems.Items[key].Type != "card" {
			continue
		}
		checked++
		def := cardDef(key)
		if def == nil {
			t.Errorf("%s: no card definition found", key)
			continue
		}
		if len(def.CardEffectLines()) == 0 {
			t.Errorf("%s has no real collection effect", key)
		}
	}
	if checked == 0 {
		t.Fatal("no authored cards")
	}
}

// On-hit card procs roll their authored chance: over many hits both outcomes
// must appear (never and always both mean the roll is not applied).
func TestCardOnHitProcsTriggerSometimes(t *testing.T) {
	for _, tc := range []struct {
		card   string
		trials int
		pct    func(*MMGame) int
		field  func(*config.ItemDefinitionConfig) int
		hit    func(*CombatSystem) bool
	}{
		{
			// Disintegrate instantly zeroes HP; the target is neither undead nor a dragon.
			card: "alien_card", trials: 500,
			pct:   (*MMGame).cardDisintegratePct,
			field: func(d *config.ItemDefinitionConfig) int { return d.CardDisintegratePct },
			hit: func(cs *CombatSystem) bool {
				m := mkTestMonster("Skeleton Warrior", 1000)
				cs.ApplyDamageToMonster(m, 10, "Idol-Breaker, the Warlord's Maul", false)
				return m.HitPoints == 0
			},
		},
		{
			// Armor pierce lands the full hit on a heavily armored target.
			card: "forest_orc_card", trials: 400,
			pct:   (*MMGame).cardArmorPiercePct,
			field: func(d *config.ItemDefinitionConfig) int { return d.CardArmorPiercePct },
			hit: func(cs *CombatSystem) bool {
				m := mkTestMonster("Armored Target", 100000)
				m.ArmorClass = 200
				before := m.HitPoints
				cs.ApplyDamageToMonster(m, 100, "Idol-Breaker, the Warlord's Maul", false)
				return before-m.HitPoints >= 100
			},
		},
		{
			// Stun-on-hit reuses the weapon stun-DR path.
			card: "minotaur_card", trials: 400,
			pct:   (*MMGame).cardStunOnHitPct,
			field: func(d *config.ItemDefinitionConfig) int { return d.CardStunOnHitPct },
			hit: func(cs *CombatSystem) bool {
				m := mkTestMonster("Target", 100000)
				cs.tryApplyWeaponStun(m, nil)
				return m.StunFramesRemaining > 0 || m.StunTurnsRemaining > 0
			},
		},
	} {
		t.Run(tc.card, func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			g := cs.game
			g.cardSlots = [MaxCardSlots]cardSlot{}
			g.cardSlots[0].key = tc.card
			if want := tc.field(cardDef(tc.card)); tc.pct(g) != want || want <= 0 || want >= 100 {
				t.Fatalf("proc pct = %d, want the authored partial chance %d", tc.pct(g), want)
			}
			hits := 0
			for i := 0; i < tc.trials; i++ {
				if tc.hit(cs) {
					hits++
				}
			}
			if hits == 0 || hits == tc.trials {
				t.Fatalf("proc fired %d/%d times, want some but not all", hits, tc.trials)
			}
		})
	}
}

// Golden Thief Bug Card: its flat fire resist through mitigateCharacterDamage
// hits the existing >=100 immunity clamp - full fire immunity.
func TestGoldenThiefBugCard_FireImmunity(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.cardSlots = [MaxCardSlots]cardSlot{}
	g.cardSlots[0].key = "golden_thief_bug_card"
	want := cardDef("golden_thief_bug_card").CardResistBonus["fire"]
	if got := g.cardResistBonusFor("fire"); got != want || want < 100 {
		t.Fatalf("cardResistBonusFor(fire) = %d, want the authored immunity %d (>= 100)", got, want)
	}
	member := g.party.Members[0]
	if got := cs.mitigateCharacterDamage(500, "fire", member, false); got != 0 {
		t.Errorf("fire damage through GTB card = %d, want 0 (immune)", got)
	}
	if got := cs.mitigateCharacterDamage(500, "physical", member, false); got == 0 {
		t.Error("physical damage should NOT be blocked by a fire-only ward")
	}
}

// Every resist card grants its authored flat bonus to its own elements only,
// and a base and elder card of the same element stack.
func TestDragonCards_ResistBonusPerElement(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	checked := 0
	for _, key := range slices.Sorted(maps.Keys(config.GlobalItems.Items)) {
		def := cardDef(key)
		if def == nil || len(def.CardResistBonus) == 0 {
			continue
		}
		checked++
		g.cardSlots = [MaxCardSlots]cardSlot{}
		g.cardSlots[0].key = key
		for _, school := range damagecalc.Types() {
			element := school.String()
			if got, want := g.cardResistBonusFor(element), def.CardResistBonus[element]; got != want {
				t.Errorf("%s resist(%s) = %d, want %d", key, element, got, want)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no authored resist cards")
	}

	// Base + elder of the same color stack.
	red, elder := cardDef("dragon_red_card"), cardDef("elder_dragon_red_card")
	stacked := 0
	for element, v := range red.CardResistBonus {
		if elder.CardResistBonus[element] == 0 {
			continue
		}
		stacked++
		g.cardSlots = [MaxCardSlots]cardSlot{}
		g.cardSlots[0].key = "dragon_red_card"
		g.cardSlots[1].key = "elder_dragon_red_card"
		if got, want := g.cardResistBonusFor(element), v+elder.CardResistBonus[element]; got != want {
			t.Errorf("stacked red dragon %s resist = %d, want %d", element, got, want)
		}
	}
	if stacked == 0 {
		t.Fatal("fixture: red dragon cards share no element")
	}
}

// Jungle Goblin Card raises the gold a kill drops by card_gold_find_pct.
func TestJungleGoblinCard_DoublesGold(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.cardSlots = [MaxCardSlots]cardSlot{}
	g.cardSlots[0].key = "jungle_goblin_card"
	pct := cardDef("jungle_goblin_card").CardGoldFindPct
	if g.cardGoldFindPct() != pct || pct <= 0 {
		t.Fatalf("cardGoldFindPct = %d, want the authored %d", g.cardGoldFindPct(), pct)
	}

	m := mkTestMonster("Goblin", 10)
	m.Gold = 40
	g.world.Monsters = []*monster.Monster3D{m}
	cs.ApplyDamageToMonster(m, 1000, "Idol-Breaker, the Warlord's Maul", false)

	if len(g.groundContainers) == 0 {
		t.Fatal("expected a loot bag to spawn")
	}
	if got, want := g.groundContainers[len(g.groundContainers)-1].Gold, m.Gold*(100+pct)/100; got != want {
		t.Errorf("dropped gold = %d, want %d (%d +%d%%)", got, want, m.Gold, pct)
	}
}

// Jungle Idol Card grants flat party max HP, applied through the same push
// used for BuffBonuses (applyPartyStatBonuses).
func TestJungleIdolCard_MaxHPBonus(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	member := g.party.Members[0]
	before := member.MaxHitPoints
	bonus := cardDef("jungle_idol_card").CardMaxHPBonus
	if bonus <= 0 {
		t.Fatal("fixture: jungle_idol_card grants no max HP")
	}

	g.cardSlots = [MaxCardSlots]cardSlot{}
	g.cardSlots[0].key = "jungle_idol_card"
	g.recomputeStatBonuses()
	if got := member.MaxHitPoints - before; got != bonus {
		t.Errorf("max HP delta = %d, want %d", got, bonus)
	}

	// Removing the card must give it back.
	g.cardSlots = [MaxCardSlots]cardSlot{}
	g.recomputeStatBonuses()
	if member.MaxHitPoints != before {
		t.Errorf("max HP after removal = %d, want back to %d", member.MaxHitPoints, before)
	}
}

// Troll Cards regenerate a % of max HP once per regen-tick cadence (RT).
func TestTrollCards_RegenPct(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	member := g.party.Members[0]

	g.cardSlots = [MaxCardSlots]cardSlot{}
	g.cardSlots[0].key = "troll_card"
	g.cardSlots[1].key = "mountain_troll_card"
	g.recomputeStatBonuses()
	regen := cardDef("troll_card").CardRegenPct + cardDef("mountain_troll_card").CardRegenPct
	if g.cardRegenPct() != regen || regen <= 0 {
		t.Fatalf("cardRegenPct = %d, want the stacked authored %d", g.cardRegenPct(), regen)
	}

	member.MaxHitPoints = 1000
	member.HitPoints = 500
	for i := 0; i < character.ManaRegenIntervalFrames; i++ {
		member.UpdateWithMode(false)
	}
	if want := 500 + 1000*regen/100; member.HitPoints != want {
		t.Errorf("HP after one regen tick = %d, want %d (500 + %d%% of 1000)", member.HitPoints, want, regen)
	}
}

// Regression: the Troll Card's HP regen only lived in the RT-only
// updateRegenAndPoison - UpdateWithMode(true) (TB) returned before reaching it,
// so equipping the card and fighting in turn-based mode silently regenerated
// nothing all fight. The TB path now ticks it via ApplyCardRegenTick on the
// same round counter as SP regen (endPartyTurn).
func TestTrollCard_RegensInTurnBasedMode(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	member := g.party.Members[0]
	member.MaxHitPoints, member.HitPoints = 1000, 500
	member.BonusRegenPct = 5 // as if a Troll Card were active

	for i := 0; i < TurnBasedSpRegenEveryNRounds; i++ {
		g.endPartyTurn()
	}
	if member.HitPoints != 550 {
		t.Errorf("HP after %d TB rounds = %d, want 550 (500 + 5%% of 1000)", TurnBasedSpRegenEveryNRounds, member.HitPoints)
	}
}

// vengefulNingyoThornsPct slots the card and returns its authored reflect share.
func vengefulNingyoThornsPct(t *testing.T, g *MMGame) int {
	t.Helper()
	g.cardSlots = [MaxCardSlots]cardSlot{}
	g.cardSlots[0].key = "vengeful_ningyo_card"
	pct := cardDef("vengeful_ningyo_card").CardThornsPct
	if g.cardThornsPct() != pct || pct <= 0 {
		t.Fatalf("cardThornsPct = %d, want the authored %d", g.cardThornsPct(), pct)
	}
	return pct
}

// Vengeful Ningyo Card reflects a flat % of incoming damage back at the
// attacking monster.
func TestVengefulNingyoCard_Thorns(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	vengefulNingyoThornsPct(t, g)

	attacker := mkTestMonster("Bandit", 1000)
	member := g.party.Members[0]
	member.HitPoints, member.MaxHitPoints = 500, 500
	member.Luck = 0 // deterministic: no Perfect Dodge so the hit (and thorns) always lands

	cs.monsterHitCharacter(attacker, member, "Bandit", hitFromMonster(attacker, 100, "physical", false, 0, false, false))
	if attacker.HitPoints >= 1000 {
		t.Errorf("attacker HP = %d, should have taken thorns reflect damage", attacker.HitPoints)
	}
}

// Vengeful Ningyo Card: a reflect that kills the attacking monster must still
// run kill finalization (XP/loot/quest credit), not just zero its HP.
func TestVengefulNingyoCard_ThornsKillFinalizesKill(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	pct := vengefulNingyoThornsPct(t, g)

	// The reflected share of a 100 hit must exceed the attacker's HP.
	hp := 100*pct/100 - 2
	if hp <= 0 {
		t.Fatalf("fixture: a %d%% reflect of 100 cannot kill a living attacker", pct)
	}
	attacker := mkTestMonster("Weak Attacker", hp)
	attacker.Experience = 50
	member := g.party.Members[0]
	member.HitPoints, member.MaxHitPoints = 500, 500
	member.Luck = 0 // deterministic: no Perfect Dodge so the hit (and thorns) always lands

	before := len(g.deadMonsterIDs)
	cs.monsterHitCharacter(attacker, member, "Weak Attacker", hitFromMonster(attacker, 100, "physical", false, 0, false, false))
	if attacker.IsAlive() {
		t.Fatal("setup: reflected damage should have killed the attacker")
	}
	if len(g.deadMonsterIDs) != before+1 {
		t.Error("thorns kill should register in deadMonsterIDs via finishMonsterKill")
	}
}

// Hexer/Isis Cards divert a share of physical damage into dark/light instead -
// same split mechanism as Archmage's fire conversion, different element.
func TestHexerIsisCards_ElementConversion(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game

	g.cardSlots = [MaxCardSlots]cardSlot{}
	g.cardSlots[0].key = "masked_hexer_girl_card" // physical -> dark
	dark := mkTestMonster("Target", 1000)
	cs.ApplyDamageToMonster(dark, 100, "Idol-Breaker, the Warlord's Maul", false)
	if got := 1000 - dark.HitPoints; got != 100 {
		t.Errorf("hexer split total damage = %d, want 100 conserved", got)
	}

	g.cardSlots = [MaxCardSlots]cardSlot{}
	g.cardSlots[0].key = "isis_card" // physical -> light, melee AND ranged
	light := mkTestMonster("Target2", 1000)
	cs.ApplyDamageToMonster(light, 100, "Idol-Breaker, the Warlord's Maul", false)
	if got := 1000 - light.HitPoints; got != 100 {
		t.Errorf("isis split total damage = %d, want 100 conserved", got)
	}
}

// Elf Archer / Skeleton Cards: card-driven bonus_vs, matched by monster Name
// (dragon) and by MonsterType (formless bosses) respectively.
func TestElfArcherSkeletonCards_BonusVs(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	vsDragon := cardDef("elf_archer_card").CardBonusVs["dragon"]
	vsFormless := cardDef("skeleton_card").CardBonusVs["formless"]
	if vsDragon <= 1 || vsFormless <= 1 {
		t.Fatalf("fixture: elf archer vs dragon %.2f, skeleton vs formless %.2f must both boost", vsDragon, vsFormless)
	}

	g.cardSlots = [MaxCardSlots]cardSlot{}
	g.cardSlots[0].key = "elf_archer_card"
	dragon := &monster.Monster3D{Name: "Dragon", Key: "dragon_red"}
	if got := g.cardBonusVsMultiplier(dragon); got != vsDragon {
		t.Errorf("elf archer vs dragon mult = %.2f, want %.2f", got, vsDragon)
	}
	notDragon := &monster.Monster3D{Name: "Goblin", Key: "goblin"}
	if got := g.cardBonusVsMultiplier(notDragon); got != 1.0 {
		t.Errorf("elf archer vs goblin mult = %.2f, want 1.0", got)
	}

	g.cardSlots = [MaxCardSlots]cardSlot{}
	g.cardSlots[0].key = "skeleton_card"
	boss := &monster.Monster3D{Name: "Golden Thief Bug", MonsterType: "formless"}
	if got := g.cardBonusVsMultiplier(boss); got != vsFormless {
		t.Errorf("skeleton vs formless mult = %.2f, want %.2f", got, vsFormless)
	}
}

// Regression: Name/Key/MonsterType often name the same identity - the real
// Dragon monster (assets/monsters.yaml) has Name="Dragon", Key="dragon", AND
// MonsterType="dragon". A single card_bonus_vs: {dragon: x} entry matched all
// three candidate fields and multiplied in three times (x^3) instead of once -
// one matching entry means "this card applies," not "multiply once per field
// that happened to match."
func TestCardBonusVs_SameIdentityAcrossFieldsAppliesOnce(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.cardSlots = [MaxCardSlots]cardSlot{}
	g.cardSlots[0].key = "elf_archer_card"
	want := cardDef("elf_archer_card").CardBonusVs["dragon"]
	if want <= 1 {
		t.Fatalf("fixture: elf archer vs dragon %.2f must boost", want)
	}

	dragon := &monster.Monster3D{Name: "Dragon", Key: "dragon", MonsterType: "dragon"}
	if got := g.cardBonusVsMultiplier(dragon); got != want {
		t.Errorf("elf archer vs a Dragon whose Name/Key/MonsterType all read \"dragon\" = %.4f, want %.4f (single application)", got, want)
	}
}

// Mummy Card grants full resistance to the monster-inflicted poison proc.
func TestMummyCard_PoisonImmunity(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.cardSlots = [MaxCardSlots]cardSlot{}
	g.cardSlots[0].key = "mummy_card"
	want := cardDef("mummy_card").CardPoisonResistPct
	if g.cardPoisonResistPct() != want || want < 100 {
		t.Fatalf("cardPoisonResistPct = %d, want the authored immunity %d (>= 100)", g.cardPoisonResistPct(), want)
	}

	m := &monster.Monster3D{Name: "Rat", PoisonChance: 1.0, PoisonDurationSec: 10}
	member := g.party.Members[0]
	member.Conditions = nil
	for i := 0; i < 50; i++ {
		cs.tryApplyMonsterPoison(m, member)
	}
	if member.HasCondition(character.ConditionPoisoned) {
		t.Error("mummy card should have fully resisted every poison roll")
	}
}

// Rat/Spider Cards inflict a real poison DoT on the STRUCK MONSTER (not the
// party) - a genuinely new status, ticking HP down over time via TickPoison.
// Stacked cards add their chances and keep the longest duration.
func TestRatSpiderCards_PoisonsMonsterOnHit(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	rat := cardDef("rat_card")
	if rat.CardPoisonProcPct <= 0 || rat.CardPoisonProcPct >= 100 || rat.CardPoisonDurationSec <= 0 {
		t.Fatalf("fixture: rat_card proc %d%%/%ds must be a partial chance with a duration", rat.CardPoisonProcPct, rat.CardPoisonDurationSec)
	}
	g.cardSlots = [MaxCardSlots]cardSlot{}
	g.cardSlots[0].key = "rat_card"
	if pct, dur := g.cardPoisonProc(); pct != rat.CardPoisonProcPct || dur != rat.CardPoisonDurationSec {
		t.Fatalf("cardPoisonProc = %d%%/%ds, want %d%%/%ds", pct, dur, rat.CardPoisonProcPct, rat.CardPoisonDurationSec)
	}
	wantPct, wantDur := rat.CardPoisonProcPct, rat.CardPoisonDurationSec
	for slot, key := range []string{"spider_card", "forest_spider_card"} {
		def := cardDef(key)
		g.cardSlots[slot+1].key = key
		wantPct += def.CardPoisonProcPct
		wantDur = max(wantDur, def.CardPoisonDurationSec)
	}
	if pct, dur := g.cardPoisonProc(); pct != wantPct || dur != wantDur {
		t.Fatalf("stacked cardPoisonProc = %d%%/%ds, want %d%%/%ds", pct, dur, wantPct, wantDur)
	}
	g.cardSlots[1].key, g.cardSlots[2].key = "", ""

	poisoned := 0
	const trials = 300
	var m *monster.Monster3D
	for i := 0; i < trials; i++ {
		m = mkTestMonster("Target", 1000)
		cs.tryCardPoisonProc(m)
		if m.PoisonedFramesRemaining > 0 {
			poisoned++
		}
	}
	if poisoned == 0 {
		t.Fatalf("poison proc never triggered over %d hits", trials)
	}
	if poisoned == trials {
		t.Fatalf("poison proc triggered on every hit (%d/%d) - should be ~%d%%", poisoned, trials, rat.CardPoisonProcPct)
	}

	// A poisoned monster loses HP over (simulated) time via TickPoison.
	tps := config.GetTargetTPS()
	m = mkTestMonster("Ticking", 1000)
	m.ApplyPoison(tps * 2) // 2 seconds of poison
	before := m.HitPoints
	for i := 0; i < tps; i++ { // 1 second - one tick should have fired
		m.TickPoison()
	}
	if m.HitPoints >= before {
		t.Errorf("poisoned monster HP = %d, should have dropped below %d after 1s of ticking", m.HitPoints, before)
	}
}

// Ronin Marksman / Bat Cards: flat crit/dodge bonuses feed the existing rolls.
func TestRoninBatCards_CritAndDodgeBonus(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	member := g.party.Members[0]
	crit, dodge := cardDef("ronin_marksman_card").CardCritBonusPct, cardDef("bat_card").CardDodgeBonusPct
	if crit <= 0 || dodge <= 0 {
		t.Fatal("fixture: ronin_marksman_card must add crit and bat_card dodge")
	}

	baseCrit := cs.CalculateCriticalChance(member)
	g.cardSlots = [MaxCardSlots]cardSlot{}
	g.cardSlots[0].key = "ronin_marksman_card"
	if got := cs.CalculateCriticalChance(member) - baseCrit; got != crit {
		t.Errorf("crit bonus delta = %d, want %d", got, crit)
	}

	_, baseDodge := cs.RollPerfectDodge(member)
	g.cardSlots = [MaxCardSlots]cardSlot{}
	g.cardSlots[0].key = "bat_card"
	_, afterDodge := cs.RollPerfectDodge(member)
	if got := afterDodge - baseDodge; got != dodge {
		t.Errorf("dodge bonus delta = %d, want %d", got, dodge)
	}
}
