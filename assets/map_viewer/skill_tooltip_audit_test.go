package main

import (
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
)

func TestEditorSkillReferencesUseCompleteLiveCatalog(t *testing.T) {
	oldTechniques := config.GlobalTechniques
	t.Cleanup(func() { config.GlobalTechniques = oldTechniques })
	if _, err := config.LoadWeaponConfig("../weapons.yaml"); err != nil {
		t.Fatal(err)
	}
	if _, err := config.LoadItemConfig("../items.yaml"); err != nil {
		t.Fatal(err)
	}
	if err := config.LoadTechniques("../techniques.yaml"); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{}
	for _, skill := range character.AllSkills {
		want[skill.String()] = skill.Description()
	}
	for _, school := range character.AllMagicSchools {
		want[school.DisplayName()+" Magic"] = character.MagicMasteryDescription(school)
	}
	for _, card := range buildSkillCards() {
		description, ok := want[card.name]
		if !ok {
			continue // Primary stats are not skill references.
		}
		delete(want, card.name)
		t.Run(card.name, func(t *testing.T) {
			if card.description != description {
				t.Fatal("editor bypassed the canonical skill description")
			}
			var rendered []string
			for _, line := range cardTooltipLines(&card) {
				if line.kind == tooltipLineDescription {
					rendered = append(rendered, line.text)
				}
			}
			compact := func(s string) string { return strings.Join(strings.Fields(s), "") }
			if compact(strings.Join(rendered, " ")) != compact(description) {
				t.Fatal("editor wrapping lost skill details")
			}
			if w, h := cardTooltipSize(&card); w > windowWidth-8 || h > windowHeight-pageBarHeight-8 {
				t.Fatalf("skill reference escapes editor viewport: %dx%d", w, h)
			}
		})
	}
	if len(want) > 0 {
		t.Fatalf("missing editor skill references: %v", want)
	}
}
