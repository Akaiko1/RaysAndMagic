package game

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
	uitext "ugataima/assets/text"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/spells"
)

// Editing wording must not change which rows are shown. This drives the same
// catalog -> game/editor formatter path used at boot, with all loaded content.
func TestTooltipWordingDoesNotControlEffectSelection(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	ch := gmReferenceChar(cs.game.config)
	source, err := filepath.Abs("../../assets/text")
	if err != nil {
		t.Fatal(err)
	}
	if err := uitext.LoadDirectory(source); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := uitext.LoadDirectory(source); err != nil {
			t.Error(err)
		}
	})
	render := func() map[string]string {
		result := map[string]string{}
		for _, full := range []bool{false, true} {
			add := func(kind, key, game string, sections []character.CardSection) {
				prefix := fmt.Sprintf("%s/%s/full=%v/", kind, key, full)
				result[prefix+"game"] = game
				result[prefix+"editor"] = strings.Join(character.RenderCardLines(sections, full), "\n")
			}
			for key, def := range config.GlobalItems.Items {
				add("item", key, GetItemTooltip(items.CreateItemFromYAML(key), ch, cs, full), character.ItemCardSections(def))
			}
			for key, def := range config.GlobalWeapons.Weapons {
				add("weapon", key, GetItemTooltip(items.CreateWeaponFromYAML(key), ch, cs, full), character.WeaponCardSections(def))
			}
			for key, def := range config.GlobalSpells.Spells {
				sd, err := spells.GetSpellDefinitionByID(spells.SpellID(key))
				if err != nil {
					t.Fatal(err)
				}
				add("spell", key, buildSpellTooltipUnified(sd, ch, cs, full), character.SpellCardSections(key, def, sd))
			}
			for _, key := range config.TrapKeysOrdered() {
				def, _ := config.GetTrapDefinition(key)
				add("trap", key, buildTrapTooltipUnified(key, def, ch, cs, full), character.TrapCardSections(def, config.TrapPlaceRangeTiles, config.MaxTrapsPerOwner))
			}
		}
		return result
	}
	before := render()
	dir := t.TempDir()
	paths, err := filepath.Glob(filepath.Join(source, "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var entries map[string]string
		if err := yaml.Unmarshal(data, &entries); err != nil {
			t.Fatal(err)
		}
		for key, line := range entries {
			entries[key] = "Edited " + line
		}
		data, err = yaml.Marshal(entries)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, filepath.Base(path)), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := uitext.LoadDirectory(dir); err != nil {
		t.Fatal(err)
	}
	itemSource, err := filepath.Abs("../../assets/items.yaml")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(itemSource)
	if err != nil {
		t.Fatal(err)
	}
	var itemRoot map[string]any
	if err := yaml.Unmarshal(data, &itemRoot); err != nil {
		t.Fatal(err)
	}
	defaults := itemRoot["tooltip_usage_defaults"].(map[string]any)
	for key, value := range defaults {
		switch value := value.(type) {
		case string:
			defaults[key] = "Edited " + value
		case []any:
			for i := range value {
				value[i] = "Edited " + value[i].(string)
			}
		}
	}
	data, err = yaml.Marshal(itemRoot)
	if err != nil {
		t.Fatal(err)
	}
	itemPath := filepath.Join(t.TempDir(), "items.yaml")
	if err := os.WriteFile(itemPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := config.LoadItemConfig(itemPath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := config.LoadItemConfig(itemSource); err != nil {
			t.Error(err)
		}
	})
	after := render()
	changed := 0
	for key, want := range before {
		got := after[key]
		if got != want {
			changed++
		}
		if strings.Contains(got, "%!") {
			t.Errorf("%s: broken formatting: %s", key, got)
		}
		if strings.ReplaceAll(got, "Edited ", "") != want {
			t.Errorf("%s: wording changed row selection\nBefore:\n%s\nAfter:\n%s", key, want, got)
		}
	}
	if changed == 0 {
		t.Fatal("catalog edits never reached game/editor")
	}
	t.Logf("verified %d rendered catalog views; %d contain edited templates", len(before), changed)
	// Button wording also stays separate from the displayed navigation action.
	h := newDisplayedModalHarness(t, 800, 680)
	node := &character.NPCDialogueChoice{Action: "info", Text: "Topic", Response: "Reply"}
	npc := &character.NPC{Name: "Talker", DialogueData: &character.NPCDialogue{Greeting: "Welcome", Choices: []*character.NPCDialogueChoice{node}}}
	h.g.beginConversation(npc)
	h.g.dialogNodePath = []*character.NPCDialogueChoice{node}
	clickDialogueNavigation(h, true, 1)
	if h.g.currentDialogNode() != nil {
		t.Fatal("edited Back label changed navigation")
	}
	clickDialogueNavigation(h, false, 1)
	if h.g.dialogActive {
		t.Fatal("edited Leave label changed navigation")
	}
}
