package game

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/world"
)

func TestGuildDoorQualification(t *testing.T) {
	for _, tc := range []struct {
		name, item, stat string
		kind             character.DoorOpeningMethod
		roll, want       int
	}{
		{"picked", "", "", character.DoorOpeningLockpick, 0, 1},
		{"pick failed", "", "", character.DoorOpeningLockpick, 99, 0},
		{"forced might", "", "Might", character.DoorOpeningForce, 0, 1},
		{"forced intellect", "", "Intellect", character.DoorOpeningForce, 0, 1},
		{"force failed", "", "Might", character.DoorOpeningForce, 99, 0},
		{"key", "ordinary_key", "", character.DoorOpeningKey, 0, 0},
		{"master key", "skeleton_key", "", character.DoorOpeningMasterKey, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			door := woodenDoorNPC()
			g := makeDoorGame(t, door)
			class := character.ClassKnight
			if tc.kind == character.DoorOpeningLockpick {
				class = character.ClassThief
			}
			g.party.Members = []*character.MMCharacter{character.CreateCharacter("Opener", class, g.config)}
			if tc.stat != "" {
				door.DoorStatReqs = []character.NPCDoorStatReq{{Stat: tc.stat, Value: 1}}
			}
			if tc.item != "" {
				g.party.AddItem(items.CreateItemFromYAML(tc.item))
			}
			idx := slices.IndexFunc(g.availableDoorUnlocks(door), func(o doorUnlockOption) bool { return o.kind == tc.kind })
			if idx < 0 {
				t.Fatal("missing unlock option")
			}
			g.resolveDoorUnlock(door, idx, tc.roll)
			if g.party.NonKeyDoorsOpened != tc.want {
				t.Fatalf("qualified doors=%d, want %d", g.party.NonKeyDoorsOpened, tc.want)
			}
			wantMethod := tc.kind
			if tc.roll == 99 {
				wantMethod = ""
			}
			if door.DoorOpenedBy != wantMethod {
				t.Fatalf("door opening method=%q, want %q", door.DoorOpenedBy, wantMethod)
			}
			wm := world.NewWorldManager(g.config)
			wm.CurrentMapKey = "forest"
			wm.LoadedMaps = map[string]*world.World3D{"forest": g.world}
			saved := g.buildSave(wm)
			data, err := json.Marshal(saved)
			if err != nil {
				t.Fatal(err)
			}
			var restored GameSave
			if err := json.Unmarshal(data, &restored); err != nil {
				t.Fatal(err)
			}
			door.DoorOpenedBy = ""
			g.party.NonKeyDoorsOpened = 0
			g.restoreSavedParty(&restored)
			g.restoreSavedNPCs(wm, &restored)
			if door.DoorOpenedBy != wantMethod || g.party.NonKeyDoorsOpened != tc.want {
				t.Fatal("save/load changed opening method or guild qualification")
			}
			if door.Visited {
				g.resolveDoorUnlock(door, idx, 0)
				if g.party.NonKeyDoorsOpened != tc.want {
					t.Fatal("open door credited twice")
				}
			}
			// Membership is party history, not the opener's presence or class.
			g.party.Members = []*character.MMCharacter{character.CreateCharacter("Knight", character.ClassKnight, g.config)}
			guild, err := character.CreateNPCFromConfig("thieves_guild", 0, 0)
			if err != nil {
				t.Fatal(err)
			}
			g.beginConversation(guild)
			g.party.Gold = guild.Guild.MembershipGold
			g.joinThievesGuild()
			if g.party.ThievesGuildMember != (tc.want > 0) {
				t.Fatal("guild admission without a Thief disagrees with door history")
			}
		})
	}
}

func TestGuildLegacyDoorHistory(t *testing.T) {
	cfg := loadTestConfig(t)
	for _, tc := range []struct {
		name           string
		opened, locked bool
		method         character.DoorOpeningMethod
		count, want    int
	}{
		{"legacy opened", true, true, "", 0, 1},
		{"legacy closed", false, true, "", 0, 0},
		{"visited prop", true, false, "", 0, 0},
		{"modern key", true, true, character.DoorOpeningKey, 0, 0},
		{"modern master key", true, true, character.DoorOpeningMasterKey, 0, 0},
		{"existing history", true, true, "", 4, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newTestWorldSized(cfg, 12, 12)
			wm := world.NewWorldManager(cfg)
			wm.CurrentMapKey = "forest"
			wm.LoadedMaps = map[string]*world.World3D{"forest": w, "elsewhere": newTestWorldSized(cfg, 12, 12)}
			g := newTestGame(cfg, w)
			save := g.buildSave(wm)
			save.Party.NonKeyDoorsOpened = tc.count
			// History may live on a different map. Several old open doors must
			// grant only one compatibility credit, including after resaving.
			for i := 0; i < 2; i++ {
				door := woodenDoorNPC()
				door.X, door.Y = TileCenterFromTile(4+i, 5, float64(cfg.GetTileSize()))
				if !tc.locked {
					door.Type, door.DoorBehavior = character.NPCTypeEncounter, ""
				}
				wm.LoadedMaps["elsewhere"].NPCs = append(wm.LoadedMaps["elsewhere"].NPCs, door)
				save.NPCStates = append(save.NPCStates, NPCSave{MapKey: "elsewhere", Name: door.Name, X: door.X, Y: door.Y, Visited: tc.opened, DoorOpenedBy: tc.method})
			}
			for round := 0; round < 2; round++ {
				if err := g.applySave(wm, &save); err != nil {
					t.Fatal(err)
				}
				if g.party.NonKeyDoorsOpened != tc.want {
					t.Fatalf("round %d: history=%d, want %d", round, g.party.NonKeyDoorsOpened, tc.want)
				}
				save = g.buildSave(wm)
			}
		})
	}
}

