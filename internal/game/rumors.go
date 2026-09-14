package game

import (
	"fmt"
	"hash/fnv"
	"os"
	"strings"

	"ugataima/internal/character"
	"ugataima/internal/quests"

	"gopkg.in/yaml.v3"
)

// Tavern rumors are objective story guide rails. Every tavern-capable NPC
// offers a "Listen for rumors" branch showing one actionable rumor. General
// quest leads share the daily rotation; unlocked boss follow-ups temporarily
// take over the pool until their named monster objective is complete.

// RumorDef is one authored rumor (assets/rumors.yaml).
type RumorDef struct {
	// Quest is the goal this rumor points at: once completed the rumor
	// retires. Empty = pure flavor/aftermath, never retires.
	Quest string `yaml:"quest,omitempty"`
	// After hides the rumor until that quest completes ("" = always eligible).
	After string `yaml:"after,omitempty"`
	// UntilMonster retires the rumor once this unique story target is no longer
	// alive. TargetMap scopes the lookup. Spawn optionally names the stable
	// "quest#spawn" event that must fire before absence can mean death.
	UntilMonster string `yaml:"until_monster,omitempty"`
	TargetMap    string `yaml:"target_map,omitempty"`
	Spawn        string `yaml:"spawn,omitempty"`
	// Priority orders simultaneous boss follow-ups. General quest leads remain
	// in the same daily deck regardless of priority so none become unreachable.
	Priority int    `yaml:"priority,omitempty"`
	Text     string `yaml:"text"`
}

type rumorConfig struct {
	Rumors []RumorDef `yaml:"rumors"`
}

var globalRumors []RumorDef

// LoadRumorConfig loads assets/rumors.yaml and validates every quest link
// against the same quest definitions used by gameplay.
func LoadRumorConfig(path string, questManager *quests.QuestManager) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("rumors: %w", err)
	}
	var cfg rumorConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("rumors: %w", err)
	}
	for i, r := range cfg.Rumors {
		if r.Text == "" {
			return fmt.Errorf("rumors: entry %d has no text", i)
		}
		if r.Priority < 0 {
			return fmt.Errorf("rumors: entry %d has negative priority", i)
		}
		if r.Quest == "" && r.UntilMonster == "" {
			return fmt.Errorf("rumors: entry %d has no retirement condition", i)
		}
		if r.UntilMonster != "" && r.After == "" {
			return fmt.Errorf("rumors: entry %d until_monster requires after", i)
		}
		if r.Spawn != "" {
			if r.UntilMonster == "" {
				return fmt.Errorf("rumors: entry %d spawn requires until_monster", i)
			}
			if !validRumorSpawnReference(r.Spawn, questManager) {
				return fmt.Errorf("rumors: entry %d references unknown quest spawn %q", i, r.Spawn)
			}
		}
		for _, ref := range []string{r.Quest, r.After} {
			if ref == "" {
				continue
			}
			if questManager == nil || questManager.Definitions()[ref] == nil {
				return fmt.Errorf("rumors: entry %d references unknown quest %q", i, ref)
			}
		}
	}
	globalRumors = cfg.Rumors
	return nil
}

func validRumorSpawnReference(ref string, questManager *quests.QuestManager) bool {
	questID, spawnID, ok := strings.Cut(ref, "#")
	if !ok || questID == "" || spawnID == "" || questManager == nil {
		return false
	}
	def := questManager.Definitions()[questID]
	if def == nil {
		return false
	}
	for _, spawn := range def.OnCompleteSpawns {
		if spawn.ID == spawnID {
			return true
		}
	}
	return false
}

// questCompleted reports whether a quest is completed in the current run.
func (g *MMGame) questCompleted(id string) bool {
	if g.questManager == nil {
		return false
	}
	q := g.questManager.GetQuest(id)
	return q != nil && q.Completed
}

// rumorSpawnFired recognizes current stable spawn keys plus both legacy save
// forms used before spawn IDs became authoritative.
func (g *MMGame) rumorSpawnFired(ref string) bool {
	if g.questSpawnsDone[ref] {
		return true
	}
	questID, spawnID, ok := strings.Cut(ref, "#")
	if !ok || g.questManager == nil {
		return false
	}
	if g.questSpawnsDone[questID] {
		return true
	}
	def := g.questManager.Definitions()[questID]
	if def == nil {
		return false
	}
	for i, spawn := range def.OnCompleteSpawns {
		if spawn.ID == spawnID {
			return g.questSpawnsDone[fmt.Sprintf("%s#%d", questID, i)]
		}
	}
	return false
}

