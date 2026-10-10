package game

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"

	uitext "ugataima/assets/text"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

type guildLedgerEntry struct {
	title, location string
	status          string
	art             guildReportArt
	ready           bool
	details         []string
}

type guildReportArt uint8

const (
	guildReportMerchant guildReportArt = iota
	guildReportTown
	guildReportDungeon
	guildReportCreature
	guildReportWildlife
	guildReportCaravan
)

func guildTimeRemaining(at, now float64) string {
	remaining := at - now
	if remaining <= 0 {
		return uitext.Text("guild.ready")
	}
	if remaining < 1 {
		hours := max(1, int(math.Ceil(remaining*24)))
		if hours == 1 {
			return uitext.Text("guild.hour_left")
		}
		return uitext.Text("guild.hours_left", hours)
	}
	days := int(math.Ceil(remaining))
	if days == 1 {
		return uitext.Text("guild.day_left")
	}
	return uitext.Text("guild.days_left", days)
}

func (g *MMGame) guildNextPhase(night bool) float64 {
	at := float64(g.currentCalendarDay())
	if night {
		at += .5
	}
	if at <= g.currentQuestDay() {
		at++
	}
	return at
}

func (g *MMGame) joinThievesGuild() {
	n := g.dialogNPC
	if n == nil || n.Guild == nil || g.party == nil || g.npcDialogKindFor(n) != dialogKindThievesGuild {
		return
	}
	if g.party.ThievesGuildMember {
		return
	}
	if refusal := g.guildJoinRefusal(n); refusal != "" {
		g.AddCombatMessage(refusal)
		return
	}
	g.party.Gold -= n.Guild.MembershipGold
	g.party.ThievesGuildMember = true
	g.AddCombatMessage(uitext.Text("guild.joined"))
	g.refreshGuildLedger()
}

// guildJoinRefusal is the one admission rule: why the party cannot join this
// guild yet, or "" when it can. The join button and the join itself ask it.
func (g *MMGame) guildJoinRefusal(n *character.NPC) string {
	switch {
	case g.party.NonKeyDoorsOpened < n.Guild.NonKeyDoorsOpened:
		return uitext.Text("guild.no_non_key_door")
	case g.party.Gold < n.Guild.MembershipGold:
		return uitext.Text("guild.no_gold")
	}
	return ""
}

func guildMapName(key string) string {
	if wm := world.GlobalWorldManager; wm != nil {
		if c := wm.MapConfigs[key]; c != nil {
			return c.Name
		}
	}
	return humanizeKey(key)
}

func guildMonsterName(key string) string {
	if monster.MonsterConfig != nil {
		if d, err := monster.MonsterConfig.GetMonsterByKey(key); err == nil {
			return d.Name
		}
	}
	return humanizeKey(key)
}

func (g *MMGame) guildQuestName(id string) string {
	if g.questManager != nil {
		if d := g.questManager.Definitions()[id]; d != nil && d.Name != "" {
			return d.Name
		}
	}
	return humanizeKey(id)
}

