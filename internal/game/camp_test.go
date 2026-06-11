package game

import (
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/monster"
)

func TestTryCamp_RefusedNearEnemiesAndWithoutFood(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorld(cfg))
	g.party.Food = 5

	// A living monster within 5 tiles of the camera → no rest.
	near := &monster.Monster3D{Name: "Goblin", HitPoints: 10, MaxHitPoints: 10,
		X: g.camera.X + 3*float64(cfg.World.TileSize), Y: g.camera.Y}
	g.world.Monsters = append(g.world.Monsters, near)

	if _, ok := g.TryCamp(); ok {
		t.Error("camp should be refused with an enemy 3 tiles away")
	}
	if g.party.Food != 5 {
		t.Errorf("refused camp must not spend food, have %d", g.party.Food)
	}

	// An ENGAGED monster blocks the camp from any distance (no resting
	// mid-fight by kiting the pursuer out of the radius).
	near.X = g.camera.X + 9*float64(cfg.World.TileSize)
	near.IsEngagingPlayer = true
	if _, ok := g.TryCamp(); ok {
		t.Error("camp should be refused while a monster is engaged, even beyond the radius")
	}
	near.IsEngagingPlayer = false

	// Dead monsters don't count; empty larder still refuses.
	near.HitPoints = 0
	g.party.Food = 0
	if _, ok := g.TryCamp(); ok {
		t.Error("camp should be refused with no food")
	}
}

func TestTryCamp_RestoresPartyAndSpendsFood(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorld(cfg))
	g.party.Food = 2

	// A far-away monster must not block the rest.
	far := &monster.Monster3D{Name: "Goblin", HitPoints: 10, MaxHitPoints: 10,
		X: g.camera.X + 10*float64(cfg.World.TileSize), Y: g.camera.Y}
	g.world.Monsters = append(g.world.Monsters, far)

	hurt := g.party.Members[0]
	hurt.HitPoints = 1
	hurt.SpellPoints = 0
	ko := g.party.Members[1]
	ko.HitPoints = 0
	ko.AddCondition(character.ConditionUnconscious)
	dead := g.party.Members[2]
	dead.HitPoints = 0
	dead.AddCondition(character.ConditionDead)

	msg, ok := g.TryCamp()
	if !ok {
		t.Fatalf("camp refused: %s", msg)
	}
	if g.party.Food != 1 {
		t.Errorf("food = %d, want 1", g.party.Food)
	}
	if hurt.HitPoints != hurt.MaxHitPoints || hurt.SpellPoints != hurt.MaxSpellPoints {
		t.Error("wounded member not fully restored")
	}
	if ko.HitPoints != ko.MaxHitPoints || ko.HasCondition(character.ConditionUnconscious) {
		t.Error("unconscious member should wake fully healed")
	}
	if dead.HitPoints != 0 || !dead.HasCondition(character.ConditionDead) {
		t.Error("camping must not revive the dead")
	}
}

func TestTavernRestAndBuyFood(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorld(cfg))
	ih := &InputHandler{game: g}
	g.party.Gold = 25
	hurt := g.party.Members[0]
	hurt.HitPoints = 1

	// Too poor for rations at 30 gold.
	ih.handleBuyFood(&character.NPCDialogueChoice{Action: "buy_food", Cost: 30, Amount: 5})
	if g.party.Gold != 25 {
		t.Errorf("gold spent on refused purchase: %d", g.party.Gold)
	}

	startFood := g.party.Food
	ih.handleBuyFood(&character.NPCDialogueChoice{Action: "buy_food", Cost: 10, Amount: 5})
	if g.party.Gold != 15 || g.party.Food != startFood+5 {
		t.Errorf("buy_food: gold=%d food=%d, want 15/%d", g.party.Gold, g.party.Food, startFood+5)
	}

	// Too poor for a 20-gold room with 15 gold left.
	g.dialogActive = true
	ih.handleTavernRest(&character.NPCDialogueChoice{Action: "tavern_rest", Cost: 20})
	if g.party.Gold != 15 {
		t.Errorf("refused rest must not charge: gold=%d, want 15", g.party.Gold)
	}
	if hurt.HitPoints == hurt.MaxHitPoints {
		t.Error("rest should have been refused (15 gold < 20)")
	}

	g.party.Gold = 50
	ih.handleTavernRest(&character.NPCDialogueChoice{Action: "tavern_rest", Cost: 20})
	if g.party.Gold != 30 {
		t.Errorf("rest gold = %d, want 30", g.party.Gold)
	}
	if hurt.HitPoints != hurt.MaxHitPoints {
		t.Error("rest should fully heal the party")
	}
	if g.dialogActive {
		t.Error("rest should close the dialog")
	}
}
