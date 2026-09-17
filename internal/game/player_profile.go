package game

import (
	"fmt"
	"strings"
	"time"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/playerprofile"
	"ugataima/internal/quests"
	"ugataima/internal/storage"
	"ugataima/internal/world"
)

// LoadPlayerProfile opens the application profile after content boot.
// Simulation fixtures intentionally omit this external persistence boundary.
func (g *MMGame) LoadPlayerProfile() {
	if g.playerProfile != nil {
		return
	}
	g.playerProfileError = ""
	p, err := playerprofile.Open(storage.AppSavePath("player_profile.json"))
	g.playerProfile = p
	if err != nil {
		g.playerProfileError = err.Error()
		fmt.Printf("Warning: player statistics unavailable: %v\n", err)
	}
}

func (g *MMGame) profileAdd(metric string, amount int64) {
	if g.playerProfile == nil {
		return
	}
	g.playerProfile.Data.Add(metric, amount)
	g.evaluateAchievements()
}

func (g *MMGame) evaluateAchievements() {
	if g.playerProfile == nil {
		return
	}
	changed := false
	for _, def := range config.GetAchievements() {
		if g.playerProfile.Data.Unlock(def.Key, def.AnyOf, def.Target, time.Now()) {
			g.pendingAchievements = append(g.pendingAchievements, def)
			changed = true
		}
	}
	if changed {
		g.playerProfile.Checkpoint()
	}
}

// updatePlayerProfile observes durable state but never treats restoration as a
// new kill, spell, loot roll or movement. Old saves can prove roster/promotions.
func (g *MMGame) updatePlayerProfile(now time.Time) {
	elapsed := now.Sub(g.profileLastTick)
	g.profileLastTick = now
	if g.playerProfile == nil {
		return
	}
	if g.appScreen == AppScreenInGame && g.party != nil {
		d := &g.playerProfile.Data
		won := g.gameVictory || g.victoryAcknowledged
		if g.questManager != nil {
			won = won || g.questManager.VictoryCompleted()
		}
		d.ObserveRun(g.playthroughID, won, g.gameOver)
		active, captives, reserve := character.StartingRoster(g.config)
		rosterSize := len(active) + len(captives) + len(reserve)
		if rosterSize > 0 && len(g.party.Captive) == 0 && len(g.party.Members)+len(g.party.Reserve) >= rosterSize {
			if len(captives) > 0 {
				d.ObserveHistorical("captives_freed", int64(len(captives)))
			}
		}
		g.recordProfileActiveHeroes()
		for _, group := range [][]*character.MMCharacter{g.party.Members, g.party.Reserve} {
			for _, c := range group {
				if c == nil {
					continue
				}
				d.Observe("highest_level", int64(c.Level))
				switch c.Promotion {
				case character.PromotionArchmage:
					d.ObserveHistorical("promotion:archmage", 1)
				case character.PromotionLich:
					d.ObserveHistorical("promotion:lich", 1)
				}
			}
		}
		g.evaluateAchievements()
		loading := g.gameLoop != nil && g.gameLoop.loadingBarrier()
		if !loading {
			g.recordProfileExploration(g.camera.X, g.camera.Y)
		}
		if !g.gameOver && !g.gameplayPausedByOverlay() && !loading && elapsed > 0 && elapsed < 250*time.Millisecond {
			activeNS := elapsed.Nanoseconds()
			d.Add("play_ns", activeNS)
			mode := "rt_ns"
			if g.turnBasedMode {
				mode = "tb_ns"
			}
			d.Add(mode, activeNS)
			for _, c := range g.party.Members {
				if c != nil {
					d.Rank("classes", c.GetClassKey(), c.Class.String(), g.portraitSpriteName(c), activeNS)
				}
			}
			if wm := world.GlobalWorldManager; wm != nil {
				key := wm.CurrentMapKey
				name, icon := profileRegionPresentation(key)
				d.Rank("regions", key, name, icon, activeNS)
			}
		}
	}
	if now.Sub(g.profileLastCheckpoint) >= 5*time.Second {
		g.playerProfile.Checkpoint()
		g.profileLastCheckpoint = now
	}
	// Achievement news waits its turn instead of displacing quest news. Once
	// queued, it is protected from overflow; loading uses the same top channel.
	if g.appScreen == AppScreenInGame && len(g.pendingAchievements) > 0 {
		def := g.pendingAchievements[0]
		g.pendingAchievements = g.pendingAchievements[1:]
		g.queueBanner(bannerAchievement, "Achievement unlocked - "+def.Name)
		g.screenBannerQueue[len(g.screenBannerQueue)-1].icon = def.Icon
	}
	if g.gameplayPausedByOverlay() && len(g.screenBannerQueue) > 0 && g.screenBannerQueue[0].kind != bannerAchievement {
		for i, b := range g.screenBannerQueue {
			if b.kind == bannerAchievement {
				copy(g.screenBannerQueue[1:i+1], g.screenBannerQueue[:i])
				g.screenBannerQueue[0] = b
				break
			}
		}
	}

}

