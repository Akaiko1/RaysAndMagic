package game

import (
	"maps"
	"math"
	"slices"
	"testing"

	"ugataima/internal/character"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/quests"
)

// The dragon_slayer hunt is sworn, never given at boot. Its targets come only
// from the summons that name the quest: each summoned monster matches the
// quest target and cannot spawn wild, no wild monster or NPC encounter yields
// a target, there are enough summons to finish the hunt, and only a
// quest-flagged target kill credits it.
func TestDragonRoster_BaseWildElitesStatueQuestGating(t *testing.T) {
	const questID = "dragon_slayer"
	cs := newTestCombatSystemWithConfig(t)
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	if err := character.LoadNPCConfig("../../assets/npcs.yaml"); err != nil {
		t.Fatalf("load NPCs: %v", err)
	}
	qc, err := quests.LoadQuestConfig("../../assets/quests.yaml")
	if err != nil {
		t.Fatalf("load quests: %v", err)
	}
	qm := quests.NewQuestManager(qc)
	qm.InitializeStartingQuests()
	cs.game.questManager = qm
	if q := qm.GetQuest(questID); q != nil {
		t.Fatal("dragon_slayer must not be a starting quest - the pilgrim gives it")
	}
	if err := qm.ActivateQuest(questID); err != nil {
		t.Fatalf("activate dragon_slayer: %v", err)
	}
	def := qm.GetQuest(questID).Definition

	monsters := monsterPkg.MonsterConfig.Monsters
	var summoned []string
	for _, key := range slices.Sorted(maps.Keys(character.NPCConfigInstance.NPCs)) {
		npc := character.NPCConfigInstance.NPCs[key]
		for _, s := range npc.Summons {
			if s.QuestID != questID {
				continue
			}
			summoned = append(summoned, s.Monster)
			d, ok := monsters[s.Monster]
			if !ok {
				t.Fatalf("%s summons unknown monster %q", key, s.Monster)
			}
			if !def.MatchesTarget(d.Name) {
				t.Errorf("%s summons %q (%q), which the hunt does not count", key, s.Monster, d.Name)
			}
			if len(d.Biomes) != 0 {
				t.Errorf("summoned target %q must not be wild-spawnable, has biomes %v", s.Monster, d.Biomes)
			}
		}
		if npc.Encounter != nil {
			for _, em := range npc.Encounter.Monsters {
				if d, ok := monsters[em.Type]; ok && def.MatchesTarget(d.Name) {
					t.Errorf("%s encounter spawns hunt target %q outside the summons", key, em.Type)
				}
			}
		}
	}
	if len(summoned) < def.TargetCount {
		t.Fatalf("%d summons name %s, the hunt needs %d kills", len(summoned), questID, def.TargetCount)
	}
	wildKey := ""
	for _, key := range slices.Sorted(maps.Keys(monsters)) {
		if d := monsters[key]; len(d.Biomes) > 0 {
			if def.MatchesTarget(d.Name) {
				t.Errorf("wild monster %q matches the hunt target", key)
			}
			if wildKey == "" {
				wildKey = key
			}
		}
	}

	prog := func() int { return qm.GetQuest(questID).CurrentCount }
	flag := func(m *monsterPkg.Monster3D) {
		m.IsEncounterMonster = true
		m.EncounterRewards = &monsterPkg.EncounterRewards{QuestID: questID}
	}
	// A wild monster never counts, even if (somehow) flagged.
	wild := monsterPkg.NewMonster3DFromConfig(0, 0, wildKey, cs.game.config)
	flag(wild)
	cs.updateQuestProgress(wild)
	if prog() != 0 {
		t.Errorf("wild %s must not credit dragon_slayer, got %d", wildKey, prog())
	}
	// An unflagged target (some future non-summon source) does not count.
	unflagged := monsterPkg.NewMonster3DFromConfig(0, 0, summoned[0], cs.game.config)
	cs.updateQuestProgress(unflagged)
	if prog() != 0 {
		t.Errorf("unflagged %s must not credit, got %d", summoned[0], prog())
	}
	// A summoned (flagged) target counts.
	target := monsterPkg.NewMonster3DFromConfig(0, 0, summoned[len(summoned)-1], cs.game.config)
	flag(target)
	cs.updateQuestProgress(target)
	if prog() != 1 {
		t.Errorf("flagged %s must credit dragon_slayer, got %d", summoned[len(summoned)-1], prog())
	}
}

// A summoned monster can always get back to its statue, where the party stood
// to call it: no tile of the spawn ring may seal it in. An elder dragon left
// in a dune-ringed nook (flight does not clear opaque dunes) was out of the
// party's reach for good, which locked the hunt. Reach uses the monster's own
// movement rule (CanTraverseTile: terrain, body, solid props and NPCs).
func TestStatueSummonsCanReachTheirStatue(t *testing.T) {
	t.Chdir("../..")
	g, _, _ := bootOpenWorldGame(t, true)
	w := g.world
	ih := g.gameLoop.inputHandler
	ts := float64(g.config.GetTileSize())
	statues := 0
	for _, npc := range w.NPCs {
		if npc == nil || len(npc.Summons) == 0 {
			continue
		}
		statues++
		sx, sy := int(npc.X/ts), int(npc.Y/ts)
		ring := map[monsterPkg.TileCoord]bool{}
		for r := encounterSpawnMinTiles; r <= encounterSpawnMaxTiles; r += 0.25 {
			for deg := 0; deg < 360; deg += 2 {
				a := float64(deg) * math.Pi / 180
				x, y := npc.X+math.Cos(a)*r*ts, npc.Y+math.Sin(a)*r*ts
				if ih.isPositionWalkable(x, y) {
					ring[monsterPkg.TileCoord{X: int(x / ts), Y: int(y / ts)}] = true
				}
			}
		}
		if len(ring) == 0 {
			t.Fatalf("%s has no walkable spawn tile", npc.Name)
		}
		for _, s := range npc.Summons {
			m := monsterPkg.NewMonster3DFromConfig(npc.X, npc.Y, s.Monster, g.config)
			g.registerSpawnedMonster(m)
			for start := range ring {
				seen := map[monsterPkg.TileCoord]bool{start: true}
				queue := []monsterPkg.TileCoord{start}
				home := false
				for h := 0; h < len(queue) && !home; h++ {
					c := queue[h]
					home = max(c.X-sx, sx-c.X, c.Y-sy, sy-c.Y) <= 1
					for _, o := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
						n := monsterPkg.TileCoord{X: c.X + o[0], Y: c.Y + o[1]}
						if !seen[n] && m.CanTraverseTile(g.collisionSystem, n) {
							seen[n] = true
							queue = append(queue, n)
						}
					}
				}
				if !home {
					t.Errorf("%s: %s spawned at tile %v cannot reach the statue at (%d,%d)", npc.Name, s.Monster, start, sx, sy)
				}
			}
		}
	}
	if statues == 0 {
		t.Fatal("no summoning statue in the open world")
	}
}
