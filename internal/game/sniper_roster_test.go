package game

import (
	"encoding/json"
	"fmt"
	"testing"
	"ugataima/internal/character"
	"ugataima/internal/world"
)

func TestSniperSaveLoadAndRecruitMigration(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(fmt.Sprint(legacy), func(t *testing.T) {
			g, _, ch, tile := sniperFixture(t, false)
			m := spawnMonsterAtTile(g, "wolf", 10, 10, tile)
			ch.AutoDrinkCooldown = 37
			ch.DesignatedTargetID = m.ID
			ch.DesignationFrames = 99
			wm := world.NewWorldManager(g.config)
			wm.LoadedMaps = map[string]*world.World3D{"forest": g.world}
			wm.CurrentMapKey = "forest"
			setTestWorldManager(t, wm)
			save := g.buildSave(wm)
			if legacy {
				save.Party.Members[0] = buildCharacterSave(character.CreateCharacter("Gareth", character.ClassKnight, g.config))
				save.Party.Reserve = nil
				save.Party.Captive = nil
			}
			data, err := json.Marshal(save)
			if err != nil {
				t.Fatal(err)
			}
			var loaded GameSave
			if err = json.Unmarshal(data, &loaded); err != nil {
				t.Fatal(err)
			}
			if err = g.applySave(wm, &loaded); err != nil {
				t.Fatal(err)
			}
			if legacy {
				if len(g.party.Reserve) != 1 || g.party.Reserve[0].Name != "Mara" {
					t.Fatal("old save did not receive new recruit")
				}
				g.ensureAdditionalRecruits()
				if len(g.party.Reserve) != 1 {
					t.Fatal("duplicate recruit")
				}
			} else {
				ch = g.party.Members[0]
				if ch.AutoDrinkCooldown != 37 || ch.DesignationFrames != 99 || ch.DesignatedTargetID != g.world.Monsters[0].ID {
					t.Fatal("tactical save lost timer or monster link")
				}
				if g.overwatchReady(ch) {
					t.Fatal("load granted immediate reaction")
				}
				if g.designationBonus(g.world.Monsters[0]) == 0 {
					t.Fatal("loaded target lost its shared damage/marker eligibility")
				}
			}
		})
	}
}

func TestPartyCreateRosterAccessibleAtAllSizes(t *testing.T) {
	cfg := loadTestConfig(t)
	for _, size := range [][2]int{{800, 600}, {1024, 768}, {1366, 768}, {1920, 1080}, {2560, 1440}, {3840, 2160}} {
		for _, extra := range []int{0, 20} {
			t.Run(fmt.Sprintf("%dx%d/extra=%d", size[0], size[1], extra), func(t *testing.T) {
				pc := newPartyCreateState(cfg)
				for range extra {
					pc.pool = append(pc.pool, pc.pool[0])
				}
				seen := make(map[int]bool)
				for scroll := 0; scroll <= partyCreateLayout(pc, size[0], size[1]).poolMaxScroll; scroll++ {
					pc.poolScroll = scroll
					lay := partyCreateLayout(pc, size[0], size[1])
					for i, r := range lay.pool {
						if r.w == 0 {
							continue
						}
						seen[i] = true
						if r.w < 100 || r.x < lay.poolArea.x || r.y < lay.poolArea.y || r.x+r.w > lay.poolArea.x+lay.poolArea.w || r.y+r.h > lay.poolArea.y+lay.poolArea.h {
							t.Fatalf("unreadable or clipped card: %+v in %+v", r, lay.poolArea)
						}
					}
				}
				if len(seen) != len(pc.pool) {
					t.Fatalf("only %d of %d reachable", len(seen), len(pc.pool))
				}
			})
		}
	}
}

func TestTavernRosterScrollAndSwapThroughDisplayedInput(t *testing.T) {
	for _, embedded := range []bool{false, true} {
		t.Run(fmt.Sprint(embedded), func(t *testing.T) {
			h := newDisplayedModalHarness(t, 800, 600)
			if embedded {
				h.g.dialogActive = true
				h.g.dialogNPC = tavernTestNPC()
				h.g.dialogTab = 0
			} else {
				h.g.rosterScreenOpen = true
			}
			h.g.party.Reserve = nil
			for i := 0; i < 30; i++ {
				h.g.party.Reserve = append(h.g.party.Reserve, character.CreateCharacter(fmt.Sprintf("Reserve %02d", i), character.ClassSniper, h.g.config))
			}
			last := h.g.party.Reserve[29]
			fp := installFakePointer(t)
			fp.moveTo(400, 300)
			oldWheel := pointerWheel
			t.Cleanup(func() { pointerWheel = oldWheel })
			pointerWheel = func() (float64, float64) { return 0, -0.2 }
			for range 40 {
				h.ui.Draw(h.screen)
				h.ui.dispatchDisplayedInput()
			}
			pointerWheel = func() (float64, float64) { return 0, 0 }
			if h.g.rosterScroll == 0 {
				t.Fatal("wheel did not scroll roster")
			}
			h.g.rosterSelectedActive = 0
			h.ui.Draw(h.screen)
			var lastRow layoutRect
			for _, cmd := range h.ui.displayedInput.commands {
				r := cmd.bounds
				if cmd.kind == uiCommandClick && r.h == 30 && r.x >= 400 && r.y > lastRow.y {
					lastRow = r
				}
			}
			if lastRow.w == 0 {
				t.Fatal("last reserve row not displayed")
			}
			h.clicks(false, lastRow.x+3, lastRow.y+3, 1)
			if h.g.party.Members[0] != last {
				t.Fatal("last recruit could not be selected")
			}
		})
	}
}

