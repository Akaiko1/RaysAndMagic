package game

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
	uitext "ugataima/assets/text"
	"ugataima/internal/character"
)

func TestPaidServiceAuthoredResponse(t *testing.T) {
	data, err := os.ReadFile("../../assets/npcs.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var catalog character.NPCConfig
	if err := yaml.Unmarshal(data, &catalog); err != nil {
		t.Fatal(err)
	}
	for key, actions := range map[string][]string{
		"solstice_camp": {"wait_until_night", "wait_until_dawn"},
		"arena_bones":   {"wait_until_night", "wait_until_dawn"},
		"tavern":        {"tavern_rest"},
	} {
		for _, action := range actions {
			choice := catalog.NPCs[key].Dialogue.TopLevelChoice(action)
			if choice == nil || choice.Response == "" {
				t.Fatalf("%s/%s has no authored wait response", key, action)
			}
			// An external prose edit with a literal percent must not become printf.
			choice.Response += " 100% sheltered."
			for _, state := range []string{"success", "poor", "already_waiting"} {
				if action == "tavern_rest" && state == "already_waiting" {
					continue
				}
				t.Run(key+"/"+action+"/"+state, func(t *testing.T) {
					g, _, _ := tbBehaviorGame(t, 5, 5)
					g.party.Gold = choice.Cost
					g.dialogNPC = &character.NPC{DialogueData: catalog.NPCs[key].Dialogue}
					g.dialogActive = true
					if state == "poor" {
						g.party.Gold--
					}
					g.dayNightSkipActive = state == "already_waiting"
					gold := g.party.Gold
					g.party.Members[0].HitPoints = 1
					g.party.Members[0].SpellPoints = 0
					rows := g.visibleNPCChoices(g.dialogNPC)
					g.selectedChoice = -1
					for i, row := range rows {
						if row == choice {
							g.selectedChoice = i
						}
					}
					if g.selectedChoice < 0 {
						t.Fatal("wait choice is not selectable")
					}
					(&InputHandler{game: g}).executeEncounterChoice()
					want := strings.ReplaceAll(choice.Response, "{cost}", fmt.Sprint(choice.Cost))
					if state == "success" {
						gold -= choice.Cost
						if g.dialogActive {
							t.Fatal("successful service must close the dialogue")
						}
						if action != "tavern_rest" {
							if !g.dayNightSkipActive {
								t.Fatal("paid wait must start time passing")
							}
							finishDayNightSkip(t, g)
							if g.dayNightIsNight != (action == "wait_until_night") {
								t.Fatal("wait advanced to the wrong phase")
							}
						}
					} else {
						want = uitext.Text("dialog.service_cannot_afford", choice.Cost)
						if state == "already_waiting" {
							want = uitext.Text("dialog.time_is_already_passing")
						}
						if !g.dialogActive || g.dayNightSkipActive != (state == "already_waiting") {
							t.Fatal("refused wait changed dialogue or time progression")
						}
					}
					messages := g.GetCombatMessages()
					if !slices.Contains(messages, want) {
						t.Fatalf("messages = %v, want %q", messages, want)
					}
					hp, sp := 1, 0
					if action == "tavern_rest" && state == "success" {
						hp, sp = g.party.Members[0].MaxHitPoints, g.party.Members[0].MaxSpellPoints
					}
					if g.party.Gold != gold || g.party.Members[0].HitPoints != hp || g.party.Members[0].SpellPoints != sp {
						t.Fatal("service restored the wrong resources or charged the wrong amount")
					}
				})
			}
		}
	}
}