func (g *MMGame) rumorMonsterObjectiveComplete(r RumorDef) bool {
	if r.UntilMonster == "" {
		return false
	}
	// A deferred boss is absent before its first arrival too. Only treat
	// absence as death after its authored spawn event has actually fired.
	if r.Spawn != "" && !g.rumorSpawnFired(r.Spawn) {
		return false
	}
	def := &quests.QuestDefinition{
		TargetMonster: quests.NormalizeTarget(r.UntilMonster),
		TargetMap:     r.TargetMap,
	}
	for _, pending := range g.pendingQuestSpawns {
		if pending.monster != nil && pending.monster.HitPoints > 0 &&
			def.MatchesTarget(questMonsterTag(pending.monster)) {
			return false
		}
	}
	return g.countLivingQuestTargets(def) == 0
}

func (g *MMGame) rumorAvailable(r RumorDef) bool {
	if r.After != "" && !g.questCompleted(r.After) {
		return false
	}
	if r.Quest != "" && g.questCompleted(r.Quest) {
		return false
	}
	return !g.rumorMonsterObjectiveComplete(r)
}

// eligibleRumors returns every actionable general quest lead unless an
// immediate boss follow-up is active. Boss follow-ups are deliberately
// exclusive: once a quest has spawned or unlocked its named target, that exact
// next step is more useful than another destination. If several such steps are
// active, only the highest-priority tier shares the daily deck.
func (g *MMGame) eligibleRumors() []string {
	var general []string
	var followups []string
	bestFollowupPriority := -1
	for _, r := range globalRumors {
		if !g.rumorAvailable(r) {
			continue
		}
		if r.UntilMonster == "" {
			general = append(general, r.Text)
			continue
		}
		if r.Priority < bestFollowupPriority {
			continue
		}
		if r.Priority > bestFollowupPriority {
			followups = followups[:0]
			bestFollowupPriority = r.Priority
		}
		followups = append(followups, r.Text)
	}
	if len(followups) > 0 {
		return followups
	}
	return general
}

// tavernRumorSeed identifies the tavern doing the talking - its OWN region plus
// its tile, so each taproom rolls its own deck and two taverns in one region
// still differ. Derived, never saved.
func tavernRumorSeed(npc *character.NPC, mapKey string) uint64 {
	h := fnv.New64a()
	fmt.Fprintf(h, "%s|%s|%d,%d", mapKey, npc.Key, int(npc.X), int(npc.Y))
	return h.Sum64()
}

// tavernRegionKey is the region the TAVERN stands in, not the party's. A
// corridor lands tight at some taverns, so the party can be inside the
// neighbouring region while talking - keying on CurrentMapKey there would hand
// one taproom two different decks.
func (g *MMGame) tavernRegionKey(npc *character.NPC) string {
	ts := float64(g.config.GetTileSize())
	if ts <= 0 {
		return currentMapKey()
	}
	return g.mapKeyAtTile(TileIndex(npc.X, ts), TileIndex(npc.Y, ts))
}

// rumorOrder is one tavern's private shuffle of the pool - its deck. Walking it
// by day visits every rumor once before any repeats.
func rumorOrder(n int, seed uint64) []int {
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	s := seed
	for i := n - 1; i > 0; i-- {
		s += 0x9e3779b97f4a7c15 // splitmix64
		x := s
		x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
		x = (x ^ (x >> 27)) * 0x94d049bb133111eb
		x ^= x >> 31
		j := int(x % uint64(i+1))
		order[i], order[j] = order[j], order[i]
	}
	return order
}

// currentRumorText picks this tavern's rumor for today. One shared pool, but
// each taproom rolls it independently - its own deck order, walked by the
// day/night clock. Two taverns landing on one hint is fine; they part ways the
// next flip.
func (g *MMGame) currentRumorText(seed uint64) string {
	pool := g.eligibleRumors()
	if len(pool) == 0 {
		return "The taproom is quiet tonight. Even the liars have nothing."
	}
	day := g.dayNightDay
	if day < 0 {
		day = 0
	}
	order := rumorOrder(len(pool), seed)
	return pool[order[day%len(order)]]
}
