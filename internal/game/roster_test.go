package game

import (
	"testing"

	"ugataima/internal/character"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/world"
)

// TestSwapRosterMember_PreservesState swaps an active member with a reserve and
// checks the pointers (and thus all gear/XP/level) move intact.
func TestSwapRosterMember_PreservesState(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorld(cfg))

	bench := character.CreateCharacter("Auberon", character.ClassPaladin, cfg)
	bench.Level = 9
	g.party.Recruit(bench)
	benchIdx := len(g.party.Reserve) - 1 // after the tavern recruits
	active1 := g.party.Members[1]

	if !g.swapRosterMember(1, benchIdx) {
		t.Fatal("swap failed")
	}
	if g.party.Members[1] != bench {
		t.Error("reserve hero should now occupy active slot 1")
	}
	if g.party.Reserve[benchIdx] != active1 {
		t.Error("former active member should now be in reserve")
	}
	if g.party.Members[1].Level != 9 {
		t.Errorf("swapped-in level = %d, want 9 (state preserved)", g.party.Members[1].Level)
	}
}

// TestBenchXP_ReserveLevelsWithParty checks reserves get the per-member XP share
// and level up (banking points, not auto-spending).
func TestBenchXP_ReserveLevelsWithParty(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	if cs.game.party == nil {
		cs.game.party = character.NewParty(cs.game.config)
	}
	bench := character.CreateCharacter("Mirelle", character.ClassDruid, cs.game.config)
	startXP := bench.Experience
	cs.game.party.Recruit(bench)

	mon := monsterPkg.NewMonster3DFromConfig(0, 0, "troll", cs.game.config) // 300 XP
	cs.awardExperienceAndGold(mon)

	share := mon.Experience / len(cs.game.party.Members)
	if bench.Experience-startXP < share && bench.Level == 1 {
		t.Errorf("reserve gained %d XP, expected the per-member share %d (or a level-up)", bench.Experience-startXP, share)
	}
}

// TestOwedChoice_DrainsOnSwapIn_BanksOnSwapOut verifies a benched hero's owed
// level-up choice surfaces when swapped in and is re-banked when swapped out.
func TestOwedChoice_DrainsOnSwapIn_BanksOnSwapOut(t *testing.T) {
	cfg := loadTestConfig(t)
	loadTestArenaData(t) // loads level_up.yaml so GetLevelUpChoices works
	g := newTestGame(cfg, newTestWorld(cfg))

	bench := character.CreateCharacter("Lyra", character.ClassSorcerer, cfg)
	bench.Level = 5
	bench.OwedLevelChoices = []int{3} // owes the L3 sorcerer choice
	g.party.Recruit(bench)
	benchIdx := len(g.party.Reserve) - 1 // after the tavern recruits

	// Swap in → owed choice should queue for the new active slot.
	g.swapRosterMember(0, benchIdx)
	if !g.hasLevelUpChoiceForChar(0) {
		t.Fatal("owed L3 choice should be queued after swap-in")
	}
	if len(bench.OwedLevelChoices) != 0 {
		t.Errorf("owed list should be drained after swap-in, got %v", bench.OwedLevelChoices)
	}

	// Swap back out (the original member now sits in bench's old slot) →
	// un-consumed choice re-banks.
	g.swapRosterMember(0, benchIdx)
	if g.hasLevelUpChoiceForChar(0) {
		t.Error("queue should not reference the benched hero after swap-out")
	}
	if len(bench.OwedLevelChoices) != 1 || bench.OwedLevelChoices[0] != 3 {
		t.Errorf("owed L3 should be re-banked on swap-out, got %v", bench.OwedLevelChoices)
	}
}

