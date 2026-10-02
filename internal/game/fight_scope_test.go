package game

import (
	"encoding/json"
	"fmt"
	"testing"

	uitext "ugataima/assets/text"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/monster"
)

// Invariant: the party is in a fight only while a party-hostile monster is
// close enough to press it - within its pursuit leash or its attack reach.
// Case table:
//   - Hostility: calm, sight chase, sticky hit, relentless, bound ally.
//   - Distance: inside camp radius, inside leash, beyond leash (stranded),
//     beyond leash but inside attack reach, lured past tether (wider leash).
//   - Entry points: TryCamp, brewSelectedRecipe, the M key on the Alchemist
//     (closed menu and open on another tab), Drakehide shedding, automatic
//     techniques. Wording follows the action: camp rests, the book brews.
//   - Persistence: sticky hostility survives save/load in both world modes;
//     a hunter stranded in another open-world region does not hold the party.
func TestFightScopeIsReachNotTheWholeMap(t *testing.T) {
	type row struct {
		name             string
		tiles            float64
		hostile          func(m *monster.Monster3D)
		reachTiles       float64
		luredPastTether  bool
		wantCombat       bool
		wantNear         bool
		wantSheddingHold bool
	}
	engaged := func(m *monster.Monster3D) { m.IsEngagingPlayer = true; m.State = monster.StatePursuing }
	sticky := func(m *monster.Monster3D) { engaged(m); m.WasAttacked = true }
	relentless := func(m *monster.Monster3D) { sticky(m); m.Relentless = true }
	bound := func(m *monster.Monster3D) { m.Bound = true }
	rows := []row{
		{name: "calm_far", tiles: 10},
		{name: "calm_near", tiles: 3, wantNear: true},
		{name: "sight_chase_inside_leash", tiles: 7, hostile: engaged, wantCombat: true, wantSheddingHold: true},
		{name: "sticky_inside_leash", tiles: 7, hostile: sticky, wantCombat: true, wantSheddingHold: true},
		{name: "sticky_stranded", tiles: 30, hostile: sticky},
		{name: "relentless_inside_leash", tiles: 7, hostile: relentless, wantCombat: true, wantSheddingHold: true},
		{name: "relentless_stranded", tiles: 30, hostile: relentless},
		{name: "sticky_beyond_leash_inside_reach", tiles: 10, hostile: sticky, reachTiles: 11, wantCombat: true, wantSheddingHold: true},
		{name: "sticky_lured_past_tether", tiles: 12, hostile: sticky, luredPastTether: true, wantCombat: true, wantSheddingHold: true},
		{name: "bound_ally_near", tiles: 2, hostile: bound},
	}
	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			g, _ := rareClassGame(t, character.ClassAlchemist, false)
			ts := float64(g.config.GetTileSize())
			// Leash = 4-tile alert x 2 hysteresis = 8 tiles at the post; 16 when lured.
			m := &monster.Monster3D{ID: "probe", Key: "goblin", Name: "Goblin", HitPoints: 10, MaxHitPoints: 10,
				X: g.camera.X + tc.tiles*ts, Y: g.camera.Y, AlertRadius: 4 * ts, TetherRadius: 4 * ts,
				AttackRadius: tc.reachTiles * ts}
			m.SpawnX, m.SpawnY = m.X, m.Y
			if tc.luredPastTether {
				m.SpawnX += 20 * ts
			}
			if tc.hostile != nil {
				tc.hostile(m)
			}
			g.world.Monsters = []*monster.Monster3D{m}

			wantCamp, wantBrew := "", ""
			switch {
			case tc.wantCombat:
				wantCamp, wantBrew = uitext.Text("ui.camp_in_combat"), uitext.Text("ui.brew_in_combat")
			case tc.wantNear:
				wantCamp, wantBrew = uitext.Text("ui.camp_enemies_near"), uitext.Text("ui.brew_enemies_near")
			}

			g.party.Food = 1
			msg, ok := g.TryCamp()
			if ok != (wantCamp == "") || (!ok && msg != wantCamp) {
				t.Fatalf("camp = %v %q, want refusal %q", ok, msg, wantCamp)
			}

			// The book opens on every route; only the brew transaction is gated.
			ih := &InputHandler{game: g}
			for _, from := range []string{"closed", "inventory"} {
				g.menuOpen, g.currentTab = from == "inventory", TabInventory
				ih.toggleTabbedMenu(TabSpellbook)
				if !g.menuOpen || g.currentTab != TabSpellbook {
					t.Fatalf("M from %s did not open the Alchemist book", from)
				}
			}
			g.selectedRare = 0
			stockAlchemyRecipe(g, &config.GlobalAlchemy.Recipes[0], 1)
			g.rareBookMessage = ""
			brewed := g.brewSelectedRecipe()
			if brewed != (wantBrew == "") || (!brewed && g.rareBookMessage != wantBrew) {
				t.Fatalf("brew = %v %q, want refusal %q", brewed, g.rareBookMessage, wantBrew)
			}

			if got := g.anyMonsterEngagingParty(); got != tc.wantSheddingHold {
				t.Fatalf("scale shedding held = %v, want %v", got, tc.wantSheddingHold)
			}
			if got := g.automaticTechniqueTrigger("quickening"); got != tc.wantCombat {
				t.Fatalf("automatic Quickening = %v, want %v", got, tc.wantCombat)
			}
		})
	}
}