func TestGuildDoorHistoryInMergedRegion(t *testing.T) {
	restoreNPCCatalog(t)
	t.Chdir("../..")
	g, wm, cfg := bootOpenWorldGame(t, true)
	door := woodenDoorNPC()
	x, y := 5.5*cfg.GetTileSize(), 5.5*cfg.GetTileSize()
	door.X, door.Y = wm.ProjectWorldPos("highlands", x, y)
	if door.X == x && door.Y == y {
		t.Fatal("fixture must exercise merged coordinates")
	}
	wm.OpenWorld.NPCs = append(wm.OpenWorld.NPCs, door)
	for _, tc := range []struct {
		name   string
		method character.DoorOpeningMethod
		want   int
	}{
		{"legacy", "", 1},
		{"key", character.DoorOpeningKey, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g.party.NonKeyDoorsOpened = 0
			save := GameSave{NPCStates: []NPCSave{{MapKey: "highlands", Name: door.Name, X: x, Y: y, Visited: true, DoorOpenedBy: tc.method}}}
			g.restoreSavedNPCs(wm, &save)
			if !door.Visited || door.DoorOpenedBy != tc.method || g.party.NonKeyDoorsOpened != tc.want {
				t.Fatal("merged-region door failed to restore its method or history")
			}
			saved := g.buildSave(wm)
			index := slices.IndexFunc(saved.NPCStates, func(n NPCSave) bool { return n.MapKey == "highlands" && n.Name == door.Name && n.X == x && n.Y == y })
			if index < 0 || saved.NPCStates[index].DoorOpenedBy != tc.method {
				t.Fatal("merged snapshot lost localized door history")
			}
		})
	}
}

func TestGuildPortalArrivalAndRejectedEntry(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, state := range []string{"member", "locked", "removed", "blocked", "gated"} {
			t.Run(fmt.Sprintf("tb=%v/%s", tb, state), func(t *testing.T) {
				g, wm, ts := travelFixture(t)
				g.turnBasedMode = tb
				wm.MapConfigs["other"] = &config.MapConfig{Name: "Other"}
				n := &character.NPC{Key: "thieves_guild", Name: "Guild", X: 12.5 * ts, Y: 14.5 * ts, Guild: &character.GuildServiceConfig{MembershipGold: 5000, NonKeyDoorsOpened: 1}}
				w := wm.LoadedMaps["other"]
				w.NPCs = []*character.NPC{n}
				g.party.ThievesGuildMember = true
				rows := g.townPortalRows()
				if len(rows) != 1 {
					t.Fatalf("destinations: %v", rows)
				}
				id := rows[0].id
				switch state {
				case "locked":
					g.party.ThievesGuildMember = false
				case "gated":
					// The entrance is a guild service: an unpaid gate withholds it too.
					n.RequiresQuest = "toll_of_blades"
				case "removed":
					w.NPCs = nil
				case "blocked":
					for y := 13; y <= 15; y++ {
						for x := 11; x <= 13; x++ {
							w.Tiles[y][x] = world.TileWall
						}
					}
				}
				equipSpellAndPrepareCaster(t, g.combat, "town_portal", 100, 20)
				caster := g.party.Members[0]
				caster.ActionsRemaining = 1
				g.currentTurn = 0
				g.townPortalCaster, g.townPortalSpell = caster, "town_portal"
				g.confirmTownPortal(id)
				if state != "member" {
					if wm.CurrentMapKey != "forest" || caster.SpellPoints != 100 || caster.ActionsRemaining != 1 || caster.RTCooldown != 0 {
						t.Fatal("rejected guild portal moved or charged party")
					}
					return
				}
				if wm.CurrentMapKey != "other" || g.camera.X != 12.5*ts || g.camera.Y != 15.5*ts {
					t.Fatal("portal missed guild entrance")
				}
				if caster.SpellPoints >= 100 || (tb && caster.ActionsRemaining != 0) || (!tb && caster.RTCooldown <= 0) {
					t.Fatal("guild portal bypassed cast cost")
				}
				if !slices.ContainsFunc(g.buffFxAnims, func(a buffFxAnim) bool { return a.overlay == "town_portal" }) {
					t.Fatal("confirmed portal has no visual effect")
				}
			})
		}
	}
}