// The ledger is a read-only snapshot of the real calendars, never an alternate
// scheduler. Rebuild on opening or explicit refresh; Draw only reads its rows.
func (g *MMGame) refreshGuildLedger() {
	g.guildLedger = [3][]guildLedgerEntry{}
	g.guildScroll = 0
	wm := world.GlobalWorldManager
	if wm == nil {
		return
	}
	now := g.currentQuestDay()
	for _, key := range slices.Sorted(maps.Keys(wm.MapConfigs)) {
		mc := wm.MapConfigs[key]
		w := wm.WorldByKey(key)
		if w != nil && character.NPCConfigInstance != nil {
			for _, npc := range w.NPCs {
				if npc == nil || !g.npcOnMapRegion(npc, key) {
					continue
				}
				d, ok := character.NPCConfigInstance.GetNPCData(npc.Key)
				if !ok || d == nil || (len(npc.MerchantStock) == 0 && d.Type != character.NPCTypeMerchant && d.StockRefreshWeeks == 0) {
					continue
				}
				entry := guildLedgerEntry{title: npc.Name, location: mc.Name, art: guildReportMerchant, status: uitext.Text("guild.no_restock")}
				if d.StockRefreshWeeks > 0 {
					week := max(1, g.calendarWeek)
					nextWeek := nextMerchantRestockWeek(week, d.StockRefreshWeeks)
					day := (nextWeek-1)*g.config.DayNight.DaysPerWeekOrDefault() + 1
					entry.status = guildTimeRemaining(float64(day), now)
					entry.details = []string{uitext.Text("guild.weekly", d.StockRefreshWeeks)}
				}
				g.guildLedger[0] = append(g.guildLedger[0], entry)
			}
		}
		if mc.RespawnDays > 0 {
			entry := guildLedgerEntry{title: mc.Name, art: guildReportTown, status: uitext.Text("guild.visit_first"), details: []string{uitext.Text("guild.unvisited")}}
			if w != nil && w.LastRespawnDay > 0 {
				at := w.LastRespawnDay + mc.RespawnDays
				entry.status = guildTimeRemaining(float64(at), now)
				entry.details = []string{uitext.Text("guild.reset_interval", mc.RespawnDays)}
				if g.currentCalendarDay() >= at {
					entry.status, entry.ready = uitext.Text("guild.ready"), true
					entry.details = []string{uitext.Text("guild.ready_entry")}
				}
			}
			g.guildLedger[1] = append(g.guildLedger[1], entry)
		}
		if a := g.adventureConfig(key); a != nil {
			if a.ResetDays > 0 {
				entry := guildLedgerEntry{title: mc.Name, art: guildReportDungeon, status: uitext.Text("guild.defeat_first"), details: []string{uitext.Text("guild.after_victory", a.ResetDays)}}
				if v := g.adventure.Visits[key]; v != nil && v.BossGranted {
					at := g.adventureResetAt(key, v)
					if now >= at {
						entry.status, entry.ready = uitext.Text("guild.ready"), true
						entry.details = append(entry.details, uitext.Text("guild.ready_entry"))
					} else {
						entry.status = guildTimeRemaining(at, now)
					}
				}
				g.guildLedger[1] = append(g.guildLedger[1], entry)
			}
			if a.Schedule != nil {
				o := g.adventureNextOpening(key)
				entry := guildLedgerEntry{title: mc.Name, art: guildReportDungeon, status: uitext.Text("guild.no_opening"), details: []string{uitext.Text("guild.current_chamber", guildMapName(o.current))}}
				if o.open {
					entry.status, entry.ready = uitext.Text("guild.ready"), true
				} else if o.next != "" {
					entry.status = guildTimeRemaining(o.nextAt, now)
				}
				if o.next != "" {
					entry.details = append(entry.details, uitext.Text("guild.next_chamber", guildMapName(o.next)))
				}
				g.guildLedger[1] = append(g.guildLedger[1], entry)
			}
		}
	}
	for _, pack := range g.config.DayNight.Packs {
		for _, night := range []bool{false, true} {
			phase := uitext.Text("guild.day")
			if night {
				phase = uitext.Text("guild.night")
			}
			at := g.guildNextPhase(night)
			for _, m := range pack.PhaseMembers(night) {
				details := []string{uitext.Text("guild.phase", phase)}
				if pack.RequireMapClear {
					details = append(details, uitext.Text("guild.clear"))
				}
				if m.MinPartyLevel > 0 {
					details = append(details, uitext.Text("guild.level", m.MinPartyLevel))
				}
				if r := m.Replacement; r != nil {
					details = append(details, uitext.Text("guild.replacement", int(math.Round(r.Chance*100)), guildMonsterName(r.Monster), r.MinPartyLevel))
				}
				g.guildLedger[2] = append(g.guildLedger[2], guildLedgerEntry{title: uitext.Text("guild.pack", guildMonsterName(m.Monster), m.Count), location: guildMapName(pack.Map), status: guildTimeRemaining(at, now), art: guildReportCreature, details: details})
			}
		}
	}
	if c := config.GlobalEcology; c != nil {
		for _, p := range c.Populations {
			names := []string{}
			for _, key := range p.Species() {
				names = append(names, guildMonsterName(key))
			}
			phase := uitext.Text("guild.day")
			if p.Phase == "night" {
				phase = uitext.Text("guild.night")
			}
			g.guildLedger[2] = append(g.guildLedger[2], guildLedgerEntry{title: strings.Join(names, ", "), location: guildMapName(p.Map), status: guildTimeRemaining(g.guildNextPhase(p.Phase == "night"), now), art: guildReportWildlife, details: []string{uitext.Text("guild.wildlife", p.Count, phase)}})
		}
		if c.Caravan.Monster != "" {
			status := uitext.Text("guild.travelling")
			details := []string{uitext.Text("guild.caravan_alive")}
			if !g.ecology.Unlocked {
				status = uitext.Text("guild.quest_locked")
				details = []string{uitext.Text("guild.caravan_locked", g.guildQuestName(c.Caravan.UnlockQuest))}
			} else if g.ecology.RespawnDay > 0 {
				status = guildTimeRemaining(float64(g.ecology.RespawnDay), now)
				details = []string{uitext.Text("guild.caravan_return")}
			}
			seen := map[string]bool{}
			for _, r := range c.Caravan.Routes {
				for _, p := range r.Points {
					seen[guildMapName(p.Map)] = true
				}
			}
			details = append(details, uitext.Text("guild.caravan_route", strings.Join(slices.Sorted(maps.Keys(seen)), ", ")))
			g.guildLedger[2] = append(g.guildLedger[2], guildLedgerEntry{title: guildMonsterName(c.Caravan.Monster), status: status, art: guildReportCaravan, details: details})
		}
	}
	for i := range g.guildLedger {
		slices.SortFunc(g.guildLedger[i], func(a, b guildLedgerEntry) int {
			return strings.Compare(a.location+"/"+a.title, b.location+"/"+b.title)
		})
	}
}

