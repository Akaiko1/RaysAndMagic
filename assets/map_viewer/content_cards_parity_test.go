package main

import (
	"path/filepath"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/spells"
)

// The game-side TestCardParity_* checks the SHARED builders
// (character.WeaponCardSections etc.). This test exercises the map editor's OWN
// wrappers (weaponCard/spellCard in content_cards.go) for EVERY weapon and spell,
// asserting each wrapper's rows actually CONTAIN the shared builder's output —
// rendered here with the same args the wrapper should use. That catches a wiring
// break the game-side test can't see: a dropped RenderCardLines call, the
// spellCard sdErr branch swallowing rows, or the wrong ArmorPhysicalReductionDivisor.
func TestEditorCardsWired(t *testing.T) {
	root := func(name string) string { return filepath.Join("..", "..", "assets", name) }
	if _, err := config.LoadWeaponConfig(root("weapons.yaml")); err != nil {
		t.Fatalf("load weapons: %v", err)
	}
	if _, err := config.LoadSpellConfig(root("spells.yaml")); err != nil {
		t.Fatalf("load spells: %v", err)
	}
	if _, err := config.LoadItemConfig(root("items.yaml")); err != nil {
		t.Fatalf("load items: %v", err)
	}
	if _, err := config.LoadTrapConfig(root("traps.yaml")); err != nil {
		t.Fatalf("load traps: %v", err)
	}

	body := func(rows []string) string { return strings.Join(rows, "\n") }
	index := func(cards []contentCard) map[string]contentCard {
		m := make(map[string]contentCard, len(cards))
		for _, c := range cards {
			m[c.key] = c
		}
		return m
	}

	// Weapons: every weapon def must have a card whose rows contain the exact
	// shared-builder output (right divisor, fully wired).
	weaponCards := index(buildWeaponCards())
	wProcessed := 0
	for key, def := range config.GlobalWeapons.Weapons {
		if def == nil {
			continue
		}
		wProcessed++
		shared := body(character.RenderCardLines(
			character.WeaponCardSections(def, character.ArmorPhysicalReductionDivisor), true))
		c, ok := weaponCards[key]
		if !ok {
			t.Errorf("weapon %q: no editor card built", key)
			continue
		}
		if !strings.Contains(body(c.tooltipRows), shared) {
			t.Errorf("weapon %q: wrapper rows don't contain the shared builder output (wiring/divisor drift):\n--- wrapper ---\n%s\n--- shared ---\n%s",
				key, body(c.tooltipRows), shared)
		}
	}
	if wProcessed != len(config.GlobalWeapons.Weapons) {
		t.Fatalf("checked %d weapons, expected %d", wProcessed, len(config.GlobalWeapons.Weapons))
	}

	if def, ok := config.GetWeaponDefinition("tonbogiri"); ok && def != nil {
		c, ok := weaponCards["tonbogiri"]
		if !ok {
			t.Fatalf("tonbogiri: no editor card built")
		}
		if def.Flavor == "" {
			t.Fatalf("tonbogiri flavor was not loaded")
		}
		if c.flavor != def.Flavor {
			t.Fatalf("tonbogiri card flavor = %q, want %q", c.flavor, def.Flavor)
		}
	} else {
		t.Fatalf("tonbogiri missing from weapons.yaml")
	}

	// Spells: same, picking the shared builder the wrapper should use (monster-only
	// spells render MonsterSpellCardSections, not the player formula).
	spellCards := index(buildSpellCards()) // includes trap cards; we only look up spell keys
	sProcessed := 0
	for key, def := range config.GlobalSpells.Spells {
		if def == nil {
			continue
		}
		sProcessed++
		sd, err := spells.GetSpellDefinitionByID(spells.SpellID(key))
		if err != nil {
			t.Fatalf("spell %q in GlobalSpells but GetSpellDefinitionByID failed: %v", key, err)
		}
		var sections []character.CardSection
		if def.MonsterOnly {
			sections = character.MonsterSpellCardSections(def, sd)
		} else {
			sections = character.SpellCardSections(key, def, sd, character.ArmorPhysicalReductionDivisor)
		}
		shared := body(character.RenderCardLines(sections, true))
		c, ok := spellCards[key]
		if !ok {
			t.Errorf("spell %q: no editor card built", key)
			continue
		}
		if !strings.Contains(body(c.tooltipRows), shared) {
			t.Errorf("spell %q: wrapper rows don't contain the shared builder output (wiring/divisor drift):\n--- wrapper ---\n%s\n--- shared ---\n%s",
				key, body(c.tooltipRows), shared)
		}
	}
	if sProcessed != len(config.GlobalSpells.Spells) {
		t.Fatalf("checked %d spells, expected %d", sProcessed, len(config.GlobalSpells.Spells))
	}

	// Item cards (armor/accessory/consumable/quest/trinket) vary in shape; just
	// prove the wrapper populates rows for every one (catches a total wiring break).
	items := buildItemCards()
	if len(items) == 0 {
		t.Fatal("items: no cards built")
	}
	for _, c := range items {
		if len(c.tooltipRows) == 0 {
			t.Errorf("item card %q has no tooltip rows", c.key)
		}
	}
}