// Authored hero names identify roster members (classes are not unique).
// Benched and captive heroes are deliberately absent from this observation.
func (g *MMGame) recordProfileActiveHeroes() {
	d := &g.playerProfile.Data
	if d.AchievementHeroes == nil {
		d.AchievementHeroes = map[string]bool{}
	}
	active, captives, recruits := character.StartingRoster(g.config)
	complete, total := true, 0
	for _, group := range [][]config.RosterEntry{active, captives, recruits} {
		for _, hero := range group {
			total++
			for _, member := range g.party.Members {
				if member != nil && member.Name == hero.Name {
					d.AchievementHeroes[hero.Name] = true
					break
				}
			}
			complete = complete && d.AchievementHeroes[hero.Name]
		}
	}
	if total > 0 && complete {
		d.Observe("all_heroes_played", 1)
	}
}

func (g *MMGame) recordProfileKill(m *monster.Monster3D) {
	if g.playerProfile == nil || m == nil || isPurePartySummon(m) || m.Bound || m.CharmedByParty {
		return
	}
	// This transient guard belongs to the loaded world: a kill replayed after
	// loading is real lifetime activity, but repeated cleanup of one corpse is not.
	if g.profileKilled == nil {
		g.profileKilled = map[string]bool{}
	}
	id := m.ID
	if id == "" {
		id = fmt.Sprintf("%p", m)
	}
	if g.profileKilled[id] {
		return
	}
	g.profileKilled[id] = true
	d := &g.playerProfile.Data
	d.Add("kills", 1)
	d.Add("kills:"+m.Key, 1)
	if m.IsBoss() {
		d.Add("bosses", 1)
		d.Rank("bosses", m.Key, m.Name, "monster:"+m.GetSpriteType(), 1)
	}
	if m.IsChampion() {
		d.Add("champions", 1)
	}
	d.Rank("kills", m.Key, m.Name, "monster:"+m.GetSpriteType(), 1)
	g.evaluateAchievements()
}

func (g *MMGame) recordProfileLoot(loot []items.Item) {
	g.recordProfileLootSource(loot, false)
}

// Both placed chests and encounter chests use the same item-unit accounting.
func (g *MMGame) recordProfileLootSource(loot []items.Item, chest bool) {
	if g.playerProfile == nil {
		return
	}
	for _, it := range loot {
		n := int64(it.Count())
		key, icon := fmt.Sprintf("%d:%s", it.Type, it.Name), itemTooltipIconName(it)
		g.playerProfile.Data.Add("loot", n)
		g.playerProfile.Data.RankValued("loot", key, it.Name, icon, n, int64(it.Attributes["value"]))
		if chest {
			g.playerProfile.Data.Add("chest_loot", n)
			g.playerProfile.Data.Rank("chest_loot", key, it.Name, icon, n)
		}
		group := ""
		if it.Type == items.ItemCard {
			group = "cards_found"
		} else if strings.EqualFold(it.Rarity, "legendary") {
			group = "legendary_loot"
		}
		if group != "" {
			g.playerProfile.Data.Add(group, n)
			g.playerProfile.Data.Rank(group, key, it.Name, icon, n)
		}
	}
}

