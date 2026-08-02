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

	// A living monster within 5 tiles of the camera -> no rest.
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

func TestTryCamp_AllowsDistantCrossfireAwayFromParty(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorld(cfg))
	g.party.Food = 2
	tile := float64(cfg.GetTileSize())

	ally := &monster.Monster3D{ID: "bound_ally", Name: "Bound Skeleton", HitPoints: 10, MaxHitPoints: 10,
		X: g.camera.X + 11*tile, Y: g.camera.Y, Bound: true}
	enemy := &monster.Monster3D{ID: "crossfire_enemy", Name: "Goblin", HitPoints: 10, MaxHitPoints: 10,
		X: g.camera.X + 10*tile, Y: g.camera.Y, AIFoe: ally, IsEngagingPlayer: true}
	g.world.Monsters = []*monster.Monster3D{enemy, ally}

	if _, ok := g.TryCamp(); !ok {
		t.Fatal("a distant enemy fighting a bound ally must not be treated as a party fight")
	}
}

// The whole ally-source matrix against camping, so a new source cannot be added
// without landing on one side of the rule. Bound creatures - every pure summon
// plus Bind Undead converts - are allies and do not block a camp. Charm is not
// an alliance but a countdown that breaks on any hit, so a pacified monster
// still blocks: resting through it would bank a full heal seconds before it
// turns hostile again.
func TestTryCamp_BoundAlliesAllowCampButCharmDoesNot(t *testing.T) {
	for _, tc := range allySourceCases() {
		t.Run(tc.name, func(t *testing.T) {
			g, _ := summonTileWorld(t)
			g.party.Food = 1
			ally := tc.spawn(t, g)
			if !ally.IsPartyControlled() {
				t.Fatal("test source did not create a party-controlled monster")
			}

			msg, ok := g.TryCamp()
			if ally.Bound {
				if !ok {
					t.Fatalf("nearby bound ally blocked camp: %s", msg)
				}
				if g.party.Food != 0 {
					t.Fatalf("successful camp left %d food, want 0", g.party.Food)
				}
				return
			}
			if ok {
				t.Fatalf("charmed monster allowed a camp: %s", msg)
			}
			if g.party.Food != 1 {
				t.Fatalf("refused camp spent food: %d, want 1", g.party.Food)
			}
		})
	}
}

// Charm is a countdown that breaks on any hit, not an alliance: a pacified
// monster next to the party still blocks the camp, or the party banks a full
// heal moments before it turns hostile again.
func TestTryCamp_PacifiedMonsterStillBlocksCamp(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorld(cfg))
	g.party.Food = 2
	tile := float64(cfg.GetTileSize())

	charmed := &monster.Monster3D{ID: "charmed", Name: "Charmed Dragon", HitPoints: 40, MaxHitPoints: 40,
		X: g.camera.X + tile, Y: g.camera.Y, Pacified: true, PacifiedFramesRemaining: 60}
	g.world.Monsters = []*monster.Monster3D{charmed}

	if !charmed.IsPartyControlled() {
		t.Fatal("setup: a pacified monster should read as party-controlled")
	}
	if msg, ok := g.TryCamp(); ok {
		t.Fatalf("camp succeeded next to a charmed monster: %s", msg)
	}
	if g.party.Food != 2 {
		t.Errorf("refused camp still spent food: %d, want 2", g.party.Food)
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