// TestCaptivesTrainAndFree verifies imprisoned heroes start in Captive, gain the
// per-member XP share (train in prison), and move to Reserve when freed.
func TestCaptivesTrainAndFree(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	cs.game.party = character.NewParty(cs.game.config)

	if len(cs.game.party.Captive) != 2 {
		t.Fatalf("expected 2 captive heroes at start, got %d", len(cs.game.party.Captive))
	}
	cap0 := cs.game.party.Captive[0]
	startXP := cap0.Experience

	mon := monsterPkg.NewMonster3DFromConfig(0, 0, "troll", cs.game.config) // 300 XP
	cs.awardExperienceAndGold(mon)
	share := mon.Experience / len(cs.game.party.Members)
	if cap0.Experience-startXP < share && cap0.Level == 1 {
		t.Errorf("captive gained %d XP, expected per-member share %d (or a level-up)", cap0.Experience-startXP, share)
	}

	freed := cs.game.party.FreeCaptives()
	wantReserve := len(cs.game.config.Characters.TavernRecruits) + 2
	if len(freed) != 2 || len(cs.game.party.Captive) != 0 || len(cs.game.party.Reserve) != wantReserve {
		t.Errorf("after FreeCaptives: freed=%d captive=%d reserve=%d, want 2/0/%d",
			len(freed), len(cs.game.party.Captive), len(cs.game.party.Reserve), wantReserve)
	}
}

// TestSaveLoad_PersistsReserveAndCaptive round-trips the reserve roster and the
// imprisoned captives (and their banked level-up state).
func TestSaveLoad_PersistsReserveAndCaptive(t *testing.T) {
	cfg := loadTestConfig(t)

	wmSave := world.NewWorldManager(cfg)
	worldSave := newTestWorld(cfg)
	wmSave.LoadedMaps = map[string]*world.World3D{"forest": worldSave}
	wmSave.CurrentMapKey = "forest"

	game := newTestGame(cfg, worldSave)
	// A benched reserve hero + give a captive distinctive, banked progression.
	game.party.Recruit(character.CreateCharacter("Benchy", character.ClassKnight, cfg))
	if len(game.party.Captive) == 0 {
		t.Fatal("expected starting captives from config")
	}
	cap0 := game.party.Captive[0]
	cap0.Level = 7
	cap0.FreeStatPoints = 12
	cap0.OwedLevelChoices = []int{3}

	save := game.buildSave(wmSave)

	wmLoad := world.NewWorldManager(cfg)
	worldLoad := newTestWorld(cfg)
	wmLoad.LoadedMaps = map[string]*world.World3D{"forest": worldLoad}
	wmLoad.CurrentMapKey = "forest"
	old := world.GlobalWorldManager
	world.GlobalWorldManager = wmLoad
	defer func() { world.GlobalWorldManager = old }()

	loaded := newTestGame(cfg, worldLoad)
	if err := loaded.applySave(wmLoad, &save); err != nil {
		t.Fatalf("apply save: %v", err)
	}

	if len(loaded.party.Reserve) != len(game.party.Reserve) {
		t.Errorf("reserve count = %d, want %d", len(loaded.party.Reserve), len(game.party.Reserve))
	}
	foundBenchy := false
	for _, r := range loaded.party.Reserve {
		if r.Name == "Benchy" {
			foundBenchy = true
		}
	}
	if !foundBenchy {
		t.Errorf("reserve not restored (Benchy missing): %d heroes", len(loaded.party.Reserve))
	}
	if len(loaded.party.Captive) != len(game.party.Captive) {
		t.Fatalf("captive count = %d, want %d", len(loaded.party.Captive), len(game.party.Captive))
	}
	lc := loaded.party.Captive[0]
	if lc.Level != 7 || lc.FreeStatPoints != 12 || len(lc.OwedLevelChoices) != 1 || lc.OwedLevelChoices[0] != 3 {
		t.Errorf("captive progression not restored: level=%d free=%d owed=%v", lc.Level, lc.FreeStatPoints, lc.OwedLevelChoices)
	}
}

// TestPaladinCanWieldAxe confirms the Paladin's new SkillAxe lets them equip axes.
func TestPaladinCanWieldAxe(t *testing.T) {
	cfg := loadTestConfig(t)
	pal := character.CreateCharacter("Auberon", character.ClassPaladin, cfg)
	if !pal.CanEquipWeaponByName("Steel Axe") {
		t.Error("Paladin should be able to equip a Steel Axe")
	}
}
