package game

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

func TestCardMoveBurstMovementContract(t *testing.T) {
	for _, rt := range []bool{false, true} {
		for _, tc := range []struct {
			name      string
			copies    int
			rooted    bool
			within    bool
			miss      bool
			empty     bool
			wantBurst bool
		}{
			{name: "pursuers and radius", copies: 1, wantBurst: true},
			{name: "duplicate cards", copies: 2, wantBurst: true},
			{name: "empty ground", copies: 1, empty: true, wantBurst: true},
			{name: "no active card"},
			{name: "zero proc chance", copies: 1, miss: true},
			{name: "rooted", copies: 1, rooted: true},
			{name: "same tile", copies: 1, within: true},
		} {
			if tc.within && !rt {
				continue // Successful TB movement always enters another tile.
			}
			t.Run(fmt.Sprintf("RT=%v/%s", rt, tc.name), func(t *testing.T) {
				g, tile := summonTileWorld(t)
				setTestWorldManager(t, nil)
				placePlayerAtTile(g, 10, 10, tile)
				g.camera.FOV = math.Pi / 3
				g.turnBasedMode = !rt
				for slot := 0; slot < tc.copies; slot++ {
					g.cardSlots[slot].key = "gorilla_titan_card"
				}
				def := cardDef("gorilla_titan_card")
				if def.CardMoveAoePct != 10 || def.CardMoveAoeDmg != 50 || def.CardMoveAoeRadiusTiles != 5 {
					t.Fatalf("unexpected authored card: %+v", def)
				}
				oldChance := def.CardMoveAoePct
				def.CardMoveAoePct = 100 // Deterministic successful RNG branch.
				if tc.miss {
					def.CardMoveAoePct = 0
				}
				t.Cleanup(func() { def.CardMoveAoePct = oldChance })
				if tc.rooted {
					g.partyRoot = PartyRootState{Frames: 120, Turns: 1}
				}
				if rt {
					g.camera.X = 11*tile - 1
					g.collisionSystem.UpdateEntity("player", g.camera.X, g.camera.Y)
				}
				g.cardBurstTileX, g.cardBurstTileY = 10, 10
				arrivalX := g.camera.X + tile
				if rt {
					arrivalX = g.camera.X + 2
				}
				wantDamage := []int{}
				if !tc.empty {
					for i, target := range []struct {
						dx, dy float64
						resist int
						immune bool
						loss   int
					}{
						{-2, 0, 0, false, 50},
						{-2, -1, 0, false, 50},
						{-5, 0, 0, false, 50},
						{-3, -4, 0, false, 50},
						{-5.01, 0, 0, false, 0},
						{-2, 0, 50, false, 25},
						{-2, 0, 0, true, 0},
					} {
						m := monster.NewMonster3DFromConfig(arrivalX+target.dx*tile, g.camera.Y+target.dy*tile, "mummy", g.config)
						m.ID = fmt.Sprintf("mummy%d", i)
						m.IsEngagingPlayer, m.WasAttacked = true, true
						m.Resistances[monster.DamagePhysical] = target.resist
						m.BossWarded = target.immune
						g.world.Monsters = append(g.world.Monsters, m)
						wantDamage = append(wantDamage, target.loss*tc.copies)
					}
					g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
				}
				ih := &InputHandler{game: g}
				if rt {
					dx := 2.0
					if tc.within {
						dx = -2 // Still inside the tracked tile.
					}
					ih.movePlayer(dx, 0)
				} else if moved := ih.moveTurnBasedInDirection(1, 0); moved == tc.rooted {
					t.Fatalf("moved=%v, rooted=%v", moved, tc.rooted)
				}
				for i, m := range g.world.Monsters {
					want := 0
					if tc.wantBurst {
						want = wantDamage[i]
					}
					if got := m.MaxHitPoints - m.HitPoints; got != want {
						t.Errorf("target %d lost %d HP, want %d", i, got, want)
					}
				}
				if g.screenShake != 0 {
					t.Errorf("card shook camera by %v", g.screenShake)
				}
				if got := len(g.spellHitEffects) > 0; got != tc.wantBurst {
					t.Fatalf("ground FX present=%v, want %v", got, tc.wantBurst)
				}
				if tc.wantBurst {
					assertCardQuakeFootprint(t, g, 5)
				}
				logged := false
				for _, message := range g.combatLogHistory {
					logged = logged || strings.Contains(message.Text, "Gorilla Titan Card erupts")
				}
				if want := tc.wantBurst && !tc.empty; logged != want {
					t.Errorf("damage message=%v, want %v", logged, want)
				}
			})
		}
	}
}

