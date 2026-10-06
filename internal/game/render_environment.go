package game

// Authored fields use the same depth-tested tile edges as player traps. The
// warning and active phases share geometry with damage and editor validation.
func (r *Renderer) collectEnvironmentEffects() {
	r.collectMonsterTelegraphs()
	g := r.game
	key := currentMapKey()
	a := g.adventureConfig(key)
	v := g.adventure.Visits[key]
	if a == nil || v == nil || g.world == nil {
		return
	}
	alpha, density, radius := r.auraEdgeParams()
	ts := float64(g.config.GetTileSize())
	for _, e := range a.Effects {
		if !g.effectEnabled(a, v, e) {
			continue
		}
		s := v.Effects[e.ID]
		if e.TriggerLane != "" {
			s = v.Effects[e.TriggerLane]
		}
		if e.Kind == "transfer" {
			r.collectAuraTileEdges(e.Destination[0], e.Destination[1], ts, density, alpha*.65, float64(radius)*ts, [3]int{100, 255, 170})
		}
		if e.Kind == "lane" && (s == nil || (s.Warning <= 0 && s.Remaining <= 0)) {
			continue
		}
		if e.Kind == "trap" && s != nil && s.Triggered {
			continue
		}
		rgb := [3]int{224, 176, 65}
		switch e.School {
		case "fire":
			rgb = [3]int{255, 110, 35}
		case "water":
			rgb = [3]int{45, 195, 220}
		case "earth":
			rgb = [3]int{170, 195, 80}
		case "air":
			rgb = [3]int{150, 220, 255}
		}
		if s != nil && s.Warning > 0 {
			rgb = [3]int{255, 245, 120}
		}
		for y := e.Rect[1]; y <= e.Rect[3]; y++ {
			for x := e.Rect[0]; x <= e.Rect[2]; x++ {
				r.collectAuraTileEdges(x, y, ts, density, alpha, float64(radius)*ts, rgb)
			}
		}
	}
}
