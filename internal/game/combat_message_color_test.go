package game

import (
	"fmt"
	"image/color"
	"testing"

	"ugataima/internal/items"
)

func sameColor(a, b color.Color) bool {
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}

func TestCombatLogHistoryKeepsMoreThanHUD(t *testing.T) {
	g, _ := newThiefTestGame(t)
	g.maxMessages = 4
	for i := 0; i < maxCombatLogHistory+5; i++ {
		g.AddCombatMessage(fmt.Sprintf("message %d", i))
	}

	if got := len(g.GetCombatMessages()); got != 4 {
		t.Fatalf("HUD messages = %d, want 4", got)
	}
	if got := len(g.combatLogHistory); got != maxCombatLogHistory {
		t.Fatalf("history messages = %d, want %d", got, maxCombatLogHistory)
	}
	if got := g.combatLogHistory[0].Text; got != "message 5" {
		t.Fatalf("oldest retained message = %q, want message 5", got)
	}
}

func TestCombatLogOpensOnDoubleClick(t *testing.T) {
	g, _ := newThiefTestGame(t)
	g.maxMessages = 4
	g.AddCombatMessage("clickable")
	ih := NewInputHandler(g)
	x, y, _, _ := combatMessageArea(g)

	g.mouseLeftClicks = append(g.mouseLeftClicks, queuedClick{x: x + 10, y: y + 10})
	if !ih.handleCombatLogOpenInput() || g.combatLogOpen {
		t.Fatal("first log click should be consumed without opening")
	}
	g.mouseLeftClicks = append(g.mouseLeftClicks, queuedClick{x: x + 10, y: y + 10})
	if !ih.handleCombatLogOpenInput() || !g.combatLogOpen {
		t.Fatal("second log click should open full history")
	}
}

func TestLootMessageColor(t *testing.T) {
	tests := []struct {
		name  string
		items []items.Item
		want  color.Color
	}{
		{
			name:  "ordinary item is gold",
			items: []items.Item{{Name: "Iron Sword", Rarity: "common"}},
			want:  combatMessageGold,
		},
		{
			name: "legendary item uses legendary color",
			items: []items.Item{
				{Name: "Iron Sword", Rarity: "common"},
				{Name: "Bow of Hellfire", Rarity: "legendary"},
			},
			want: rarityColor("legendary"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := lootMessageColor(tt.items); !sameColor(got, tt.want) {
				t.Fatalf("lootMessageColor() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPickupLegendaryItemUsesLegendaryMessageColor(t *testing.T) {
	g, _ := newThiefTestGame(t)
	g.groundContainers = []GroundContainer{{
		Kind:  ContainerKindLootBag,
		Items: []items.Item{{Name: "Bow of Hellfire", Rarity: "legendary"}},
	}}

	g.pickupGroundContainerAt(0)

	if got := g.GetCombatMessages(); len(got) != 1 {
		t.Fatalf("combat messages = %v, want one pickup message", got)
	}
	if got := g.GetCombatMessageColor(0); !sameColor(got, rarityColor("legendary")) {
		t.Fatalf("pickup message color = %v, want legendary color", got)
	}
}
