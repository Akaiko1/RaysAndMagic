package main

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ugataima/internal/boot"
	"ugataima/internal/game"
	"ugataima/internal/items"
	"ugataima/internal/stash"
	"ugataima/internal/storage"
)

func TestSavedItemAccentUsesTypeWithoutRarity(t *testing.T) {
	for _, tc := range []struct {
		kind items.ItemType
		want contentKind
	}{
		{items.ItemWeapon, cardWeapon}, {items.ItemArmor, cardItem}, {items.ItemAccessory, cardItem},
		{items.ItemConsumable, cardItem}, {items.ItemQuest, cardItem}, {items.ItemTrinket, cardItem}, {items.ItemCard, cardItem},
		{items.ItemBattleSpell, cardSpell}, {items.ItemUtilitySpell, cardSpell}, {items.ItemTrap, cardSpell},
	} {
		for _, rarity := range []string{"", "legendary"} {
			it := items.Item{Name: "Unknown saved item", Type: tc.kind, Rarity: rarity}
			got := cardForSavedItem(it)
			if got.kind != tc.want {
				t.Fatalf("type %v became card kind %v", tc.kind, got.kind)
			}
			expected := contentCard{kind: tc.want, rarity: rarity}
			if cardAccentColor(&got) != cardAccentColor(&expected) {
				t.Fatalf("wrong accent for %+v", it)
			}
		}
	}
}

func TestMobInfoScrollRetainsFractionalWheelInput(t *testing.T) {
	original := mobsPage
	t.Cleanup(func() { mobsPage = original })
	mobsPage.info = make([]infoLine, 20)
	mobsPage.infoCapacity = 10
	for _, tc := range []struct {
		name   string
		start  float64
		deltas []float64
		want   float64
	}{
		{"small downward", 0, []float64{-.1, -.1, -.1, -.1}, 1.2},
		{"small upward", 5, []float64{.1, .1, .1, .1}, 3.8},
		{"whole wheel", 0, []float64{-1}, 3},
		{"reverse", 5, []float64{-.2, .2}, 5},
		{"top discards overscroll", 0, []float64{10, -.2}, .6},
		{"bottom discards overscroll", 10, []float64{-10, .2}, 9.4},
		{"resize clamps", 15, []float64{0}, 10},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mobsPage.infoOffset = tc.start
			for _, delta := range tc.deltas {
				scrollMobInfo(delta)
			}
			if math.Abs(mobsPage.infoOffset-tc.want) > 1e-8 {
				t.Fatalf("offset=%v want=%v", mobsPage.infoOffset, tc.want)
			}
		})
	}
}

func TestMapSidebarUsesLoadTimeLighting(t *testing.T) {
	t.Chdir("../..")
	cfg, _ := boot.LoadGameData()
	maps, err := loadMaps(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range maps {
		if m.LightingText != game.MapLightingText(m.Config) {
			t.Fatalf("%s: missing cached lighting", m.Key)
		}
		old := *m.Config
		copyConfig := old
		m.Config = &copyConfig
		m.Config.SkyTexture = "missing_preview_sky"
		m.Config.AmbientLight = .123
		for range 3 {
			rows := buildMapInfoLines(m, brush{kind: brushEraser})
			found := false
			for _, row := range rows {
				if row.text == m.LightingText {
					found = true
				}
			}
			if !found {
				t.Fatalf("%s: draw re-read lighting instead of cached metadata", m.Key)
			}
		}
	}
}

func TestSaveBrowserCollectionUsesLoadRulesReadOnly(t *testing.T) {
	t.Chdir("../..")
	boot.LoadGameData()
	storage.SetDataRootForTesting(t.TempDir())
	t.Cleanup(func() { storage.SetDataRootForTesting("") })
	card := items.CreateItemFromYAML("troll_card")
	card.InstanceID = 987
	owned := stash.Stash{}
	owned.CardSlots[0] = card
	if err := stash.Save(&owned); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		party game.PartySave
		want  int
	}{
		{"noncard", game.PartySave{CardCollection: []string{"granite"}}, 0},
		{"missing", game.PartySave{CardCollection: []string{"missing"}}, 0},
		{"legacy owned", game.PartySave{CardCollection: []string{"troll_card"}}, 0},
		{"physical owned", game.PartySave{CardCollectionItems: []items.Item{card}}, 0},
		{"slot cap", game.PartySave{CardCollection: []string{"puma_card", "puma_card", "puma_card", "puma_card", "puma_card", "puma_card", "puma_card", "puma_card", "puma_card"}}, game.MaxCardSlots},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, _ := json.Marshal(game.GameSave{Party: tc.party})
			path := filepath.Join(t.TempDir(), "save.json")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			stashBefore, _ := os.ReadFile(storage.AppSavePath("stash.json"))
			_, rows := buildSaveDetail(path, "Fixture", game.SaveSummary{})
			count := 0
			for _, row := range rows {
				if row.item != nil {
					count++
					if row.item.Type != items.ItemCard {
						t.Fatal("noncard in collection")
					}
				}
			}
			if count != tc.want {
				t.Fatalf("collection size=%d want=%d", count, tc.want)
			}
			after, _ := os.ReadFile(path)
			stashAfter, _ := os.ReadFile(storage.AppSavePath("stash.json"))
			if !bytes.Equal(data, after) || !bytes.Equal(stashBefore, stashAfter) {
				t.Fatal("browsing wrote save or stash")
			}
		})
	}
	// Both authored prose fields must survive the actual editor card path.
	for _, c := range buildItemsCards() {
		if c.key == "tonbogiri" {
			text := strings.Join(c.tooltipRows, "\n")
			for _, want := range []string{"A legendary spear so keen", "A dragonfly that lit"} {
				if !strings.Contains(text, want) {
					t.Fatal(text)
				}
			}
		}
	}
}