func TestSniperAutoLevelAndMasteryProgression(t *testing.T) {
	cfg := loadTestConfig(t)
	for _, tc := range []struct {
		name                                                                       string
		points, speed, endurance, accuracy, wantSpeed, wantEndurance, wantAccuracy int
	}{
		{"speed floor", 1, 15, 17, 18, 16, 17, 18},
		{"endurance and accuracy", 2, 16, 17, 18, 16, 18, 19},
		{"accuracy focus", 6, 16, 18, 18, 16, 18, 24},
		{"accuracy cap", 2, 16, 18, 99, 16, 20, 99},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ch := character.CreateCharacter("Mara", character.ClassSniper, cfg)
			ch.Speed, ch.Endurance, ch.Accuracy = tc.speed, tc.endurance, tc.accuracy
			ch.FreeStatPoints = tc.points
			autoDistributeStatPoints(ch, cfg)
			if ch.Speed != tc.wantSpeed || ch.Endurance != tc.wantEndurance || ch.Accuracy != tc.wantAccuracy || ch.FreeStatPoints != 0 {
				t.Fatalf("unexpected auto stats: speed=%d endurance=%d accuracy=%d", ch.Speed, ch.Endurance, ch.Accuracy)
			}
		})
	}
	for _, skill := range []character.SkillType{character.SkillOverwatch, character.SkillFieldMedicine, character.SkillBallistics, character.SkillDesignateTarget} {
		ch := character.CreateCharacter("Mara", character.ClassSniper, cfg)
		// Leave only this upgrade so the ordinary level-choice pad must offer it.
		for key, s := range ch.Skills {
			if key != skill {
				s.Mastery = character.MasteryGrandMaster
			}
		}
		for tier := character.MasteryNovice; tier < character.MasteryGrandMaster; tier++ {
			options := padLevelUpOptions(ch, nil)
			if len(options) != 1 || options[0].skillType != skill {
				t.Fatalf("%s missing from regular progression", skill)
			}
			if !upgradeSkillMastery(ch, skill) || ch.Skills[skill].Mastery != tier+1 {
				t.Fatal("mastery upgrade failed")
			}
		}
	}
}

func TestPartyCreateScrollingKeepsMaraSelectableAndDraggable(t *testing.T) {
	for _, wheel := range []bool{false, true} {
		t.Run(fmt.Sprint(wheel), func(t *testing.T) {
			h := newDisplayedModalHarness(t, 800, 600)
			h.g.appScreen = AppScreenPartyCreate
			h.g.partyCreate = newPartyCreateState(h.g.config)
			pc := h.g.partyCreate
			fp := installFakePointer(t)
			step := func() {
				h.loop.Draw(h.screen)
				if err := h.loop.Update(); err != nil {
					t.Fatal(err)
				}
			}
			oldWheel := pointerWheel
			defer func() { pointerWheel = oldWheel }()
			for i := 0; i < 10; i++ {
				lay := partyCreateLayout(pc, 800, 600)
				if wheel {
					fp.moveTo(lay.poolArea.x+5, lay.poolArea.y+5)
					pointerWheel = func() (float64, float64) { return 0, -0.2 }
					step()
				} else {
					fp.moveTo(lay.poolDown.x+4, lay.poolDown.y+4)
					fp.press()
					step()
					fp.release()
					step()
					fp.idle()
				}
			}
			pointerWheel = func() (float64, float64) { return 0, 0 }
			lay := partyCreateLayout(pc, 800, 600)
			i := len(pc.pool) - 1
			r := lay.pool[i]
			if r.w == 0 || pc.pool[i].char.Name != "Mara" {
				t.Fatal("last hero not visible")
			}
			fp.moveTo(r.x+r.w/2, r.y+r.h/2)
			fp.press()
			step()
			if pc.detail.char.Name != "Mara" {
				t.Fatal("scrolled hero cannot be selected")
			}
			slot := lay.slots[0]
			fp.hold()
			fp.moveTo(slot.x+slot.w/2, slot.y+slot.h/2)
			step()
			fp.release()
			step()
			if pc.slots[0].char.Name != "Mara" {
				t.Fatal("scrolled hero cannot be dragged into party")
			}
		})
	}
}
