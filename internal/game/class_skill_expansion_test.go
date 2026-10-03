package game

import (
	"slices"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
	"ugataima/internal/items"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

// Every authored class kit reaches a new hero exactly: each listed skill at its
// authored start mastery and nothing else, each school with exactly its listed
// spells, the main-hand weapon, every starting item in the slot its equip_slot
// names, and the quick slot. The class is registered end to end and offered by
// a roster hero, since party creation pools roster entries only.
func TestClassKitsMatchAuthoredClassYAML(t *testing.T) {
	cfg := loadTestConfig(t)
	active, captives, recruits := character.StartingRoster(cfg)
	rostered := map[string]bool{}
	for _, e := range slices.Concat(active, captives, recruits) {
		rostered[e.Class] = true
	}
	if len(cfg.Characters.Classes) != len(character.PlayableClasses) {
		t.Fatalf("config authors %d classes, PlayableClasses lists %d", len(cfg.Characters.Classes), len(character.PlayableClasses))
	}
	for key, stats := range cfg.Characters.Classes {
		t.Run(key, func(t *testing.T) {
			class, ok := character.ClassFromKey(key)
			if !ok || class.Key() != key || !slices.Contains(character.PlayableClasses, class) {
				t.Fatalf("class key %q is not registered as a playable class", key)
			}
			if class.String() == "" || len(stats.Description) == 0 {
				t.Errorf("class %q lacks a display name or pitch", key)
			}
			if !rostered[key] {
				t.Errorf("no roster hero offers class %q", key)
			}

			ch := character.CreateCharacter("Kit", class, cfg)
			if len(ch.Skills) != len(stats.Skills) {
				t.Errorf("hero has %d skills, kit lists %d", len(ch.Skills), len(stats.Skills))
			}
			for _, sk := range stats.Skills {
				st, ok := character.SkillTypeFromKey(sk)
				if !ok {
					t.Fatalf("kit lists unknown skill %q", sk)
				}
				want := character.MasteryNovice
				if tier, ok := stats.SkillStartMastery[sk]; ok {
					want, _ = character.MasteryFromKey(tier)
				}
				if s := ch.Skills[st]; s == nil || s.Mastery != want {
					t.Errorf("skill %s = %+v, want mastery %v", sk, s, want)
				}
			}

			if len(ch.MagicSchools) != len(stats.Magic) {
				t.Errorf("hero has %d schools, kit lists %d", len(ch.MagicSchools), len(stats.Magic))
			}
			for _, entry := range stats.Magic {
				ms := ch.MagicSchools[character.MagicSchoolID(entry.School)]
				if ms == nil || ms.Mastery != character.MasteryNovice {
					t.Errorf("school %s = %+v, want an open Novice school", entry.School, ms)
					continue
				}
				known := make([]string, len(ms.KnownSpells))
				for i, id := range ms.KnownSpells {
					known[i] = string(id)
				}
				if !slices.Equal(known, entry.Spells) {
					t.Errorf("school %s knows %v, kit lists %v", entry.School, known, entry.Spells)
				}
			}

			want := map[items.EquipSlot]string{}
			if stats.MainHand != "" {
				def, ok := config.GetWeaponDefinition(stats.MainHand)
				if !ok {
					t.Fatalf("main_hand %q is not in weapons.yaml", stats.MainHand)
				}
				want[items.SlotMainHand] = def.Name
			}
			for _, itemKey := range stats.Equipment {
				def, ok := config.GetItemDefinition(itemKey)
				if !ok {
					t.Fatalf("equipment %q is not in items.yaml", itemKey)
				}
				slot := items.SlotArmor // an item without equip_slot wears as body armor
				if def.EquipSlot != "" {
					slot, _ = items.EquipSlotFromName(def.EquipSlot)
				}
				if _, taken := want[items.SlotRing1]; slot == items.SlotRing1 && taken {
					slot = items.SlotRing2
				}
				want[slot] = def.Name
			}
			if stats.QuickTrap != "" {
				def, ok := config.GetTrapDefinition(stats.QuickTrap)
				if !ok {
					t.Fatalf("quick_trap %q is not in traps.yaml", stats.QuickTrap)
				}
				want[items.SlotSpell] = def.Name
			}
			if stats.QuickSpell != "" {
				def, err := spells.GetSpellDefinitionByID(spells.SpellID(stats.QuickSpell))
				if err != nil {
					t.Fatalf("quick_spell %q: %v", stats.QuickSpell, err)
				}
				want[items.SlotSpell] = def.Name
			}
			if len(ch.Equipment) != len(want) {
				t.Errorf("hero wears %d items, kit authors %d", len(ch.Equipment), len(want))
			}
			for slot, name := range want {
				if got, ok := ch.Equipment[slot]; !ok || got.Name != name {
					t.Errorf("slot %s = %q, want %q", slot.DisplayName(), got.Name, name)
				}
			}
		})
	}
}

func TestBlasterRequiresAndUsesBlasterMastery(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	archer := character.CreateCharacter("Archer", character.ClassArcher, cs.game.config)
	thief := character.CreateCharacter("Thief", character.ClassThief, cs.game.config)
	sorcerer := character.CreateCharacter("Sorcerer", character.ClassSorcerer, cs.game.config)
	// Firearms need no training: every trained character can equip a blaster,
	// with or without the Blaster skill. The skill only pays mastery bonuses
	// (asserted below).
	for _, member := range []*character.MMCharacter{archer, thief, sorcerer} {
		if !member.CanEquipWeaponByName("Alien Blaster") {
			t.Errorf("%s cannot equip a Blaster - blasters need no training", member.Class)
		}
	}
	if sorcerer.HasSkill(character.SkillBlaster) {
		t.Fatal("the Sorcerer kit must not include the Blaster skill (it tests the untrained case)")
	}
	if trueDmg, _ := cs.weaponMasteryStrike(sorcerer, lookupWeaponConfigByKey("alien_blaster")); trueDmg != 0 {
		t.Errorf("untrained blaster mastery = %d true damage, want 0", trueDmg)
	}

	blaster := lookupWeaponConfigByKey("alien_blaster")
	if blaster == nil {
		t.Fatal("alien_blaster missing")
	}
	archer.Skills[character.SkillBlaster].Mastery = character.MasteryGrandMaster
	trueDamage, ignoresDodge := cs.weaponMasteryStrike(archer, blaster)
	if trueDamage != 3*MasteryWeaponTrueDamagePerTier || !ignoresDodge {
		t.Fatalf("GM Blaster mastery = (%d,%v), want (%d,true)",
			trueDamage, ignoresDodge, 3*MasteryWeaponTrueDamagePerTier)
	}
}

func TestElementalGMConvertsOnlySchoolMasteryBonusToTrueDamage(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	caster := character.CreateCharacter("Sorcerer", character.ClassSorcerer, cs.game.config)
	wantTrue := int(character.MasteryGrandMaster) * MasterySpellEffectPerLevel
	cases := []struct {
		school character.MagicSchoolID
		spell  spells.SpellID
	}{
		{character.MagicSchoolFire, "firebolt"},
		{character.MagicSchoolWater, "ice_bolt"},
		{character.MagicSchoolAir, "lightning"},
		{character.MagicSchoolEarth, "rock_blast"},
		{character.MagicSchoolLight, "ray_of_light"},
		{character.MagicSchoolDark, "darkbolt"},
	}
	for _, tc := range cases {
		t.Run(tc.school.String(), func(t *testing.T) {
			caster.MagicSchools[tc.school] = &character.MagicSkill{Mastery: character.MasteryGrandMaster}
			_, _, total := cs.CalculateSpellDamage(tc.spell, caster)
			parts := cs.spellDamageParts(tc.spell, caster, total)
			if parts.True != wantTrue || parts.Normal != total-wantTrue {
				t.Fatalf("GM %s parts = %+v, total %d, want true %d", tc.school, parts, total, wantTrue)
			}
		})
	}

	caster.MagicSchools[character.MagicSchoolFire].Mastery = character.MasteryMaster
	_, _, total := cs.CalculateSpellDamage("fireball", caster)
	parts := cs.spellDamageParts("fireball", caster, total)
	if parts != (damagecalc.Parts{Normal: total}) {
		t.Fatalf("non-GM elemental spell split into true damage: %+v", parts)
	}

	caster.MagicSchools[character.MagicSchoolBody] = &character.MagicSkill{Mastery: character.MasteryGrandMaster}
	_, _, total = cs.CalculateSpellDamage("harm", caster)
	if parts = cs.spellDamageParts("harm", caster, total); parts != (damagecalc.Parts{Normal: total}) {
		t.Fatalf("self-magic GM spell split into true damage: %+v", parts)
	}
	if got := cs.spellResistPierce(caster, "harm"); got != SelfMagicGMResistPiercePct {
		t.Fatalf("self-magic GM pierce = %d, want %d", got, SelfMagicGMResistPiercePct)
	}
}

func TestElementalSpellTrueDamageReachesPrimaryAndAoeTargets(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	caster := character.CreateCharacter("Sorcerer", character.ClassSorcerer, cs.game.config)
	caster.MagicSchools[character.MagicSchoolFire].Mastery = character.MasteryGrandMaster

	primary := mkTestMonster("Primary", 100)
	secondary := mkTestMonster("Secondary", 100)
	primary.X, primary.Y = 64, 64
	secondary.X, secondary.Y = 96, 64
	cs.game.world.Monsters = []*monsterPkg.Monster3D{primary, secondary}

	projectile := &MagicProjectile{
		Active: true, LifeTime: 1,
		Damage: 0, TrueDamage: 15,
		SpellType: "fireball", Attacker: caster,
	}
	cs.applyProjectileDamage(projectile, "magic_projectile", primary, "elemental-true-test")

	if got := 100 - primary.HitPoints; got != 15 {
		t.Fatalf("primary received %d typed true damage, want 15", got)
	}
	if got := 100 - secondary.HitPoints; got != 15 {
		t.Fatalf("AoE target received %d typed true damage, want 15", got)
	}
}

// Inferno is a nova: its authored cost sets the base and each mastery tier adds
// its authored step, all of it normal damage.
func TestInfernoScalesByMasteryAndStaysNormalDamage(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	def, err := spells.GetSpellDefinitionByID("inferno")
	if err != nil {
		t.Fatal(err)
	}
	if def.MasteryDamagePerTier <= 0 {
		t.Fatalf("inferno authors mastery_damage_per_tier %d, want a positive step", def.MasteryDamagePerTier)
	}
	caster := character.CreateCharacter("Sorcerer", character.ClassSorcerer, cs.game.config)
	for tier := character.MasteryNovice; tier <= character.MasteryGrandMaster; tier++ {
		caster.MagicSchools[character.MagicSchoolFire].Mastery = tier
		want := def.SpellPointsCost*spells.SpellDamagePerSP + int(tier)*def.MasteryDamagePerTier
		got := cs.CalculatePartyNovaDamage(def, caster)
		if got != want {
			t.Errorf("Inferno tier %d = %d, want %d", tier, got, want)
		}
		if parts := cs.spellDamageParts(def.ID, caster, got); parts != (damagecalc.Parts{Normal: got}) {
			t.Errorf("Inferno tier %d produced true damage: %+v", tier, parts)
		}
	}
}

func TestNaturalHealerScalesWholeSpellHeal(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	cleric := character.CreateCharacter("Cleric", character.ClassCleric, cs.game.config)
	delete(cleric.Skills, character.SkillNaturalHealer)
	_, _, base := cs.CalculateSpellHealing("heal_other", cleric)
	for tier := 0; tier <= 3; tier++ {
		cleric.Skills[character.SkillNaturalHealer] = &character.Skill{Mastery: character.SkillMastery(tier)}
		_, _, got := cs.CalculateSpellHealing("heal_other", cleric)
		want := base * (100 + character.NaturalHealerBonusPct(tier)) / 100
		if got != want {
			t.Errorf("Natural Healer tier %d = %d, want %d", tier, got, want)
		}
	}

	cleric.Skills[character.SkillNaturalHealer].Mastery = character.MasteryGrandMaster
	target := character.CreateCharacter("Target", character.ClassKnight, cs.game.config)
	target.HitPoints = 1
	cleric.SpellPoints = 100
	cs.game.party.Members = []*character.MMCharacter{cleric, target}
	cs.game.selectedChar = 0
	def, err := spells.GetSpellDefinitionByID("heal_other")
	if err != nil {
		t.Fatal(err)
	}
	_, _, wantHeal := cs.CalculateSpellHealing("heal_other", cleric)
	if !cs.castKnownHealOn("heal_other", def, 1) {
		t.Fatal("Natural Healer's targeted heal did not cast")
	}
	wantHP := 1 + wantHeal
	if wantHP > target.MaxHitPoints {
		wantHP = target.MaxHitPoints
	}
	if target.HitPoints != wantHP {
		t.Fatalf("targeted Natural Healer restored target to %d HP, want %d", target.HitPoints, wantHP)
	}
}

func TestImpenetrableDefenseAndSacrificeUsePostMitigationDamage(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	knight := character.CreateCharacter("Knight", character.ClassKnight, cs.game.config)
	knight.Equipment = map[items.EquipSlot]items.Item{}
	knight.Skills[character.SkillImpenetrableDefense].Mastery = character.MasteryNovice
	got := cs.mitigateCharacterDamageParts(damagecalc.Parts{Normal: 20, True: 20}, "physical", knight, false)
	if got.Normal != 17 || got.True != 20 {
		t.Fatalf("Impenetrable Defense result = %+v, want normal 17 true 20", got)
	}
	cs.game.combatBuffs = []TimedCombatBuff{{SpellID: "test", Frames: 1, ResistPct: 50, InReduce: 5}}
	got = cs.mitigateCharacterDamageParts(damagecalc.Parts{Normal: 100, True: 20}, "physical", knight, false)
	if got.Normal != 42 || got.True != 10 {
		t.Fatalf("mitigation order result = %+v, want resist -> personal -3 -> buff -5 and resisted true 10", got)
	}
	cs.game.combatBuffs = nil

	victim := character.CreateCharacter("Victim", character.ClassSorcerer, cs.game.config)
	protector := character.CreateCharacter("Protector", character.ClassPaladin, cs.game.config)
	victim.HitPoints, victim.MaxHitPoints = 1000, 1000
	protector.HitPoints, protector.MaxHitPoints = 1000, 1000
	protector.Skills[character.SkillSacrifice].Mastery = character.MasteryGrandMaster
	protector.Equipment[items.SlotGauntlets] = items.CreateItemFromYAML("drakehide_gauntlets")
	cs.game.party.Members = []*character.MMCharacter{victim, protector}
	if remaining := cs.redirectDamageThroughSacrifice(victim, 100); remaining != 50 {
		t.Fatalf("victim retained %d damage, want 50", remaining)
	}
	if protector.HitPoints != 950 {
		t.Fatalf("protector HP = %d, want 950", protector.HitPoints)
	}
	if protector.ScaleStacks != 1 {
		t.Fatalf("protector scale stacks = %d, want 1 after redirected damage", protector.ScaleStacks)
	}
}

func TestAnimalBondingBearCopiesDruidStatsAndIsPureSummon(t *testing.T) {
	oldTiles := world.GlobalTileManager
	world.GlobalTileManager = world.NewTileManager(testTileSizeClasses())
	if err := world.GlobalTileManager.LoadTileConfig("../../assets/tiles.yaml"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { world.GlobalTileManager = oldTiles })

	game, _, _ := tbBehaviorGame(t, 12, 12)
	druid := character.CreateCharacter("Druid", character.ClassDruid, game.config)
	druid.Skills[character.SkillAnimalBonding].Mastery = character.MasteryMaster
	druid.MaxHitPoints, druid.HitPoints = 250, 250
	druid.Equipment[items.SlotArmor] = items.CreateItemFromYAML("leather_armor")
	game.party.Members = []*character.MMCharacter{druid}
	placePlayerAtTile(game, 5, 5, float64(game.config.GetTileSize()))

	wantArmor := game.combat.CalculateTotalArmorClass(druid) * character.AnimalBondingStatPct(2) / 100
	weapon := druid.Equipment[items.SlotMainHand]
	_, _, attack := game.combat.CalculateWeaponDamage(weapon, druid)
	if def := lookupWeaponConfigByName(weapon.Name); def != nil {
		trueDamage, _ := game.combat.weaponMasteryStrike(druid, def)
		attack += trueDamage
	}
	wantAttack := attack * character.AnimalBondingStatPct(2) / 100

	if !game.combat.summonAnimalBondingBear(druid) {
		t.Fatal("Animal Bonding could not place a bear")
	}
	if len(game.world.Monsters) != 1 {
		t.Fatalf("summoned monsters = %d, want 1", len(game.world.Monsters))
	}
	bear := game.world.Monsters[0]
	if bear.Key != "bear" || !bear.Bound || !isPurePartySummon(bear) || !bear.QuestProgressIgnored {
		t.Fatalf("bear lifecycle flags are wrong: key=%q bound=%v owner=%q ignored=%v",
			bear.Key, bear.Bound, bear.SummonedBy, bear.QuestProgressIgnored)
	}
	wantHP := druid.MaxHitPoints * character.AnimalBondingHPPct(2) / 100
	if bear.MaxHitPoints != wantHP || bear.ArmorClass != wantArmor ||
		bear.DamageMin != wantAttack || bear.DamageMax != wantAttack {
		t.Fatalf("bear copied stats HP=%d AC=%d damage=%d-%d; want %d/%d/%d",
			bear.MaxHitPoints, bear.ArmorClass, bear.DamageMin, bear.DamageMax, wantHP, wantArmor, wantAttack)
	}
	bear.Experience = 999
	if xp := game.combat.awardExperienceAndGold(bear); xp != 0 {
		t.Fatalf("pure bear summon awarded %d XP", xp)
	}
	game.crumbleBoundAlliesOnDeparture(game.world)
	if len(game.world.Monsters) != 0 {
		t.Fatal("Animal Bonding bear survived map departure")
	}
}

func TestAnimalBondingCapsLivingBearsPerDruid(t *testing.T) {
	game, _ := summonTileWorld(t)
	druid := character.CreateCharacter("Druid", character.ClassDruid, game.config)
	druid.Skills[character.SkillAnimalBonding].Mastery = character.MasteryGrandMaster
	game.party.Members = []*character.MMCharacter{druid}

	for i := 0; i < character.AnimalBondingSummonMax; i++ {
		if !game.combat.summonAnimalBondingBear(druid) {
			t.Fatalf("Animal Bonding bear %d/%d was rejected", i+1, character.AnimalBondingSummonMax)
		}
	}
	if game.combat.summonAnimalBondingBear(druid) {
		t.Fatalf("Animal Bonding exceeded its %d-bear live cap", character.AnimalBondingSummonMax)
	}
	owner := animalBondingOwner(druid)
	if got := game.combat.countLiveSummonsByOwner(owner); got != character.AnimalBondingSummonMax {
		t.Fatalf("living Animal Bonding bears = %d, want %d", got, character.AnimalBondingSummonMax)
	}

	game.world.Monsters[0].HitPoints = 0
	if !game.combat.summonAnimalBondingBear(druid) {
		t.Fatal("a dead Animal Bonding bear did not release its summon slot")
	}
	if got := game.combat.countLiveSummonsByOwner(owner); got != character.AnimalBondingSummonMax {
		t.Fatalf("living bears after replacement = %d, want %d", got, character.AnimalBondingSummonMax)
	}
}

// An old hero holding one earned kit skill gains every other current kit skill
// at its start mastery; earned mastery is never overwritten, and a second pass
// has nothing to add.
func TestExistingCharacterAddsOnlyMissingCurrentKitSkills(t *testing.T) {
	cfg := newTestCombatSystemWithConfig(t).game.config
	for key, stats := range cfg.Characters.Classes {
		t.Run(key, func(t *testing.T) {
			class, _ := character.ClassFromKey(key)
			if len(stats.Skills) < 2 {
				t.Fatalf("kit lists %d skills; the case needs one kept and one missing", len(stats.Skills))
			}
			kept, _ := character.SkillTypeFromKey(stats.Skills[0])
			old := &character.MMCharacter{
				Class:  class,
				Skills: map[character.SkillType]*character.Skill{kept: {Mastery: character.MasteryMaster}},
			}
			if !old.EnsureClassKitSkills(cfg) {
				t.Fatal("old hero was not migrated")
			}
			fresh := character.CreateCharacter("Kit", class, cfg)
			if len(old.Skills) != len(fresh.Skills) {
				t.Errorf("migrated hero has %d skills, a new hero %d", len(old.Skills), len(fresh.Skills))
			}
			for st, s := range fresh.Skills {
				if st == kept {
					continue
				}
				if got := old.Skills[st]; got == nil || got.Mastery != s.Mastery {
					t.Errorf("migrated %s = %+v, want mastery %v", st, got, s.Mastery)
				}
			}
			if old.Skills[kept].Mastery != character.MasteryMaster {
				t.Error("class-kit migration overwrote earned mastery")
			}
			if old.EnsureClassKitSkills(cfg) {
				t.Error("a second migration pass reported added skills")
			}
		})
	}
}

func TestApplySaveMigratesNewClassSkillsInEveryRoster(t *testing.T) {
	cfg := loadTestConfig(t)
	const mapKey = "class_skill_migration"
	makeWorldManager := func(w *world.World3D) *world.WorldManager {
		wm := world.NewWorldManager(cfg)
		wm.CurrentMapKey = mapKey
		wm.LoadedMaps = map[string]*world.World3D{mapKey: w}
		return wm
	}
	kitSkills := func(c *character.MMCharacter) []character.SkillType {
		var out []character.SkillType
		for _, sk := range cfg.Characters.Classes[c.Class.Key()].Skills {
			st, _ := character.SkillTypeFromKey(sk)
			out = append(out, st)
		}
		return out
	}
	// An old save: each hero kept only the first skill of its current kit.
	oldHero := func(name string, class character.CharacterClass) *character.MMCharacter {
		c := character.CreateCharacter(name, class, cfg)
		if len(kitSkills(c)) < 2 {
			t.Fatalf("%s kit lists fewer than two skills; nothing would be missing", class.Key())
		}
		for _, st := range kitSkills(c)[1:] {
			delete(c.Skills, st)
		}
		return c
	}

	saveWorld := newTestWorldSized(cfg, 8, 8)
	saveGame := newTestGame(cfg, saveWorld)
	saveGame.party.Members = []*character.MMCharacter{oldHero("Old Archer", character.ClassArcher)}
	saveGame.party.Reserve = []*character.MMCharacter{oldHero("Old Thief", character.ClassThief)}
	saveGame.party.Captive = []*character.MMCharacter{oldHero("Old Cleric", character.ClassCleric)}
	save := saveGame.buildSave(makeWorldManager(saveWorld))

	loadWorld := newTestWorldSized(cfg, 8, 8)
	loaded := newTestGame(cfg, loadWorld)
	if err := loaded.applySave(makeWorldManager(loadWorld), &save); err != nil {
		t.Fatal(err)
	}
	// Load may append newly available recruits after the saved reserve.
	for _, tc := range []struct {
		roster  string
		members []*character.MMCharacter
		want    string
	}{
		{"active", loaded.party.Members, "Old Archer"},
		{"reserve", loaded.party.Reserve, "Old Thief"},
		{"captive", loaded.party.Captive, "Old Cleric"},
	} {
		if len(tc.members) == 0 || tc.members[0].Name != tc.want {
			t.Fatalf("%s roster lost %s on load", tc.roster, tc.want)
		}
		for _, st := range kitSkills(tc.members[0]) {
			if !tc.members[0].HasSkill(st) {
				t.Errorf("%s %s did not regain kit skill %s on old-save load", tc.roster, tc.want, st)
			}
		}
	}
	if !loaded.loadNeedsResave {
		t.Fatal("class-kit migration did not request a repaired save")
	}
}