func TestGuildLedgerCountdown(t *testing.T) {
	cfg := loadTestConfig(t)
	w := newTestWorldSized(cfg, 10, 10)
	g := newTestGame(cfg, w)
	wm := world.NewWorldManager(cfg)
	wm.CurrentMapKey = "forest"
	wm.LoadedMaps = map[string]*world.World3D{"forest": w}
	wm.MapConfigs = map[string]*config.MapConfig{"forest": {Name: "Forest", RespawnDays: 3}}
	setTestWorldManager(t, wm)
	for _, tc := range []struct {
		name         string
		visited, day int
		phase        float64
		want         string
		ready        bool
	}{
		{"unvisited", 0, 4, 0, "Visit first", false},
		{"days", 2, 3, 0, "2 days remaining", false},
		{"one day", 2, 4, 0, "1 day remaining", false},
		{"hours", 2, 4, .5, "12 hours remaining", false},
		{"ready", 2, 5, 0, "Ready now", true},
		{"overdue", 2, 8, 0, "Ready now", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w.LastRespawnDay, g.calendarDay = tc.visited, tc.day
			cycle := g.dayNightCycleFrames()
			g.dayNightFrames = (g.dayNightPhaseStartFrame(false) + int(tc.phase*float64(cycle))) % cycle
			g.refreshGuildLedger()
			if len(g.guildLedger[1]) != 1 {
				t.Fatalf("expected one map report: %+v", g.guildLedger[1])
			}
			got := g.guildLedger[1][0]
			if got.status != tc.want || got.ready != tc.ready {
				t.Fatalf("report=%q ready=%t, want %q ready=%t", got.status, got.ready, tc.want, tc.ready)
			}
		})
	}
}

func TestGuildMembershipAndPortalLifecycle(t *testing.T) {
	cfg := loadTestConfig(t)
	restoreNPCCatalog(t)
	if err := character.LoadNPCConfig("../../assets/npcs.yaml"); err != nil {
		t.Fatal(err)
	}
	w := newTestWorldSized(cfg, 10, 10)
	n, err := character.CreateNPCFromConfig("thieves_guild", 4.5*cfg.GetTileSize(), 4.5*cfg.GetTileSize())
	if err != nil {
		t.Fatal(err)
	}
	w.NPCs = append(w.NPCs, n)
	g := newTestGame(cfg, w)
	wm := world.NewWorldManager(cfg)
	wm.CurrentMapKey = "forest"
	wm.LoadedMaps = map[string]*world.World3D{"forest": w}
	wm.MapConfigs = map[string]*config.MapConfig{"forest": {Name: "Forest"}}
	setTestWorldManager(t, wm)
	g.visitedTavernMaps = map[string]bool{"forest": true}
	g.beginConversation(n)
	fee := n.Guild.MembershipGold
	g.party.Gold = fee + 73
	g.joinThievesGuild()
	if g.party.ThievesGuildMember || g.party.Gold != fee+73 || len(g.townPortalRows()) != 1 {
		t.Fatal("membership bypassed non-key door requirement")
	}
	g.party.NonKeyDoorsOpened = n.Guild.NonKeyDoorsOpened
	g.party.Gold = fee - 1
	g.joinThievesGuild()
	if g.party.ThievesGuildMember || g.party.Gold != fee-1 {
		t.Fatal("membership bypassed price")
	}
	g.party.Gold = fee + 73
	g.joinThievesGuild()
	if !g.party.ThievesGuildMember || g.party.Gold != 73 {
		t.Fatal("membership not granted for one fee")
	}
	g.joinThievesGuild()
	if g.party.Gold != 73 {
		t.Fatal("membership charged twice")
	}
	dests := g.townPortalRows()
	ordinary := slices.IndexFunc(dests, func(r townPortalRow) bool { return r.id == "forest" })
	if len(dests) != 2 || ordinary < 0 {
		t.Fatalf("guild replaced ordinary destination: %v", dests)
	}
	guild := dests[1-ordinary]
	if !strings.Contains(guild.label, n.Name) {
		t.Fatal("guild is not named in portal picker")
	}
	save := g.buildSave(wm)
	if len(save.VisitedTavernMaps) != 1 {
		t.Fatal("derived guild destinations leaked into saved map unlocks")
	}
	bytes, err := json.Marshal(save)
	if err != nil {
		t.Fatal(err)
	}
	var restored GameSave
	if err = json.Unmarshal(bytes, &restored); err != nil {
		t.Fatal(err)
	}
	g.restoreSavedParty(&restored)
	if !g.party.ThievesGuildMember || g.party.NonKeyDoorsOpened != n.Guild.NonKeyDoorsOpened || g.party.Gold != 73 || !slices.Contains(g.townPortalRows(), guild) {
		t.Fatal("membership or portal lost on restore")
	}
	g.party = &character.Party{}
	if len(g.guildPortalDestinations()) != 0 {
		t.Fatal("new party inherited membership")
	}
	if w.NPCs[0].Visited {
		t.Fatal("guild was consumed by joining")
	}
}