func assertCardQuakeFootprint(t *testing.T, g *MMGame, radius float64) {
	t.Helper()
	if len(g.spellHitEffects) > tileScatterMaxTiles {
		t.Fatalf("ground effect count %d exceeds budget", len(g.spellHitEffects))
	}
	tile := float64(g.config.GetTileSize())
	var reach float64
	for _, fx := range g.spellHitEffects {
		for _, p := range fx.Particles {
			d := math.Hypot(p.X-g.camera.X, p.Y-g.camera.Y) / tile
			if !p.Solid || d > radius+1e-9 {
				t.Fatalf("invalid rubble particle: solid=%v distance=%v radius=%v", p.Solid, d, radius)
			}
			reach = max(reach, d)
		}
	}
	if reach < radius-1 {
		t.Fatalf("rubble reaches only %.2f tiles, want %.2f", reach, radius)
	}
}

func TestCardMoveBurstRadiusAfterSaveLoad(t *testing.T) {
	g, tile := summonTileWorld(t)
	wm := world.NewWorldManager(g.config)
	wm.CurrentMapKey = "forest"
	wm.LoadedMaps = map[string]*world.World3D{"forest": g.world}
	setTestWorldManager(t, wm)
	for slot := 0; slot < 2; slot++ {
		if !g.setCardCollectionSlot(slot, items.CreateItemFromYAML("gorilla_titan_card")) {
			t.Fatal("could not collect card")
		}
	}
	save := g.buildSave(wm)
	loaded := newTestGame(g.config, g.world)
	if err := loaded.applySave(wm, &save); err != nil {
		t.Fatal(err)
	}
	if loaded.cardMoveAoePct() != 20 || loaded.cardMoveAoeDmg() != 100 || loaded.cardMoveAoeRadiusTiles() != 5 {
		t.Fatal("loaded duplicate cards changed burst chance, damage or radius")
	}
	if got := strings.Join(loaded.cardCollectionEffectLines(), "\n"); !strings.Contains(got, "20% on move: 100 physical true damage within 5 tiles") {
		t.Fatalf("aggregate tooltip does not match loaded cards: %s", got)
	}
	def := cardDef("gorilla_titan_card")
	oldRadius := def.CardMoveAoeRadiusTiles
	def.CardMoveAoeRadiusTiles = 3.5
	t.Cleanup(func() { def.CardMoveAoeRadiusTiles = oldRadius })
	if loaded.cardMoveAoeRadiusTiles() != 3.5 || !strings.Contains(cardEffectText(def), "within 3.5 tiles") ||
		!strings.Contains(strings.Join(loaded.cardCollectionEffectLines(), "\n"), "within 3.5 tiles") {
		t.Fatal("runtime and tooltips must use the live catalog radius")
	}
	loaded.combat = NewCombatSystem(loaded)
	m := monster.NewMonster3DFromConfig(loaded.camera.X-4*tile, loaded.camera.Y, "mummy", g.config)
	loaded.world.Monsters = []*monster.Monster3D{m}
	if loaded.combat.cardMoveBurstApply(100, loaded.cardMoveAoeRadiusTiles()) {
		t.Fatal("target outside changed catalog radius was hit")
	}
}

func TestCardMoveBurstReentry(t *testing.T) {
	for _, rt := range []bool{false, true} {
		t.Run(fmt.Sprintf("RT=%v", rt), func(t *testing.T) {
			g, tile := summonTileWorld(t)
			setTestWorldManager(t, nil)
			placePlayerAtTile(g, 10, 10, tile)
			g.turnBasedMode = !rt
			g.cardSlots[0].key = "gorilla_titan_card"
			def := cardDef("gorilla_titan_card")
			old := def.CardMoveAoePct
			def.CardMoveAoePct = 100
			t.Cleanup(func() { def.CardMoveAoePct = old })
			g.cardBurstTileX, g.cardBurstTileY = 10, 10
			ih := &InputHandler{game: g}
			beforeSP, beforeAP := g.party.Members[0].SpellPoints, g.party.Members[0].ActionsRemaining
			for _, dir := range []int{1, -1} {
				before := len(g.spellHitEffects)
				if rt {
					ih.movePlayer(float64(dir)*tile, 0)
				} else if !ih.moveTurnBasedInDirection(dir, 0) {
					t.Fatal("empty-ground movement rejected")
				}
				if len(g.spellHitEffects) <= before {
					t.Fatalf("direction %d produced no new burst", dir)
				}
				after := len(g.spellHitEffects)
				g.maybeCardMoveBurst()
				if len(g.spellHitEffects) != after {
					t.Fatal("same tile retriggered without movement")
				}
			}
			if g.party.Members[0].SpellPoints != beforeSP || g.party.Members[0].ActionsRemaining != beforeAP {
				t.Fatal("card proc spent spell points or actions")
			}
		})
	}
}
