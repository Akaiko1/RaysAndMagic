package game

import (
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

// TestArenaUniqueWeaponData: every arena unique carries its signature rider,
// so a YAML edit can't silently strip an ability the shop advertises.
func TestArenaUniqueWeaponData(t *testing.T) {
	newTestCombatSystemWithConfig(t)
	checks := []struct {
		key string
		ok  func(*config.WeaponDefinitionConfig) bool
	}{
		{"gladius", func(w *config.WeaponDefinitionConfig) bool { return w.BonusVsStunned > 1 }},
		{"arena_labrys", func(w *config.WeaponDefinitionConfig) bool { return w.ArmorShredPct > 0 && w.ArmorShredSeconds > 0 }},
		{"morningstar", func(w *config.WeaponDefinitionConfig) bool { return w.StunChance > 0 && w.StunTurns > 0 }},
		{"hasta", func(w *config.WeaponDefinitionConfig) bool { return w.ArmorClassBonus > 0 }},
		{"trident", func(w *config.WeaponDefinitionConfig) bool { return w.RootChance > 0 && w.RootSeconds > 0 }},
		{"parry_dagger", func(w *config.WeaponDefinitionConfig) bool { return w.ThornsPct > 0 }},
		{"lion_warhammer", func(w *config.WeaponDefinitionConfig) bool { return w.ArmorPiercePct > 0 }},
		{"arena_shortbow", func(w *config.WeaponDefinitionConfig) bool {
			return w.CooldownMultiplier > 0 && w.CooldownMultiplier < 1.2
		}},
		{"arbalest", func(w *config.WeaponDefinitionConfig) bool { return w.PierceCount > 0 }},
		{"lanista_scepter", func(w *config.WeaponDefinitionConfig) bool { return w.EquipPersonalityMin > 0 }},
		{"bronze_cesti", func(w *config.WeaponDefinitionConfig) bool { return w.DoubleStrike }},
	}
	for _, c := range checks {
		def, ok := config.GetWeaponDefinition(c.key)
		if !ok || def == nil {
			t.Fatalf("weapon %q missing", c.key)
		}
		if def.Rarity != "unique" {
			t.Errorf("%s rarity = %q, want unique", c.key, def.Rarity)
		}
		if !c.ok(def) {
			t.Errorf("%s lost its signature rider", c.key)
		}
		// The shortbow's whole identity is its cadence, which renders via
		// character.WeaponCombatLines rather than EffectLines.
		if c.key != "arena_shortbow" && len(def.EffectLines()) == 0 {
			t.Errorf("%s has no tooltip effect lines", c.key)
		}
	}
}

// TestArmorShredAndPierce: the Pit Labrys shred and the warhammer's flat
// pierce both reduce the armor value combat mitigates against.
func TestArmorShredAndPierce(t *testing.T) {
	newTestCombatSystemWithConfig(t)
	m := &monster.Monster3D{ArmorClass: 40}
	m.ApplyArmorShred(20, 100, 1)
	if got := m.EffectiveArmorClass(); got != 32 {
		t.Fatalf("shredded AC = %d, want 32", got)
	}
	hammer, _ := config.GetWeaponDefinition("lion_warhammer")
	if got := effectiveMonsterArmor(m, hammer); got != 32*(100-hammer.ArmorPiercePct)/100 {
		t.Fatalf("pierced AC = %d, want %d", got, 32*(100-hammer.ArmorPiercePct)/100)
	}
	// Shred expires with its clocks.
	for i := 0; i < 100; i++ {
		m.TickArmorShredFrame()
	}
	m.TickArmorShredTurn()
	if got := m.EffectiveArmorClass(); got != 40 {
		t.Fatalf("expired shred AC = %d, want 40", got)
	}
}

// TestStunnedBonusMultiplier: the Gladius hits stunned targets harder.
func TestStunnedBonusMultiplier(t *testing.T) {
	newTestCombatSystemWithConfig(t)
	gladius, _ := config.GetWeaponDefinition("gladius")
	m := &monster.Monster3D{}
	if got := weaponStunnedBonusMultiplier(gladius, m); got != 1.0 {
		t.Fatalf("unstunned mult = %v, want 1.0", got)
	}
	m.StunFramesRemaining = 10
	if got := weaponStunnedBonusMultiplier(gladius, m); got != gladius.BonusVsStunned {
		t.Fatalf("stunned mult = %v, want %v", got, gladius.BonusVsStunned)
	}
}

// TestScepterPersonalityGate: the Lanista's Scepter equips on Personality
// alone - no staff skill needed - and refuses below the threshold.
func TestScepterPersonalityGate(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	fillTestParty(t, cs.game)
	knight := cs.game.party.Members[0]
	delete(knight.Skills, character.SkillStaff)
	def, _ := config.GetWeaponDefinition("lanista_scepter")
	knight.Personality = def.EquipPersonalityMin
	if !knight.CanEquipWeaponByName("Lanista's Scepter") {
		t.Fatal("Personality at threshold must equip the scepter")
	}
	knight.Personality = def.EquipPersonalityMin - 1
	if knight.CanEquipWeaponByName("Lanista's Scepter") {
		t.Fatal("below threshold must refuse without the staff skill")
	}
}

// TestArmorSetBonuses: 4 equipped pieces complete a set - padded halves stun
// durations on the wearer, ringmail feeds +10 Endurance into effective stats.
func TestArmorSetBonuses(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	fillTestParty(t, cs.game)
	ch := cs.game.party.Members[0]
	ch.Skills[character.SkillLeather] = &character.Skill{}
	ch.Skills[character.SkillChain] = &character.Skill{}

	equip := func(keys ...string) {
		ch.Equipment = map[items.EquipSlot]items.Item{}
		for _, k := range keys {
			it, err := items.TryCreateItemFromYAML(k)
			if err != nil {
				t.Fatalf("create %s: %v", k, err)
			}
			ch.Equipment[it.PreferredSlot(items.SlotArmor)] = it
		}
	}

	equip("padded_cap", "padded_vest", "padded_gloves")
	if got := ch.SetStunDurationPct(); got != 0 {
		t.Fatalf("3 pieces should grant no set bonus, got %d", got)
	}
	equip("padded_cap", "padded_vest", "padded_gloves", "padded_boots")
	if got := ch.SetStunDurationPct(); got != -50 {
		t.Fatalf("padded set bonus = %d, want -50", got)
	}

	base := ch.GetEffectiveEndurance()
	equip("ringmail_coif", "ringmail_hauberk", "ringmail_gloves", "ringmail_boots")
	if got := ch.GetEffectiveEndurance(); got != base+10 {
		t.Fatalf("ringmail set endurance = %d, want %d", got, base+10)
	}
}

// TestHastaAndParmaArmor: weapon AC bonus reaches the bearer; the Parma's
// shield wall reaches every OTHER member, never the bearer.
func TestHastaAndParmaArmor(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	fillTestParty(t, cs.game)
	bearer := cs.game.party.Members[0]
	other := cs.game.party.Members[1]

	baseBearer := cs.CalculateTotalArmorClass(bearer)
	baseOther := cs.CalculateTotalArmorClass(other)

	hasta, err := items.TryCreateWeaponFromYAML("hasta")
	if err != nil {
		t.Fatalf("hasta: %v", err)
	}
	bearer.Equipment[items.SlotMainHand] = hasta
	def, _ := config.GetWeaponDefinition("hasta")
	if got := cs.CalculateTotalArmorClass(bearer); got != baseBearer+def.ArmorClassBonus {
		t.Fatalf("hasta AC = %d, want %d", got, baseBearer+def.ArmorClassBonus)
	}
	delete(bearer.Equipment, items.SlotMainHand)

	parma, err := items.TryCreateItemFromYAML("parma_shield")
	if err != nil {
		t.Fatalf("parma: %v", err)
	}
	bearer.Equipment[items.SlotOffHand] = parma
	parmaDef, _, okDef := config.GetItemDefinitionByName("Parma")
	if !okDef || parmaDef.PartyArmorBonus <= 0 {
		t.Fatal("Parma must author a positive party_armor_bonus")
	}
	gotOther := cs.CalculateTotalArmorClass(other)
	if gotOther != baseOther+parmaDef.PartyArmorBonus {
		t.Fatalf("parma aura on ally = %d, want %d", gotOther, baseOther+parmaDef.PartyArmorBonus)
	}
	// The bearer gets the shield's own AC but NOT the aura.
	wantBearer := baseBearer + cs.CalculateArmorClassContribution(parma, bearer)
	if got := cs.CalculateTotalArmorClass(bearer); got != wantBearer {
		t.Fatalf("parma bearer AC = %d, want %d (no self-aura)", got, wantBearer)
	}
	// A character OUTSIDE the party (champion templates run through the same
	// AC math) must never borrow the party's shield wall.
	outsider := character.CreateCharacter("Outsider", bearer.Class, cs.game.config)
	outsiderBase := cs.CalculateTotalArmorClass(outsider)
	if aura := cs.game.partyArmorAuraBonusFor(outsider); aura != 0 {
		t.Fatalf("non-party character got aura %d, want 0", aura)
	}
	if got := cs.CalculateTotalArmorClass(outsider); got != outsiderBase {
		t.Fatalf("non-party AC drifted: %d -> %d", outsiderBase, got)
	}
}

// TestManaPotion: restores SP scaled by Personality; refuses at full SP.
func TestManaPotion(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	fillTestParty(t, cs.game)
	g := cs.game
	ch := g.party.Members[0]
	ch.MaxSpellPoints, ch.SpellPoints = 100, 10

	pot, err := items.TryCreateItemFromYAML("mana_potion")
	if err != nil {
		t.Fatalf("mana potion: %v", err)
	}
	g.party.Inventory = []items.Item{pot}
	if !g.UseConsumableFromInventory(0, 0) {
		t.Fatal("mana potion not consumed")
	}
	want := 10 + 25 + ch.GetEffectivePersonality()/4
	if want > 100 {
		want = 100
	}
	if ch.SpellPoints != want {
		t.Fatalf("SP after potion = %d, want %d", ch.SpellPoints, want)
	}
	// Full SP refuses (potion kept).
	ch.SpellPoints = ch.MaxSpellPoints
	g.party.Inventory = []items.Item{pot}
	if g.UseConsumableFromInventory(0, 0) {
		t.Fatal("full-SP drink must refuse")
	}
	if len(g.party.Inventory) != 1 {
		t.Fatal("refused potion must stay in the bag")
	}
}

// TestFlyTileRules: with Fly active the party passes through interior solids
// but NEVER the border ring.
func TestFlyTileRules(t *testing.T) {
	cfg := loadTestConfig(t)
	w := newTestWorldSized(cfg, 6, 6)
	w.Tiles[3][3] = world.TileWall
	if !w.IsTileBlocking(3, 3) {
		t.Fatal("wall must block without Fly")
	}
	w.SetFlyActive(true)
	if w.IsTileBlocking(3, 3) {
		t.Fatal("Fly must pass through interior walls")
	}
	for _, edge := range [][2]int{{0, 3}, {5, 3}, {3, 0}, {3, 5}} {
		if !w.IsTileBlocking(edge[0], edge[1]) {
			t.Fatalf("Fly must NOT pass the border tile %v", edge)
		}
	}
	// Projectiles keep REAL terrain collision while the party flies - a bolt
	// must never sail through a wall just because Fly is up.
	w.SetFlyActive(true)
	ts := float64(cfg.GetTileSize())
	if w.CanProjectileMoveTo(3.5*ts, 3.5*ts) {
		t.Fatal("projectiles must not pass through walls under Fly")
	}
	w.SetFlyActive(false)
	if !w.IsTileBlocking(3, 3) {
		t.Fatal("expired Fly must restore collision")
	}
}

// TestFlyDropsIndoors: an active Fly ends the moment the party stands on a
// map without an open sky (dungeon entry / indoor load) - a lingering Fly
// indoors lets the party phase into walls and get sealed in on expiry.
func TestFlyDropsIndoors(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	t.Chdir("../..") // the sky-variant check stats assets/ from the repo root
	prev := world.GlobalWorldManager
	t.Cleanup(func() { world.GlobalWorldManager = prev })

	// Outdoor map (arena sky ships day/night variants): Fly holds.
	world.GlobalWorldManager = &world.WorldManager{
		MapConfigs:    map[string]*config.MapConfig{"arena": {SkyTexture: "arena_panorama"}},
		CurrentMapKey: "arena",
	}
	game.flyActive, game.flyDuration = true, 999
	game.dropFlyWithoutOpenSky()
	if !game.flyActive {
		t.Fatal("Fly must survive on an open-sky map")
	}

	// Indoor map (church panorama has no phase variants): Fly fades.
	world.GlobalWorldManager = &world.WorldManager{
		MapConfigs:    map[string]*config.MapConfig{"church": {SkyTexture: "church_panorama"}},
		CurrentMapKey: "church",
	}
	game.dropFlyWithoutOpenSky()
	if game.flyActive || game.flyDuration != 0 {
		t.Fatalf("Fly must fade indoors (active=%v duration=%d)", game.flyActive, game.flyDuration)
	}
}

// TestStoneBlossomMortar: the bloom detonates exactly at its scheduled point,
// damaging and stunning monsters there and sparing everything near the caster.
func TestStoneBlossomMortar(t *testing.T) {
	game, _, ts := tbBehaviorGame(t, 12, 12)
	equipSpellAndPrepareCaster(t, game.combat, "stone_blossom", 200, 30)
	placePlayerAtTile(game, 2, 2, ts)
	game.camera.Angle = 0 // facing +X

	far := monster.NewMonster3DFromConfig(game.camera.X+7*ts, game.camera.Y, "goblin", game.config)
	far.MaxHitPoints, far.HitPoints = 500, 500
	near := monster.NewMonster3DFromConfig(game.camera.X+ts, game.camera.Y+ts, "goblin", game.config)
	near.MaxHitPoints, near.HitPoints = 500, 500
	game.world.Monsters = []*monster.Monster3D{far, near}

	if !game.combat.CastEquippedSpell() {
		t.Fatal("mortar cast failed")
	}
	if len(game.pendingMortars) != 1 {
		t.Fatalf("pendingMortars = %d, want 1", len(game.pendingMortars))
	}
	flight := game.pendingMortars[0].FramesLeft
	for i := 0; i < flight+2 && len(game.pendingMortars) > 0; i++ {
		game.tickPendingMortars()
	}
	if len(game.pendingMortars) != 0 {
		t.Fatal("mortar never detonated")
	}
	if far.HitPoints >= 500 {
		t.Fatal("bloom did not damage the landing-zone monster")
	}
	if far.StunFramesRemaining <= 0 && far.StunTurnsRemaining <= 0 {
		t.Fatal("bloom did not stun the landing-zone monster")
	}
	if near.HitPoints < 500 {
		t.Fatal("bloom must not hit monsters near the caster (7-tile dead zone)")
	}

	// A bloom in flight belongs to ITS map: transitions (map switch/load/new
	// game all run clearTransientCombatState) must drop it.
	if !game.combat.CastEquippedSpell() {
		t.Fatal("second mortar cast failed")
	}
	if len(game.pendingMortars) == 0 {
		t.Fatal("second mortar not scheduled")
	}
	game.clearTransientCombatState()
	if len(game.pendingMortars) != 0 {
		t.Fatal("map transition must clear in-flight mortars")
	}
}

// TestFireShieldBuff: +50 fire resist for the party, fire only.
func TestFireShieldBuff(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	equipSpellAndPrepareCaster(t, game.combat, "fire_shield", 200, 30)
	if !game.combat.CastEquippedSpell() {
		t.Fatal("fire shield cast failed")
	}
	if got := game.combatBuffSchoolResistPct("fire"); got != 50 {
		t.Fatalf("fire resist buff = %d, want 50", got)
	}
	if got := game.combatBuffSchoolResistPct("water"); got != 0 {
		t.Fatalf("water resist buff = %d, want 0 (fire only)", got)
	}
}

// TestArenaDayFlips: the arena clock advances on BOTH dusk and dawn, and the
// paid bones doze jumps straight to the requested phase.
func TestArenaDayFlips(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	day0 := game.dayNightDay
	game.advanceDayNightToPhase(true) // doze until nightfall
	if !game.dayNightIsNight {
		t.Fatal("expected night after dozing to nightfall")
	}
	if game.dayNightDay != day0+1 {
		t.Fatalf("arena day = %d, want %d (dusk counts)", game.dayNightDay, day0+1)
	}
	game.advanceDayNightToPhase(false) // doze until dawn
	if game.dayNightIsNight {
		t.Fatal("expected day after dozing to dawn")
	}
	if game.dayNightDay != day0+2 {
		t.Fatalf("arena day = %d, want %d (dawn counts too)", game.dayNightDay, day0+2)
	}
}

// TestDayNightDozeNeverRewinds: dozing to a phase the party is ALREADY in must
// fast-forward a full cycle, never wind the clock backward (the paid-rest bug).
func TestDayNightDozeNeverRewinds(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	cycle := game.dayNightCycleFrames()
	if cycle <= 0 {
		t.Skip("map has no day/night cycle")
	}
	// Deep into night (frac ~0.6), then doze "until nightfall" - same phase.
	game.dayNightFrames = 3 * cycle / 5
	game.dayNightIsNight = true
	before := game.dayNightFrames
	day0 := game.dayNightDay
	game.advanceDayNightToPhase(true)
	if !game.dayNightIsNight {
		t.Fatal("still night after dozing to the next nightfall")
	}
	// Forward-only: reaching the next nightfall from mid-night wraps through
	// dawn (day) back to dusk (night) - two flips, and the frame lands at the
	// nightfall mark, which is EARLIER in the cycle than `before` (wrapped).
	if game.dayNightDay != day0+2 {
		t.Fatalf("arena day = %d, want %d (dawn+dusk crossed)", game.dayNightDay, day0+2)
	}
	if game.dayNightFrames != cycle/4+1 {
		t.Fatalf("frames = %d, want nightfall %d", game.dayNightFrames, cycle/4+1)
	}
	// The move wrapped forward past the cycle end, not backward within it.
	if game.dayNightFrames >= before {
		t.Fatalf("frames %d did not wrap (was %d) - clock moved backward", game.dayNightFrames, before)
	}
}

// TestDualSchoolTownPortal: learnable through EITHER school; files into the
// school the learner has open and never silently opens the other.
func TestDualSchoolTownPortal(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	fillTestParty(t, cs.game)
	ch := cs.game.party.Members[0]
	ch.MagicSchools = map[character.MagicSchoolID]*character.MagicSkill{
		character.MagicSchoolAir: {},
	}
	id := spells.SpellID("town_portal")
	if !ch.HasSchoolOpenFor(id) {
		t.Fatal("air-open character must qualify for the dual-school spell")
	}
	if !ch.LearnSpell(id) {
		t.Fatal("learn failed")
	}
	if ch.MagicSchools[character.MagicSchoolEarth] != nil {
		t.Fatal("learning through Air must not open Earth")
	}
	if !ch.KnowsSpell(id) {
		t.Fatal("spell not filed under the open school")
	}
	if ch.LearnSpell(id) {
		t.Fatal("re-learn must report already known")
	}
}
