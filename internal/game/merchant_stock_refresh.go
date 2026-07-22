package game

import (
	"ugataima/internal/character"
	"ugataima/internal/world"
)

// refreshScheduledMerchantStocks refills merchants whose configured cadence
// divides the current calendar week. It runs only when a week boundary occurs.
func (g *MMGame) refreshScheduledMerchantStocks() {
	if g.calendarWeek <= 1 || world.GlobalWorldManager == nil || character.NPCConfigInstance == nil {
		return
	}
	world.GlobalWorldManager.EachWorld(func(_ string, w *world.World3D) {
		for _, npc := range w.NPCs {
			if npc == nil || npc.Key == "" {
				continue
			}
			data, ok := character.NPCConfigInstance.GetNPCData(npc.Key)
			if !ok || data.StockRefreshWeeks <= 0 {
				continue
			}
			if (g.calendarWeek-1)%data.StockRefreshWeeks != 0 {
				continue
			}
			fresh, err := character.CreateNPCFromConfig(npc.Key, npc.X, npc.Y)
			if err == nil {
				npc.MerchantStock = fresh.MerchantStock
			}
		}
	})
}