func TestStrandedHunterSurvivesSaveWithoutHoldingTheParty(t *testing.T) {
	for _, unified := range []bool{false, true} {
		for _, place := range []string{"same_region_inside_leash", "other_region"} {
			if place == "other_region" && !unified {
				continue // a split map holds only its own roster
			}
			t.Run(fmt.Sprintf("open=%v/%s", unified, place), func(t *testing.T) {
				t.Chdir("../..")
				g, wm, cfg := bootOpenWorldGame(t, unified)
				if err := g.switchToMap("dragon_cliffs"); err != nil {
					t.Fatal(err)
				}
				ts := cfg.GetTileSize()
				// The authored start tile: a region start unified, the map's own split.
				g.camera.X, g.camera.Y = g.world.GetStartingPosition()
				if sx, sy, ok := wm.OpenWorldRegionStart("dragon_cliffs"); ok {
					g.camera.X, g.camera.Y = sx, sy
				}
				x, y := g.camera.X+6*ts, g.camera.Y
				if place == "other_region" {
					var ok bool
					if x, y, ok = wm.OpenWorldRegionStart("deep_jungle"); !ok {
						t.Fatal("deep_jungle region has no start")
					}
				}
				hunter := monster.NewMonster3DFromConfig(x, y, "ocelot", cfg)
				hunter.WasAttacked, hunter.IsEngagingPlayer, hunter.State = true, true, monster.StatePursuing
				g.world.Monsters = []*monster.Monster3D{hunter}

				data, err := json.Marshal(g.buildSave(wm))
				if err != nil {
					t.Fatal(err)
				}
				var loaded GameSave
				if err := json.Unmarshal(data, &loaded); err != nil {
					t.Fatal(err)
				}
				if err := g.applySave(wm, &loaded); err != nil {
					t.Fatal(err)
				}
				var got *monster.Monster3D
				for _, m := range g.world.Monsters {
					if m.Key == "ocelot" && m.WasAttacked {
						got = m
					}
				}
				if got == nil {
					t.Fatal("sticky hostility was not persisted")
				}
				_, safe := g.safeToPrepare(campActivity())
				if want := place == "other_region"; safe != want {
					t.Fatalf("camp safe = %v, want %v (hunter %.1f tiles away)", safe, want, Distance(got.X, got.Y, g.camera.X, g.camera.Y)/ts)
				}
			})
		}
	}
}