// Count committed quest resolutions: claims, promotions and automatic rewards.
// Callers own the one-shot transition; restoring resolved quests never calls this.
// Keep the historical total even when older records lack per-quest details.
func (g *MMGame) recordProfileQuestResolution(quest *quests.Quest) {
	if g.playerProfile == nil || quest == nil || quest.Definition == nil || !quest.Completed {
		return
	}
	rewards := quest.Definition.Rewards
	d := &g.playerProfile.Data
	d.Add("quest_rewards", 1)
	d.Rank("quest_rewards", quest.ID, quest.Definition.Name, "icon_achievement_archmage", 1)
	d.Add("quest_gold", int64(rewards.Gold))
	d.Add("quest_xp", int64(rewards.Experience))
	d.Add("quest_arena_points", int64(rewards.ArenaPoints))
	g.evaluateAchievements()
}

// Record the committed payment, not the number of goods received. Shop-wide
// currencies and per-entry overrides both pass the resolved currency here.
func (g *MMGame) recordProfileItemTrade(currency, name string, units int) {
	key, ok := character.CurrencyItemKey(currency)
	if g.playerProfile == nil || !ok || units <= 0 {
		return
	}
	g.playerProfile.Data.Add("items_traded", int64(units))
	g.playerProfile.Data.Rank("items_traded", key, name, "icon_item_"+key, int64(units))
}

func (g *MMGame) recordProfileTravel(origin, destination string) {
	if origin != "" && origin != destination {
		g.profileAdd("departures:"+origin, 1)
	}
}

// Called only on successful RT/TB walking, before any arrival teleporter.
func (g *MMGame) recordProfileStep(oldX, oldY float64) {
	if g.playerProfile == nil {
		return
	}
	ts := g.config.GetTileSize()
	if TileIndex(oldX, ts) != TileIndex(g.camera.X, ts) || TileIndex(oldY, ts) != TileIndex(g.camera.Y, ts) {
		g.profileAdd("steps", 1)
		g.recordProfileExploration(g.camera.X, g.camera.Y)
	}
}

// Damage rankings measure net HP lost to direct attacks (including sacrifice,
// AoE and eradication), after mitigation and survival effects. DoTs/environment
// have no persistent attacker identity and are deliberately not attributed.
func (g *MMGame) beginProfileMonsterHit(m *monster.Monster3D, name string) func() {
	if g.playerProfile == nil || g.party == nil {
		return func() {}
	}
	before := make([]int, len(g.party.Members))
	for i, c := range g.party.Members {
		if c != nil {
			before[i] = max(0, c.HitPoints)
		}
	}
	return func() {
		loss, ko := 0, 0
		for i, c := range g.party.Members {
			if c == nil || i >= len(before) {
				continue
			}
			loss += max(0, before[i]-max(0, c.HitPoints))
			if before[i] > 0 && c.HitPoints <= 0 {
				ko++
			}
		}
		if loss == 0 {
			return
		}
		key, icon := "source:"+strings.ToLower(name), "icon_achievement_first_blood"
		if m != nil {
			key, name, icon = m.Key, m.Name, "monster:"+m.GetSpriteType()
		}
		if name == "" {
			name = "Unknown monster"
		}
		d := &g.playerProfile.Data
		d.Add("monster_damage", int64(loss))
		d.Add("knockouts", int64(ko))
		d.Rank("danger", key, name, icon, int64(loss))
		d.Rank("knockouts", key, name, icon, int64(ko))
	}
}

// Paused achievements use the interface clock; the world clock owns all other
// news. Loading freezes this channel while its own indicator is visible.
func (g *MMGame) tickPausedAchievementBanner() {
	if g.gameLoop != nil && g.gameLoop.loadingBarrier() {
		return
	}
	if b := g.currentScreenBanner(); b != nil && b.kind == bannerAchievement {
		b.frame++
		if b.frame >= g.bannerLifetime(b.kind) {
			g.screenBannerQueue = g.screenBannerQueue[1:]
		}
	}
}
