package game

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"ugataima/internal/config"
	"ugataima/internal/monster"
)

// Every authored route x both directions x each active/retired checkpoint.
// Save indices are retained; only interior Forest points may be retired.
func TestCaravanForestTransitAndLegacyCheckpoints(t *testing.T) {
	g, wm, ts := ecologyTestGame(t)
	if err := config.LoadEcology("../../assets/ecology.yaml"); err != nil {
		t.Fatal(err)
	}
	config.GlobalEcology.Populations = nil
	config.GlobalEcology.Fish = nil
	g.world = newTestWorldSized(g.config, 60, 50)
	wm.LoadedMaps["forest"] = g.world
	wm.CurrentMapKey = "forest"
	for _, route := range config.GlobalEcology.Caravan.Routes {
		active := []config.RoutePoint{}
		for _, p := range route.Points {
			if p.Skip && p.Map != "forest" {
				t.Fatal("detour removed outside Forest")
			}
			if p.Map == "forest" && !p.Skip {
				active = append(active, p)
			}
		}
		if len(active) != 2 || active[0].X != 48 || active[0].Y != 15 || active[1].X != 46 || active[1].Y != 7 {
			t.Fatalf("%s does not go directly between Forest entrances: %+v", route.ID, active)
		}
		for _, returning := range []bool{false, true} {
			for index, p := range route.Points {
				if p.Map != "forest" {
					continue
				}
				for _, entry := range []string{"target", "update"} {
					t.Run(fmt.Sprintf("%s/return%v/checkpoint%d/%s", route.ID, returning, index, entry), func(t *testing.T) {
						m := monster.NewMonster3DFromConfig(40.5*ts, 12.5*ts, "desert_caravan", g.config)
						g.world.Monsters = []*monster.Monster3D{m}
						g.ecology = EcologyState{Unlocked: true, ActorID: m.ID, Route: route.ID, Returning: returning, Checkpoint: index}
						// Old saves encode only the integer index, with no knowledge of Skip.
						data, err := json.Marshal(g.ecology)
						if err != nil {
							t.Fatal(err)
						}
						g.ecology = EcologyState{}
						if err := json.Unmarshal(data, &g.ecology); err != nil {
							t.Fatal(err)
						}
						if entry == "target" {
							g.setCaravanTarget(m)
						} else {
							g.updateEcology()
						}
						expected := p
						if p.Skip {
							expected = active[1]
							if returning {
								expected = active[0]
							}
						}
						got := route.Points[g.ecology.Checkpoint]
						if got != expected {
							t.Fatalf("checkpoint redirected to %+v, want %+v", got, expected)
						}
						if m.X != 40.5*ts || m.Y != 12.5*ts {
							t.Fatal("checkpoint migration teleported caravan")
						}
						if entry == "target" && (m.AITargetX != (float64(expected.X)+.5)*ts || m.AITargetY != (float64(expected.Y)+.5)*ts) {
							t.Fatal("retired waypoint remained the travel target")
						}
					})
				}
			}
		}
	}
}

func TestCaravanDetourSaveRestoreAndAlertReset(t *testing.T) {
	for _, returning := range []bool{false, true} {
		t.Run(fmt.Sprint(returning), func(t *testing.T) {
			g, wm, ts := ecologyTestGame(t)
			if err := config.LoadEcology("../../assets/ecology.yaml"); err != nil {
				t.Fatal(err)
			}
			g.world = newTestWorldSized(g.config, 60, 50)
			wm.LoadedMaps["forest"] = g.world
			wm.CurrentMapKey = "forest"
			m := monster.NewMonster3DFromConfig(30.5*ts, 14.5*ts, "desert_caravan", g.config)
			g.world.Monsters = []*monster.Monster3D{m}
			g.ecology = EcologyState{Unlocked: true, ActorID: m.ID, Route: "bandit_wells", Returning: returning}
			route := g.caravanRoute()
			for i, p := range route.Points {
				if p.Skip {
					g.ecology.Checkpoint = i
					break
				}
			}
			saved := g.buildSave(wm)
			data, err := json.Marshal(saved)
			if err != nil {
				t.Fatal(err)
			}
			var loaded GameSave
			if err := json.Unmarshal(data, &loaded); err != nil {
				t.Fatal(err)
			}
			g.caravanAttackAlertUntil = time.Now().Add(time.Hour)
			if err := g.applySave(wm, &loaded); err != nil {
				t.Fatal(err)
			}
			_, restored := g.ecologyActor()
			if restored == nil {
				t.Fatal("saved caravan missing")
			}
			g.setCaravanTarget(restored)
			if route.Points[g.ecology.Checkpoint].Skip {
				t.Fatal("save resumed a retired detour")
			}
			if !g.caravanAttackAlertUntil.IsZero() {
				t.Fatal("loaded timeline inherited an old HUD cooldown")
			}
			g.notifyCaravanAttack(restored)
			assertCaravanAlerts(t, g, 1)
			if restored.X != m.X || restored.Y != m.Y {
				t.Fatal("save migration moved caravan")
			}
		})
	}
}