// Guild portals are derived from membership and the placed guilds, not from a
// second saved unlock flag. Source coordinates keep IDs stable in stitched and
// split worlds. Normal map/tavern destinations retain their original keys. A
// guild behind an unpaid requires_quest gate offers no entrance either.
func (g *MMGame) guildPortalDestinations() map[string]guildPortalDestination {
	result := map[string]guildPortalDestination{}
	if g.party == nil || !g.party.ThievesGuildMember || world.GlobalWorldManager == nil {
		return result
	}
	wm := world.GlobalWorldManager
	for key := range wm.MapConfigs {
		w := wm.WorldByKey(key)
		if w == nil {
			continue
		}
		for _, n := range w.NPCs {
			if n == nil || n.Guild == nil || !g.npcOnMapRegion(n, key) || g.npcAbsent(n) || !g.npcServiceGateOpen(n) {
				continue
			}
			// Placement identity is local to the canonical source map.
			tx, ty := TileIndex(n.X, g.config.GetTileSize()), TileIndex(n.Y, g.config.GetTileSize())
			tx, ty = wm.LocalizeTile(key, tx, ty)
			id := fmt.Sprintf("guild:%s:%s:%d:%d", key, n.Key, tx, ty)
			result[id] = guildPortalDestination{key, n}
		}
	}
	return result
}

type guildPortalDestination struct {
	mapKey string
	npc    *character.NPC
}

// guildPortalRequest lands the party beside the guild. The arrival passes the
// same check as the transition itself, so a cast is never paid for a jump that
// would then be refused.
func (g *MMGame) guildPortalRequest(dest guildPortalDestination) (mapTransition, error) {
	x, y, ok := nearestWalkableNeighbor(g.worldByKey(dest.mapKey), g.config.GetTileSize(), dest.npc.X, dest.npc.Y)
	if !ok {
		return mapTransition{}, fmt.Errorf("%s", uitext.Text("guild.portal_blocked"))
	}
	tile := [2]int{}
	tile[0], tile[1] = world.GlobalWorldManager.LocalizeTile(dest.mapKey, TileIndex(x, g.config.GetTileSize()), TileIndex(y, g.config.GetTileSize()))
	if g.arrivalTileError(dest.mapKey, tile) != nil {
		return mapTransition{}, fmt.Errorf("%s", uitext.Text("guild.portal_blocked"))
	}
	return mapTransition{mapKey: dest.mapKey, arrival: mapArrivalTownPortal, arrivalTile: &tile}, nil
}
