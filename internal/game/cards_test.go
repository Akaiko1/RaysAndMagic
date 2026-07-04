package game

import (
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

// mkTestMonster builds a bare, alive monster with no resistances — the common
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

	// Effects aggregate straight from the card defs.
	g.cardCollection = [MaxCardSlots]string{}
	g.cardCollection[0] = "thief_bug_card"
	g.cardCollection[1] = "puma_card"
	if got := g.cardMoveSpeedPct(); got != 25 {
		t.Errorf("cardMoveSpeedPct = %d, want 25", got)
	}
	if got := g.cardBonusActions(); got != 1 {
		t.Errorf("cardBonusActions = %d, want 1", got)
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
	g.cardCollection = [MaxCardSlots]string{}
	g.party.Inventory = append(g.party.Inventory, puma)
	invN := len(g.party.Inventory)
	idxs := g.inventoryCardIndices()
	if len(idxs) == 0 {
		t.Fatal("expected the puma card in inventory")
	}
	if !g.placeCardFromInventory(idxs[len(idxs)-1]) {
		t.Fatal("placeCardFromInventory failed")
	}
	if g.cardCollection[0] != "puma_card" {
		t.Errorf("slot 0 = %q, want puma_card", g.cardCollection[0])
	}
	if len(g.party.Inventory) != invN-1 {
		t.Errorf("inventory should shrink by 1 on place (%d -> %d)", invN, len(g.party.Inventory))
	}
	if g.cardBonusActions() != 1 {
		t.Errorf("placed puma should grant +1 action, got %d", g.cardBonusActions())
	}

	if !g.removeCardToInventory(0) {
		t.Fatal("removeCardToInventory failed")
	}
	if g.cardCollection[0] != "" {
		t.Errorf("slot 0 should be empty after removal, got %q", g.cardCollection[0])
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
	g.cardCollection = [MaxCardSlots]string{}
	g.cardCollection[0] = "ocelot_card"          // +15 Speed
	g.cardCollection[1] = "masked_huntress_card" // +20% ranged
	g.cardCollection[2] = "samurai_card"         // +20 true melee
	g.cardCollection[3] = "medusa_card"          // walk on water

	if g.cardStatBonuses().Speed != 15 || g.cardRangedDmgPct() != 20 || g.cardMeleeTrueDmg() != 20 {
		t.Fatalf("aggregates: speed=%d ranged=%d true=%d", g.cardStatBonuses().Speed, g.cardRangedDmgPct(), g.cardMeleeTrueDmg())
	}
	if !g.hasCardWalkOnWater() {
		t.Error("medusa card should grant walk-on-water")
	}

	// Stacking is additive.
	g.cardCollection[4] = "ocelot_card"
	if g.cardStatBonuses().Speed != 30 {
		t.Errorf("two ocelot cards should stack to +30 Speed, got %d", g.cardStatBonuses().Speed)
	}

	// Ocelot Speed reaches the party through the stat-bonus pipeline.
	g.recomputeStatBonuses()
	if g.statBonuses.Speed != 30 {
		t.Errorf("party stat Speed bonus = %d, want 30", g.statBonuses.Speed)
	}
	if len(g.party.Members) > 0 && g.party.Members[0].BuffBonuses.Speed != 30 {
		t.Errorf("member BuffBonuses.Speed = %d, want 30", g.party.Members[0].BuffBonuses.Speed)
	}

	// Effect text is derived from the card's fields.
	for key, want := range map[string]string{
		"ocelot_card":          "+15 Speed",
		"medusa_card":          "Walk on water",
		"samurai_card":         "+20 true melee damage",
		"masked_huntress_card": "+20% ranged damage",
	} {
		if got := cardEffectText(cardDef(key)); got != want {
			t.Errorf("cardEffectText(%s) = %q, want %q", key, got, want)
		}
	}

	// The effect also appears in the item tooltip (EffectLines), so hovering a
	// loose card anywhere shows what it does in the collection.
	found := false
	for _, ln := range cardDef("ocelot_card").EffectLines() {
		if ln == "Collection: +15 Speed" {
			found = true
		}
	}
	if !found {
		t.Errorf("ocelot card tooltip should list its collection effect, got %v", cardDef("ocelot_card").EffectLines())
	}
}

// Batch-B procs: aggregation, pure helpers (split / reviveHalf), effect text.
func TestCardEffects_BatchB(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.cardCollection = [MaxCardSlots]string{}
	g.cardCollection[0] = "archmage_card"
	g.cardCollection[1] = "ningyo_card"
	g.cardCollection[2] = "lich_card"
	g.cardCollection[3] = "gorilla_titan_card"

	if g.cardPhysToFirePct() != 25 || g.cardHealOnAttackPct() != 5 || g.cardHealAmount() != 25 ||
		g.cardLethalSavePct() != 10 || g.cardMoveAoePct() != 10 || g.cardMoveAoeDmg() != 50 {
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
		"archmage_card":      "25% of physical damage dealt as fire",
		"ningyo_card":        "5% to self-heal 25 on attack",
		"lich_card":          "10% to cheat death (half HP+SP)",
		"gorilla_titan_card": "10% on move: 50 pure to nearby foes",
	} {
		if got := cardEffectText(cardDef(key)); got != want {
			t.Errorf("cardEffectText(%s) = %q, want %q", key, got, want)
		}
	}
}

// AoE lethal damage routes through knockOut, so the Lich Card can cheat death on
// it too — not just plain melee. Fireburst stands in for the AoE/Inferno branches
// that previously set ConditionUnconscious directly. Statistical: with a 10% save
// over 400 lethal hits, both outcomes must appear (a direct KO would never save).
func TestLichCard_SavesOnAoEFireburst(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.cardCollection = [MaxCardSlots]string{}
	g.cardCollection[0] = "lich_card"
	if g.cardLethalSavePct() != 10 {
		t.Fatalf("lich save pct = %d, want 10", g.cardLethalSavePct())
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
		if member.HitPoints > 0 { // reviveHalf left HP up → cheated death (direct KO would be 0)
			saves++
		}
	}
	if saves == 0 {
		t.Fatalf("Lich Card never cheated death over %d AoE Fireburst hits — the AoE branch bypasses knockOut", trials)
	}
	if saves == trials {
		t.Fatalf("every AoE hit cheated death (%d/%d) — the save roll isn't being applied", saves, trials)
	}
}

// Archmage Card splits melee into phys + fire on the PRIMARY; the AoE splash must
// carry the SAME split (full magnitude), not just the physical remainder (the bug
// dropped the fire share, splashing 75 instead of 100).
func TestArchmageCard_SplashGetsFullSplit(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	if g.world == nil {
		t.Skip("test combat system has no world")
	}
	g.cardCollection = [MaxCardSlots]string{}
	g.cardCollection[0] = "archmage_card"
	if g.cardPhysToFirePct() != 25 {
		t.Fatalf("archmage pct = %d, want 25", g.cardPhysToFirePct())
	}
	ts := float64(g.config.GetTileSize())
	primary := &monster.Monster3D{Name: "Primary", X: 0, Y: 0, HitPoints: 1000, MaxHitPoints: 1000}
	near := &monster.Monster3D{Name: "Near", X: ts, Y: 0, HitPoints: 1000, MaxHitPoints: 1000} // 1 tile, inside 1.5
	g.world.Monsters = []*monster.Monster3D{primary, near}

	// Idol-Breaker: physical mace, aoe_radius_tiles 1.5. No armor/resist on targets,
	// so the splash should land the full 100 (75 phys + 25 converted fire).
	cs.ApplyDamageToMonster(primary, 100, "Idol-Breaker, the Warlord's Maul", false)

	if got := 1000 - near.HitPoints; got != 100 {
		t.Fatalf("splash dealt %d, want 100 (75 phys + 25 fire) — the fire share is dropped if this is 75", got)
	}
}

// The Gorilla move-burst hits living monsters within 1.5 tiles, not distant ones.
func TestCardMoveBurst_HitsNearbyOnly(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	if g.world == nil {
		t.Skip("test combat system has no world")
	}
	ts := float64(g.config.GetTileSize())
	near := &monster.Monster3D{ID: "near", HitPoints: 100, MaxHitPoints: 100, X: g.camera.X, Y: g.camera.Y, Resistances: map[monster.DamageType]int{}}
	far := &monster.Monster3D{ID: "far", HitPoints: 100, MaxHitPoints: 100, X: g.camera.X + ts*5, Y: g.camera.Y, Resistances: map[monster.DamageType]int{}}
	g.world.Monsters = append(g.world.Monsters, near, far)

	if !cs.cardMoveBurstApply(50) {
		t.Fatal("expected the burst to hit the nearby monster")
	}
	if near.HitPoints != 50 {
		t.Errorf("near monster should take 50 pure (hp=%d, want 50)", near.HitPoints)
	}
	if far.HitPoints != 100 {
		t.Errorf("far monster should be untouched (hp=%d, want 100)", far.HitPoints)
	}
}

// The Gorilla move-burst hits FOES only — never the party's own bound allies
// (card summons / bind-undead) or charmed (pacified) monsters.
func TestCardMoveBurst_SkipsAlliesAndPacified(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	if g.world == nil {
		t.Skip("test combat system has no world")
	}
	mk := func(id string, mod func(*monster.Monster3D)) *monster.Monster3D {
		m := &monster.Monster3D{ID: id, Name: id, HitPoints: 100, MaxHitPoints: 100,
			X: g.camera.X, Y: g.camera.Y, Resistances: map[monster.DamageType]int{}}
		mod(m)
		return m
	}
	foe := mk("foe", func(m *monster.Monster3D) {})
	ally := mk("ally", func(m *monster.Monster3D) { m.Bound = true })
	charmed := mk("charmed", func(m *monster.Monster3D) { m.Pacified = true })
	warded := mk("warded", func(m *monster.Monster3D) { m.BossWarded = true })
	g.world.Monsters = []*monster.Monster3D{foe, ally, charmed, warded}

	if !cs.cardMoveBurstApply(50) {
		t.Fatal("burst should report a hit on the foe")
	}
	if foe.HitPoints != 50 {
		t.Errorf("foe should take 50 pure (hp=%d, want 50)", foe.HitPoints)
	}
	if ally.HitPoints != 100 {
		t.Errorf("bound ally must NOT be hit by the burst (hp=%d, want 100)", ally.HitPoints)
	}
	if charmed.HitPoints != 100 {
		t.Errorf("pacified monster must NOT be hit by the burst (hp=%d, want 100)", charmed.HitPoints)
	}
	// Invulnerable boss is skipped entirely — no flash/hit/message, not just 0 damage.
	if warded.HitPoints != 100 || warded.HitTintFrames != 0 {
		t.Errorf("warded boss must be skipped (hp=%d hitTint=%d)", warded.HitPoints, warded.HitTintFrames)
	}
}

// The Gorilla move-burst is PURE: physical resistance (or immunity) must NOT
// reduce it, so a resistant mob still takes the full advertised amount.
func TestCardMoveBurst_PureBypassesPhysicalResist(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	if g.world == nil {
		t.Skip("test combat system has no world")
	}
	resistant := &monster.Monster3D{ID: "res", HitPoints: 100, MaxHitPoints: 100, X: g.camera.X, Y: g.camera.Y,
		Resistances: map[monster.DamageType]int{monster.DamagePhysical: 50}}
	immune := &monster.Monster3D{ID: "imm", HitPoints: 100, MaxHitPoints: 100, X: g.camera.X, Y: g.camera.Y,
		Resistances: map[monster.DamageType]int{monster.DamagePhysical: 100}}
	g.world.Monsters = append(g.world.Monsters, resistant, immune)

	if !cs.cardMoveBurstApply(50) {
		t.Fatal("expected the burst to hit")
	}
	if resistant.HitPoints != 50 {
		t.Errorf("50%% physical-resist mob should still take full 50 pure (hp=%d, want 50)", resistant.HitPoints)
	}
	if immune.HitPoints != 50 {
		t.Errorf("physical-immune mob should still take full 50 pure (hp=%d, want 50)", immune.HitPoints)
	}
}

// Batch-C summon: aggregation/text, and the deterministic spawn core produces
// permanent allied (Bound) monsters tagged for the per-collection limit.
func TestCardSummon_AlliesAndLimit(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.cardCollection = [MaxCardSlots]string{}
	g.cardCollection[0] = "orc_warlord_card"

	if g.cardSummonChance() != 15 || g.cardSummonLimit() != 2 || g.cardSummonMonsterKey() != "masked_huntress" {
		t.Fatalf("aggregates: chance=%d limit=%d key=%q", g.cardSummonChance(), g.cardSummonLimit(), g.cardSummonMonsterKey())
	}
	if got := cardEffectText(cardDef("orc_warlord_card")); got != "15% on action: summon allies (max 2)" {
		t.Errorf("effect text = %q", got)
	}

	// markCardAlly turns a spawned monster into a permanent ally (tile-spawning
	// itself needs world infra the harness lacks, so place allies directly).
	a1 := &monster.Monster3D{ID: "a1", HitPoints: 50, MaxHitPoints: 50}
	a2 := &monster.Monster3D{ID: "a2", HitPoints: 50, MaxHitPoints: 50}
	markCardAlly(a1)
	markCardAlly(a2)
	if !a1.Bound || a1.BoundFramesRemaining != 0 || a1.SummonedBy != cardSummonOwner || !a1.QuestProgressIgnored {
		t.Fatalf("markCardAlly: Bound=%v frames=%d by=%q questIgnored=%v",
			a1.Bound, a1.BoundFramesRemaining, a1.SummonedBy, a1.QuestProgressIgnored)
	}
	g.world.Monsters = append(g.world.Monsters, a1, a2)

	if cs.countCardSummons() != 2 {
		t.Errorf("countCardSummons() = %d, want 2", cs.countCardSummons())
	}
	// At the limit, tryCardSummonOnAction would request 0 more.
	if want := g.cardSummonLimit() - cs.countCardSummons(); want != 0 {
		t.Errorf("remaining summon capacity = %d, want 0 (at limit)", want)
	}
	// A slain ally frees a slot.
	a2.HitPoints = 0
	if cs.countCardSummons() != 1 {
		t.Errorf("after one ally dies, countCardSummons() = %d, want 1", cs.countCardSummons())
	}
	if want := g.cardSummonLimit() - cs.countCardSummons(); want != 1 {
		t.Errorf("freed capacity = %d, want 1", want)
	}
}

// resetCardCollection (called from startNewGameWithParty) clears the collection
// and recomputes stats, so a fresh party never inherits the old run's effects.
func TestResetCardCollection_ClearsEffects(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.cardCollection[0] = "thief_bug_card"
	g.cardCollection[1] = "ocelot_card"
	g.recomputeStatBonuses()
	if g.cardMoveSpeedPct() == 0 || g.cardStatBonuses().Speed == 0 || g.statBonuses.Speed == 0 {
		t.Fatal("precondition: cards should be active first")
	}

	g.resetCardCollection()

	if g.cardCollection != ([MaxCardSlots]string{}) {
		t.Errorf("collection should be empty, got %v", g.cardCollection)
	}
	if g.cardMoveSpeedPct() != 0 || g.cardStatBonuses().Speed != 0 {
		t.Errorf("card effects should be gone: move=%d speed=%d", g.cardMoveSpeedPct(), g.cardStatBonuses().Speed)
	}
	if g.statBonuses.Speed != 0 {
		t.Errorf("party stat bonus should recompute to 0, got %d", g.statBonuses.Speed)
	}
}

// All loose cards are enumerated (even past one collector page) so pagination can
// reach every one — there are 11 card types, the page shows cardInvMaxShown(8).
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
	game.cardCollection[0] = "thief_bug_card"
	game.cardCollection[3] = "puma_card"

	save := game.buildSave(wm)

	loaded := newTestGame(cfg, w)
	loaded.cardCollection[1] = "lich_card" // should be wiped by restore
	if err := loaded.applySave(wm, &save); err != nil {
		t.Fatalf("apply save: %v", err)
	}
	if loaded.cardCollection[0] != "thief_bug_card" || loaded.cardCollection[3] != "puma_card" {
		t.Fatalf("collection not restored: %v", loaded.cardCollection)
	}
	if loaded.cardCollection[1] != "" {
		t.Errorf("stale slot should clear on restore, got %q", loaded.cardCollection[1])
	}
	if loaded.cardMoveSpeedPct() != 25 || loaded.cardBonusActions() != 1 {
		t.Errorf("restored effects wrong: speed=%d actions=%d", loaded.cardMoveSpeedPct(), loaded.cardBonusActions())
	}
}

// Every card added in the 2026-07-03 roster expansion must parse into a real
// item def with a non-empty, non-"not implemented" effect line — catches a
// typo'd card_* field name or a key mismatch across all 41 in one pass.
func TestNewRosterCards_AllHaveRealEffects(t *testing.T) {
	newTestCombatSystemWithConfig(t)
	keys := []string{
		"alien_card", "ashigaru_firelock_card", "bandit_card", "bat_card", "bear_card",
		"dire_wolf_card", "dragon_card", "dragon_gold_card", "dragon_green_card", "dragon_red_card",
		"elder_dragon_card", "elder_dragon_gold_card", "elder_dragon_green_card", "elder_dragon_red_card",
		"elf_archer_card", "elf_swordsman_card", "forest_orc_card", "forest_spider_card", "goblin_card",
		"golden_thief_bug_card", "isis_card", "jungle_goblin_card", "jungle_idol_card", "lich_king_card",
		"masked_hexer_girl_card", "masked_serpent_dancer_card", "minotaur_card", "mountain_troll_card",
		"mummy_card", "octopus_card", "orc_card", "pixie_card", "rat_card", "revenant_card",
		"ronin_marksman_card", "skeleton_card", "spider_card", "treant_card", "troll_card",
		"vengeful_ningyo_card", "wolf_card",
	}
	if len(keys) != 41 {
		t.Fatalf("expected 41 keys, got %d", len(keys))
	}
	for _, key := range keys {
		def := cardDef(key)
		if def == nil {
			t.Errorf("%s: no item definition found", key)
			continue
		}
		if got := cardEffectText(def); got == "" || got == "Currently not implemented" {
			t.Errorf("%s: cardEffectText = %q, want a real effect", key, got)
		}
	}
}

// Alien Card: statistical — 2% of hits should instantly zero HP, but not all.
func TestAlienCard_DisintegrateOnHit(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.cardCollection = [MaxCardSlots]string{}
	g.cardCollection[0] = "alien_card"
	if g.cardDisintegratePct() != 2 {
		t.Fatalf("cardDisintegratePct = %d, want 2", g.cardDisintegratePct())
	}

	killed, survived := 0, 0
	const trials = 500
	for i := 0; i < trials; i++ {
		m := mkTestMonster("Skeleton Warrior", 1000) // not undead/dragon, so eligible
		cs.ApplyDamageToMonster(m, 10, "Idol-Breaker, the Warlord's Maul", false)
		if m.HitPoints == 0 {
			killed++
		} else {
			survived++
		}
	}
	if killed == 0 {
		t.Fatalf("disintegrate never triggered over %d hits", trials)
	}
	if survived == 0 {
		t.Fatalf("disintegrate triggered on every hit (%d/%d) — should be ~2%%", killed, trials)
	}
}

// Golden Thief Bug Card: 100 flat fire resist through mitigateCharacterDamage
// hits the existing >=100 immunity clamp — full fire immunity.
func TestGoldenThiefBugCard_FireImmunity(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.cardCollection = [MaxCardSlots]string{}
	g.cardCollection[0] = "golden_thief_bug_card"
	if got := g.cardResistBonusFor("fire"); got != 100 {
		t.Fatalf("cardResistBonusFor(fire) = %d, want 100", got)
	}
	member := g.party.Members[0]
	if got := cs.mitigateCharacterDamage(500, "fire", member, false); got != 0 {
		t.Errorf("fire damage through GTB card = %d, want 0 (immune)", got)
	}
	if got := cs.mitigateCharacterDamage(500, "physical", member, false); got == 0 {
		t.Error("physical damage should NOT be blocked by a fire-only ward")
	}
}

// Dragon Cards grant a flat resist bonus to their own element, at two tiers
// (base 50 / elder 75), and stack when both a base and elder card are held.
func TestDragonCards_ResistBonusPerElement(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game

	cases := []struct {
		key     string
		element string
		want    int
	}{
		{"dragon_card", "fire", 50},
		{"dragon_red_card", "fire", 50},
		{"dragon_green_card", "earth", 50},
		{"dragon_gold_card", "air", 50},
		{"elder_dragon_card", "fire", 75},
		{"elder_dragon_red_card", "fire", 75},
		{"elder_dragon_green_card", "earth", 75},
		{"elder_dragon_gold_card", "air", 75},
	}
	for _, c := range cases {
		g.cardCollection = [MaxCardSlots]string{}
		g.cardCollection[0] = c.key
		if got := g.cardResistBonusFor(c.element); got != c.want {
			t.Errorf("%s resist(%s) = %d, want %d", c.key, c.element, got, c.want)
		}
	}

	// Base + elder of the same color stack.
	g.cardCollection = [MaxCardSlots]string{}
	g.cardCollection[0] = "dragon_red_card"
	g.cardCollection[1] = "elder_dragon_red_card"
	if got := g.cardResistBonusFor("fire"); got != 125 {
		t.Errorf("stacked red dragon fire resist = %d, want 125", got)
	}
	_ = cs
}

// Jungle Goblin Card doubles gold from a kill (card_gold_find_pct: 100).
func TestJungleGoblinCard_DoublesGold(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	if g.world == nil {
		t.Skip("test combat system has no world")
	}
	g.cardCollection = [MaxCardSlots]string{}
	g.cardCollection[0] = "jungle_goblin_card"
	if g.cardGoldFindPct() != 100 {
		t.Fatalf("cardGoldFindPct = %d, want 100", g.cardGoldFindPct())
	}

	m := mkTestMonster("Goblin", 10)
	m.Gold = 40
	g.world.Monsters = []*monster.Monster3D{m}
	cs.ApplyDamageToMonster(m, 1000, "Idol-Breaker, the Warlord's Maul", false)

	if len(g.groundContainers) == 0 {
		t.Fatal("expected a loot bag to spawn")
	}
	if got := g.groundContainers[len(g.groundContainers)-1].Gold; got != 80 {
		t.Errorf("dropped gold = %d, want 80 (40 doubled)", got)
	}
}

// Treant Card grants flat party Armor Class.
func TestTreantCard_ArmorBonus(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	member := g.party.Members[0]
	before := cs.CalculateTotalArmorClass(member)

	g.cardCollection = [MaxCardSlots]string{}
	g.cardCollection[0] = "treant_card"
	after := cs.CalculateTotalArmorClass(member)
	if after-before != 10 {
		t.Errorf("treant card AC delta = %d, want 10", after-before)
	}
}

// Jungle Idol Card grants flat party max HP, applied through the same push
// used for BuffBonuses (applyPartyStatBonuses).
func TestJungleIdolCard_MaxHPBonus(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	member := g.party.Members[0]
	before := member.MaxHitPoints

	g.cardCollection = [MaxCardSlots]string{}
	g.cardCollection[0] = "jungle_idol_card"
	g.recomputeStatBonuses()
	if got := member.MaxHitPoints - before; got != 25 {
		t.Errorf("max HP delta = %d, want 25", got)
	}

	// Removing the card must give it back.
	g.cardCollection = [MaxCardSlots]string{}
	g.recomputeStatBonuses()
	if member.MaxHitPoints != before {
		t.Errorf("max HP after removal = %d, want back to %d", member.MaxHitPoints, before)
	}
	_ = cs
}

// Troll Cards regenerate a % of max HP once per regen-tick cadence (RT).
func TestTrollCards_RegenPct(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	member := g.party.Members[0]

	g.cardCollection = [MaxCardSlots]string{}
	g.cardCollection[0] = "troll_card"          // 2%
	g.cardCollection[1] = "mountain_troll_card" // 3%
	g.recomputeStatBonuses()
	if g.cardRegenPct() != 5 {
		t.Fatalf("cardRegenPct = %d, want 5", g.cardRegenPct())
	}

	member.MaxHitPoints = 1000
	member.HitPoints = 500
	for i := 0; i < character.ManaRegenIntervalFrames; i++ {
		member.Update()
	}
	if member.HitPoints != 550 {
		t.Errorf("HP after one regen tick = %d, want 550 (500 + 5%% of 1000)", member.HitPoints)
	}
}

// Regression: the Troll Card's HP regen only lived in the RT-only
// updateRegenAndPoison — UpdateWithMode(true) (TB) returned before reaching it,
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

// Vengeful Ningyo Card reflects a flat % of incoming damage back at the
// attacking monster.
func TestVengefulNingyoCard_Thorns(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.cardCollection = [MaxCardSlots]string{}
	g.cardCollection[0] = "vengeful_ningyo_card"
	if g.cardThornsPct() != 12 {
		t.Fatalf("cardThornsPct = %d, want 12", g.cardThornsPct())
	}

	attacker := mkTestMonster("Bandit", 1000)
	member := g.party.Members[0]
	member.HitPoints, member.MaxHitPoints = 500, 500
	member.Luck = 0 // deterministic: no Perfect Dodge so the hit (and thorns) always lands

	cs.monsterHitCharacter(attacker, member, "Bandit", 100, "physical", false, 0)
	if attacker.HitPoints >= 1000 {
		t.Errorf("attacker HP = %d, should have taken thorns reflect damage", attacker.HitPoints)
	}
}

// Vengeful Ningyo Card: a reflect that kills the attacking monster must still
// run kill finalization (XP/loot/quest credit), not just zero its HP.
func TestVengefulNingyoCard_ThornsKillFinalizesKill(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.cardCollection = [MaxCardSlots]string{}
	g.cardCollection[0] = "vengeful_ningyo_card" // 12% reflect
	if g.cardThornsPct() != 12 {
		t.Fatalf("cardThornsPct = %d, want 12", g.cardThornsPct())
	}

	attacker := mkTestMonster("Weak Attacker", 10) // 12% of 100 = 12 > 10 HP
	attacker.Experience = 50
	member := g.party.Members[0]
	member.HitPoints, member.MaxHitPoints = 500, 500
	member.Luck = 0 // deterministic: no Perfect Dodge so the hit (and thorns) always lands

	before := len(g.deadMonsterIDs)
	cs.monsterHitCharacter(attacker, member, "Weak Attacker", 100, "physical", false, 0)
	if attacker.IsAlive() {
		t.Fatal("setup: reflected damage should have killed the attacker")
	}
	if len(g.deadMonsterIDs) != before+1 {
		t.Error("thorns kill should register in deadMonsterIDs via finishMonsterKill")
	}
}

// Hexer/Isis Cards divert a share of physical damage into dark/light instead —
// same split mechanism as Archmage's fire conversion, different element.
func TestHexerIsisCards_ElementConversion(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	if g.world == nil {
		t.Skip("test combat system has no world")
	}

	g.cardCollection = [MaxCardSlots]string{}
	g.cardCollection[0] = "masked_hexer_girl_card" // 20% -> dark
	dark := mkTestMonster("Target", 1000)
	cs.ApplyDamageToMonster(dark, 100, "Idol-Breaker, the Warlord's Maul", false)
	if got := 1000 - dark.HitPoints; got != 100 {
		t.Errorf("hexer split total damage = %d, want 100 conserved", got)
	}

	g.cardCollection = [MaxCardSlots]string{}
	g.cardCollection[0] = "isis_card" // 50% -> light, melee AND ranged
	light := mkTestMonster("Target2", 1000)
	cs.ApplyDamageToMonster(light, 100, "Idol-Breaker, the Warlord's Maul", false)
	if got := 1000 - light.HitPoints; got != 100 {
		t.Errorf("isis split total damage = %d, want 100 conserved", got)
	}
}

// Hexer/Isis Cards: the AoE splash must carry the SAME dark/light split as the
// primary hit, not just the physical remainder (mirrors the pre-existing
// Archmage fire-split splash test).
func TestHexerCard_AoESplashCarriesDarkConversion(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	if g.world == nil {
		t.Skip("test combat system has no world")
	}
	g.cardCollection = [MaxCardSlots]string{}
	g.cardCollection[0] = "masked_hexer_girl_card" // 20% -> dark
	ts := float64(g.config.GetTileSize())
	primary := mkTestMonster("Primary", 1000)
	primary.X, primary.Y = 0, 0
	near := mkTestMonster("Near", 1000)
	near.X, near.Y = ts, 0
	g.world.Monsters = []*monster.Monster3D{primary, near}

	cs.ApplyDamageToMonster(primary, 100, "Idol-Breaker, the Warlord's Maul", false)
	if got := 1000 - near.HitPoints; got != 100 {
		t.Errorf("splash dealt %d, want 100 (80 phys + 20 dark) — the dark share is dropped if this is 80", got)
	}
}

// Masked Serpent Dancer Card is a flat +20% melee weapon damage multiplier.
func TestMaskedSerpentDancerCard_MeleeDmgPct(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	if g.world == nil {
		t.Skip("test combat system has no world")
	}
	g.cardCollection = [MaxCardSlots]string{}
	g.cardCollection[0] = "masked_serpent_dancer_card"
	if g.cardMeleeDmgPct() != 20 {
		t.Fatalf("cardMeleeDmgPct = %d, want 20", g.cardMeleeDmgPct())
	}
	m := mkTestMonster("Target", 1000)
	cs.ApplyDamageToMonster(m, 100, "Idol-Breaker, the Warlord's Maul", false)
	if got := 1000 - m.HitPoints; got != 120 {
		t.Errorf("melee damage with +20%% card = %d, want 120", got)
	}
}

// Elf Archer / Skeleton Cards: card-driven bonus_vs, matched by monster Name
// (dragon) and by MonsterType (formless bosses) respectively.
func TestElfArcherSkeletonCards_BonusVs(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game

	g.cardCollection = [MaxCardSlots]string{}
	g.cardCollection[0] = "elf_archer_card"
	dragon := &monster.Monster3D{Name: "Dragon", Key: "dragon_red"}
	if got := g.cardBonusVsMultiplier(dragon); got != 1.25 {
		t.Errorf("elf archer vs dragon mult = %.2f, want 1.25", got)
	}
	notDragon := &monster.Monster3D{Name: "Goblin", Key: "goblin"}
	if got := g.cardBonusVsMultiplier(notDragon); got != 1.0 {
		t.Errorf("elf archer vs goblin mult = %.2f, want 1.0", got)
	}

	g.cardCollection = [MaxCardSlots]string{}
	g.cardCollection[0] = "skeleton_card"
	boss := &monster.Monster3D{Name: "Golden Thief Bug", MonsterType: "formless"}
	if got := g.cardBonusVsMultiplier(boss); got != 1.20 {
		t.Errorf("skeleton vs formless mult = %.2f, want 1.20", got)
	}
	_ = cs
}

// Regression: Name/Key/MonsterType often name the same identity — the real
// Dragon monster (assets/monsters.yaml) has Name="Dragon", Key="dragon", AND
// MonsterType="dragon". A single card_bonus_vs: {dragon: 1.25} entry matched
// all three candidate fields and multiplied in three times (1.25^3 ≈ 1.95)
// instead of once — one matching entry means "this card applies," not
// "multiply once per field that happened to match."
func TestCardBonusVs_SameIdentityAcrossFieldsAppliesOnce(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.cardCollection = [MaxCardSlots]string{}
	g.cardCollection[0] = "elf_archer_card"

	dragon := &monster.Monster3D{Name: "Dragon", Key: "dragon", MonsterType: "dragon"}
	if got := g.cardBonusVsMultiplier(dragon); got != 1.25 {
		t.Errorf("elf archer vs a Dragon whose Name/Key/MonsterType all read \"dragon\" = %.4f, want 1.25 (single application)", got)
	}
}

// Forest Orc Card: statistical armor-ignore chance — some hits should land at
// full (armor-bypassed) damage against a heavily armored target.
func TestForestOrcCard_ArmorPierceOnHit(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.cardCollection = [MaxCardSlots]string{}
	g.cardCollection[0] = "forest_orc_card"
	if g.cardArmorPiercePct() != 10 {
		t.Fatalf("cardArmorPiercePct = %d, want 10", g.cardArmorPiercePct())
	}

	bypassed, mitigated := 0, 0
	const trials = 400
	for i := 0; i < trials; i++ {
		m := mkTestMonster("Armored Target", 100000)
		m.ArmorClass = 200 // heavy armor, would meaningfully cut unmitigated damage
		before := m.HitPoints
		cs.ApplyDamageToMonster(m, 100, "Idol-Breaker, the Warlord's Maul", false)
		dealt := before - m.HitPoints
		if dealt >= 100 {
			bypassed++
		} else {
			mitigated++
		}
	}
	if bypassed == 0 {
		t.Fatalf("armor-pierce never triggered over %d hits", trials)
	}
	if mitigated == 0 {
		t.Fatalf("armor-pierce triggered on every hit (%d/%d) — should be ~10%%", bypassed, trials)
	}
}

// Mummy Card grants full (100%) resistance to the monster-inflicted poison proc.
func TestMummyCard_PoisonImmunity(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.cardCollection = [MaxCardSlots]string{}
	g.cardCollection[0] = "mummy_card"
	if g.cardPoisonResistPct() != 100 {
		t.Fatalf("cardPoisonResistPct = %d, want 100", g.cardPoisonResistPct())
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
// party) — a genuinely new status, ticking HP down over time via TickPoison.
func TestRatSpiderCards_PoisonsMonsterOnHit(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.cardCollection = [MaxCardSlots]string{}
	g.cardCollection[0] = "rat_card" // 12%/20s
	if pct, dur := g.cardPoisonProc(); pct != 12 || dur != 20 {
		t.Fatalf("cardPoisonProc = %d%%/%ds, want 12%%/20s", pct, dur)
	}
	g.cardCollection[1] = "spider_card"        // +15%/20s
	g.cardCollection[2] = "forest_spider_card" // +15%/20s
	if pct, dur := g.cardPoisonProc(); pct != 42 || dur != 20 {
		t.Fatalf("stacked cardPoisonProc = %d%%/%ds, want 42%%/20s", pct, dur)
	}
	g.cardCollection[1], g.cardCollection[2] = "", ""

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
		t.Fatalf("poison proc triggered on every hit (%d/%d) — should be ~12%%", poisoned, trials)
	}

	// A poisoned monster loses HP over (simulated) time via TickPoison.
	tps := config.GetTargetTPS()
	m = mkTestMonster("Ticking", 1000)
	m.ApplyPoison(tps * 2) // 2 seconds of poison
	before := m.HitPoints
	for i := 0; i < tps; i++ { // 1 second — one tick should have fired
		m.TickPoison()
	}
	if m.HitPoints >= before {
		t.Errorf("poisoned monster HP = %d, should have dropped below %d after 1s of ticking", m.HitPoints, before)
	}
}

// Minotaur Card: statistical stun-on-hit, reusing the existing stun-DR path.
func TestMinotaurCard_StunOnHit(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.cardCollection = [MaxCardSlots]string{}
	g.cardCollection[0] = "minotaur_card"
	if g.cardStunOnHitPct() != 8 {
		t.Fatalf("cardStunOnHitPct = %d, want 8", g.cardStunOnHitPct())
	}

	stunned := 0
	const trials = 400
	for i := 0; i < trials; i++ {
		m := mkTestMonster("Target", 100000)
		cs.tryApplyWeaponStun(m, nil)
		if m.StunFramesRemaining > 0 || m.StunTurnsRemaining > 0 {
			stunned++
		}
	}
	if stunned == 0 {
		t.Fatalf("stun-on-hit never triggered over %d hits", trials)
	}
	if stunned == trials {
		t.Fatalf("stun-on-hit triggered on every hit (%d/%d) — should be ~8%%", stunned, trials)
	}
}

// Ronin Marksman / Bat Cards: flat crit/dodge bonuses feed the existing rolls.
func TestRoninBatCards_CritAndDodgeBonus(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	member := g.party.Members[0]

	baseCrit := cs.CalculateCriticalChance(member)
	g.cardCollection = [MaxCardSlots]string{}
	g.cardCollection[0] = "ronin_marksman_card"
	if got := cs.CalculateCriticalChance(member) - baseCrit; got != 5 {
		t.Errorf("crit bonus delta = %d, want 5", got)
	}

	_, baseDodge := cs.RollPerfectDodge(member)
	g.cardCollection = [MaxCardSlots]string{}
	g.cardCollection[0] = "bat_card"
	_, afterDodge := cs.RollPerfectDodge(member)
	if got := afterDodge - baseDodge; got != 4 {
		t.Errorf("dodge bonus delta = %d, want 4", got)
	}
}
