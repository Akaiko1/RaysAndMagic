package game

import (
	"fmt"
	"testing"
	"ugataima/internal/config"
)

// Presentation contract: race derives standard rarity; class overrides win.
// Geometry cells cover small, standard, tall and ultrawide screens. Rendering
// does not persist rarity or modify character statistics (save/load N/A).
func TestHeroCardRarityAndLayout(t *testing.T) {
	cfg, err := config.LoadConfig("../../config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ class, race, want string }{
		{"knight", "", "common"}, {"cleric", "human", "common"},
		{"knight", "half_orc", "uncommon"}, {"sorcerer", "dark_elf", "uncommon"},
		{"cleric", "celestial", "uncommon"}, {"archer", "halfling", "uncommon"},
		{"arms_master", "", "rare"}, {"monk", "", "rare"}, {"monk", "dark_elf", "rare"},
		{"battle_mage", "", "legendary"}, {"battle_mage", "dark_elf", "legendary"},
	} {
		t.Run(tc.class+"/"+tc.race, func(t *testing.T) {
			h := &pcHero{entry: config.RosterEntry{Class: tc.class, Race: tc.race}}
			if got := h.cardRarity(cfg); got != tc.want {
				t.Fatalf("rarity=%s want %s", got, tc.want)
			}
		})
	}
	pc := newPartyCreateState(cfg)
	for _, size := range [][2]int{{800, 680}, {1024, 768}, {1280, 720}, {1920, 1080}, {3440, 1440}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			l := partyCreateLayout(pc, size[0], size[1])
			all := append([]rect{l.detail, l.begin, l.back}, l.slots[:]...)
			all = append(all, l.pool...)
			for i, r := range all {
				if r.x < 0 || r.y < 0 || r.x+r.w > size[0] || r.y+r.h > size[1] {
					t.Fatalf("rectangle %d outside viewport: %+v", i, r)
				}
				for j, q := range all[:i] {
					if r.x < q.x+q.w && q.x < r.x+r.w && r.y < q.y+q.h && q.y < r.y+r.h {
						t.Fatalf("rectangles %d and %d overlap", j, i)
					}
				}
			}
		})
	}
}

// The new tips screen owns clicks at both resolutions and in both play modes.
func TestControlTipsDisplayedNavigation(t *testing.T) {
	for _, size := range [][2]int{{800, 680}, {1920, 1080}} {
		for _, tb := range []bool{false, true} {
			t.Run(fmt.Sprintf("%dx%d/tb=%v", size[0], size[1], tb), func(t *testing.T) {
				h := newDisplayedModalHarness(t, size[0], size[1])
				h.g.turnBasedMode = tb
				h.g.mainMenuOpen, h.g.mainMenuMode = true, MenuMain
				index := -1
				for i, option := range mainMenuOptions {
					if option.key == "control_tips" {
						index = i
					}
				}
				if index < 0 {
					t.Fatal("Control Tips option missing")
				}
				w, height := menuPanelSize(MenuMain)
				px, py := (size[0]-w)/2, (size[1]-height)/2
				r, _, _ := menuRowRect(px, py, w, mainMenuListTopY, mainMenuRowPitch, index)
				h.clicks(false, (r.x1+r.x2)/2, (r.y1+r.y2)/2, 1)
				if h.g.mainMenuMode != MenuControlTips {
					t.Fatal("click did not open tips")
				}
				w, height = menuPanelSize(MenuControlTips)
				px, py = (size[0]-w)/2, (size[1]-height)/2
				h.clicks(false, px+24+menuBackButtonW/2, py+height-46+menuBackButtonH/2, 1)
				if h.g.mainMenuMode != MenuMain || !h.g.mainMenuOpen {
					t.Fatal("Back did not return to pause menu")
				}
			})
		}
	}
}

// Prefer filling the available row before wrapping at the same card size.
func TestHeroRosterUsesAvailableWidth(t *testing.T) {
	cfg, err := config.LoadConfig("../../config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	pc := newPartyCreateState(cfg)
	for _, physical := range [][2]int{{1280, 720}, {1920, 1080}, {2560, 1440}, {3440, 1440}, {3840, 2160}} {
		w, h := logicalScreenSize(physical[0], physical[1])
		l := partyCreateLayout(pc, w, h)
		for i, r := range l.pool {
			// Scrolled-out cards have no displayed rectangle.
			if i == 0 || r.w == 0 || l.pool[i-1].w == 0 || r.y == l.pool[i-1].y {
				continue
			}
			prev := l.pool[i-1]
			if prev.x+prev.w+12+r.w <= w-20 {
				t.Fatalf("%v: wrapped with enough space left in row", physical)
			}
		}
	}
}
