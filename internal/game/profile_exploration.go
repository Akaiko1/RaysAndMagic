package game

import (
	"sort"

	"ugataima/internal/playerprofile"
	"ugataima/internal/world"
)

// This records occupied tiles only. Visibility, tile art, walkability buffs and
// map reveals never contribute to exploration. Seamless coordinates use the
// same inverse transform as saves, so moving/rotating a region keeps its history.
func (g *MMGame) recordProfileExploration(x, y float64) {
	wm := world.GlobalWorldManager
	if g.playerProfile == nil || wm == nil || g.world == nil || x < 0 || y < 0 {
		return
	}
	ts := g.config.GetTileSize()
	tx, ty := TileIndex(x, ts), TileIndex(y, ts)
	key, w, h := wm.CurrentMapKey, g.world.Width, g.world.Height
	if g.openWorldActive() {
		r := wm.OpenWorldRegionAtTile(tx, ty)
		if r == nil {
			return
		}
		key, w, h = r.MapKey, r.LocalWidth, r.LocalHeight
		tx, ty = wm.LocalizeTile(key, tx, ty)
	}
	// Connecting corridors outside authored region bounds have no denominator
	// cell; do not invent a tile by clamping them onto an interior coordinate.
	if tx >= 0 && ty >= 0 && tx < w && ty < h {
		g.playerProfile.Data.VisitTile(key, tx, ty)
	}
}

func profileRegionPresentation(key string) (name, icon string) {
	name, icon = key, "icon_achievement_first_steps"
	if wm := world.GlobalWorldManager; wm != nil {
		if mc := wm.MapConfigs[key]; mc != nil {
			name = mc.Name
			if mc.SkyTexture != "" {
				icon = "sky:" + mc.SkyTexture
			}
		}
	}
	return
}

// Built once per statistics visit; never scan visited coordinates every Draw.
// Ranking compares exact ratios before formatting tenths of a percent.
func (g *MMGame) profileExplorationRankings() []playerprofile.Entry {
	wm := world.GlobalWorldManager
	if wm == nil || g.playerProfile == nil {
		return nil
	}
	type regionScore struct {
		entry         playerprofile.Entry
		visited, area int64
	}
	var scores []regionScore
	for key := range g.playerProfile.Data.VisitedTiles {
		w, h := 0, 0
		if region := wm.OpenWorldRegionByKey(key); region != nil {
			w, h = region.LocalWidth, region.LocalHeight
		} else if mapped := wm.WorldByKey(key); mapped != nil {
			w, h = mapped.Width, mapped.Height
		}
		area := int64(w) * int64(h)
		visited := g.playerProfile.Data.VisitedTileCount(key, w, h)
		if area <= 0 || visited == 0 {
			continue
		}
		name, icon := profileRegionPresentation(key)
		scores = append(scores, regionScore{playerprofile.Entry{Name: name, Icon: icon, Count: 1000 * visited / area}, visited, area})
	}
	sort.Slice(scores, func(i, j int) bool {
		a, b := scores[i], scores[j]
		if left, right := a.visited*b.area, b.visited*a.area; left != right {
			return left > right
		}
		return a.entry.Name < b.entry.Name
	})
	entries := make([]playerprofile.Entry, len(scores))
	for i, score := range scores {
		entries[i] = score.entry
	}
	return entries
}
