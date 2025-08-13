package game

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

// ErrExit is returned from the game loop to request a clean exit
var ErrExit = errors.New("exit game")

// DefaultSavePath is the default file used for saving/loading
const DefaultSavePath = "savegame.json"

// slotPath returns a filename for a numbered save slot (0-based index)
func slotPath(slot int) string { return fmt.Sprintf("save%d.json", slot+1) }

// mainMenuOptions defines the visible options in the ESC menu
var mainMenuOptions = []string{"Continue", "Save", "Load", "Exit"}

// GameSave captures minimal persistent state for save/load
type GameSave struct {
	MapKey      string        `json:"map_key"`
	PlayerX     float64       `json:"player_x"`
	PlayerY     float64       `json:"player_y"`
	PlayerAngle float64       `json:"player_angle"`
	TurnBased   bool          `json:"turn_based"`
	SavedAt     string        `json:"saved_at"`
	Party       PartySave     `json:"party"`
	Monsters    []MonsterSave `json:"monsters"`
	NPCStates   []NPCSave     `json:"npc_states"`
}

type PartySave struct {
	Gold      int             `json:"gold"`
	Food      int             `json:"food"`
	Inventory []items.Item    `json:"inventory"`
	Members   []CharacterSave `json:"members"`
}

type CharacterSave struct {
	Name           string             `json:"name"`
	Class          int                `json:"class"`
	Level          int                `json:"level"`
	Experience     int                `json:"experience"`
	HitPoints      int                `json:"hit_points"`
	MaxHitPoints   int                `json:"max_hit_points"`
	SpellPoints    int                `json:"spell_points"`
	MaxSpellPoints int                `json:"max_spell_points"`
	Might          int                `json:"might"`
	Intellect      int                `json:"intellect"`
	Personality    int                `json:"personality"`
	Endurance      int                `json:"endurance"`
	Accuracy       int                `json:"accuracy"`
	Speed          int                `json:"speed"`
	Luck           int                `json:"luck"`
	FreeStatPoints int                `json:"free_stat_points"`
	Conditions     []int              `json:"conditions"`
	Skills         []SkillEntry       `json:"skills"`
	MagicSchools   []MagicSchoolEntry `json:"magic_schools"`
	Equipment      []EquipmentEntry   `json:"equipment"`
}

type SkillEntry struct {
	Type    int `json:"type"`
	Level   int `json:"level"`
	Mastery int `json:"mastery"`
}

type MagicSchoolEntry struct {
	School      int      `json:"school"`
	Level       int      `json:"level"`
	Mastery     int      `json:"mastery"`
	KnownSpells []string `json:"known_spells"`
}

type EquipmentEntry struct {
	Slot int        `json:"slot"`
	Item items.Item `json:"item"`
}

type MonsterSave struct {
	Key       string  `json:"key"`
	Name      string  `json:"name"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	HitPoints int     `json:"hit_points"`
}

// NPCSave tracks persistent NPC flags across maps
type NPCSave struct {
	MapKey  string `json:"map_key"`
	Name    string `json:"name"`
	Visited bool   `json:"visited"`
}

// SaveSummary is lightweight info used for menu display
type SaveSummary struct {
	Exists    bool
	SavedAt   string
	MapKey    string
	TurnBased bool
}

// GetSaveSlotSummary reads minimal info from a save slot for UI display
func GetSaveSlotSummary(slot int) SaveSummary {
	path := slotPath(slot)
	f, err := os.Open(path)
	if err != nil {
		return SaveSummary{Exists: false}
	}
	defer f.Close()
	var s GameSave
	if err := json.NewDecoder(f).Decode(&s); err != nil {
		return SaveSummary{Exists: false}
	}
	return SaveSummary{Exists: true, SavedAt: s.SavedAt, MapKey: s.MapKey, TurnBased: s.TurnBased}
}

// SaveGameToFile writes the current game state to a JSON file
func (g *MMGame) SaveGameToFile(path string) error {
	wm := world.GlobalWorldManager
	if wm == nil {
		return errors.New("world manager not available")
	}
	save := g.buildSave(wm)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(&save)
}

// LoadGameFromFile loads state from a JSON file and applies it
func (g *MMGame) LoadGameFromFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	var save GameSave
	if err := json.NewDecoder(f).Decode(&save); err != nil {
		return err
	}

	wm := world.GlobalWorldManager
	if wm == nil {
		return errors.New("world manager not available")
	}
	return g.applySave(wm, &save)
}

// buildSave gathers game state into a serializable struct
func (g *MMGame) buildSave(wm *world.WorldManager) GameSave {
	// Party
	ps := PartySave{
		Gold:      g.party.Gold,
		Food:      g.party.Food,
		Inventory: g.party.Inventory,
		Members:   make([]CharacterSave, 0, len(g.party.Members)),
	}
	for _, m := range g.party.Members {
		cs := CharacterSave{
			Name:           m.Name,
			Class:          int(m.Class),
			Level:          m.Level,
			Experience:     m.Experience,
			HitPoints:      m.HitPoints,
			MaxHitPoints:   m.MaxHitPoints,
			SpellPoints:    m.SpellPoints,
			MaxSpellPoints: m.MaxSpellPoints,
			Might:          m.Might,
			Intellect:      m.Intellect,
			Personality:    m.Personality,
			Endurance:      m.Endurance,
			Accuracy:       m.Accuracy,
			Speed:          m.Speed,
			Luck:           m.Luck,
			FreeStatPoints: m.FreeStatPoints,
		}
		// Conditions
		if len(m.Conditions) > 0 {
			cs.Conditions = make([]int, len(m.Conditions))
			for i, c := range m.Conditions {
				cs.Conditions[i] = int(c)
			}
		}
		// Skills
		for t, s := range m.Skills {
			cs.Skills = append(cs.Skills, SkillEntry{Type: int(t), Level: s.Level, Mastery: int(s.Mastery)})
		}
		// Magic schools
		for school, ms := range m.MagicSchools {
			entry := MagicSchoolEntry{School: int(school), Level: ms.Level, Mastery: int(ms.Mastery)}
			if len(ms.KnownSpells) > 0 {
				entry.KnownSpells = make([]string, len(ms.KnownSpells))
				for i, sp := range ms.KnownSpells {
					entry.KnownSpells[i] = string(sp)
				}
			}
			cs.MagicSchools = append(cs.MagicSchools, entry)
		}
		// Equipment (convert map to slice)
		for slot, item := range m.Equipment {
			cs.Equipment = append(cs.Equipment, EquipmentEntry{Slot: int(slot), Item: item})
		}
		ps.Members = append(ps.Members, cs)
	}

	// Monsters
	var ms []MonsterSave
	if g.world != nil {
		for _, mon := range g.world.Monsters {
			key := findMonsterKeyByName(mon.Name)
			ms = append(ms, MonsterSave{Key: key, Name: mon.Name, X: mon.X, Y: mon.Y, HitPoints: mon.HitPoints})
		}
	}

	// NPC states across all loaded maps
	var nstates []NPCSave
	if wm != nil {
		for mapKey, w := range wm.LoadedMaps {
			for _, npc := range w.NPCs {
				nstates = append(nstates, NPCSave{MapKey: mapKey, Name: npc.Name, Visited: npc.Visited})
			}
		}
	}

	return GameSave{
		MapKey:      wm.CurrentMapKey,
		PlayerX:     g.camera.X,
		PlayerY:     g.camera.Y,
		PlayerAngle: g.camera.Angle,
		TurnBased:   g.turnBasedMode,
		SavedAt:     time.Now().Format(time.RFC3339),
		Party:       ps,
		Monsters:    ms,
		NPCStates:   nstates,
	}
}

// applySave restores game state from a save struct
func (g *MMGame) applySave(wm *world.WorldManager, save *GameSave) error {
	// Switch map if needed
	if save.MapKey != "" && save.MapKey != wm.CurrentMapKey && wm.IsValidMap(save.MapKey) {
		if err := wm.SwitchToMap(save.MapKey); err != nil {
			return err
		}
	}
	// Update world reference and visuals
	g.world = wm.GetCurrentWorld()
	g.UpdateSkyAndGroundColors()
	g.collisionSystem.UpdateTileChecker(g.world)

	// Restore player
	g.camera.X = save.PlayerX
	g.camera.Y = save.PlayerY
	g.camera.Angle = save.PlayerAngle
	g.collisionSystem.UpdateEntity("player", save.PlayerX, save.PlayerY)

	// Restore party
	g.party = &character.Party{Members: make([]*character.MMCharacter, 0, len(save.Party.Members)), Gold: save.Party.Gold, Food: save.Party.Food, Inventory: save.Party.Inventory}
	for _, cs := range save.Party.Members {
		m := &character.MMCharacter{
			Name:           cs.Name,
			Class:          character.CharacterClass(cs.Class),
			Level:          cs.Level,
			Experience:     cs.Experience,
			HitPoints:      cs.HitPoints,
			MaxHitPoints:   cs.MaxHitPoints,
			SpellPoints:    cs.SpellPoints,
			MaxSpellPoints: cs.MaxSpellPoints,
			Might:          cs.Might,
			Intellect:      cs.Intellect,
			Personality:    cs.Personality,
			Endurance:      cs.Endurance,
			Accuracy:       cs.Accuracy,
			Speed:          cs.Speed,
			Luck:           cs.Luck,
			FreeStatPoints: cs.FreeStatPoints,
			Skills:         make(map[character.SkillType]*character.Skill),
			MagicSchools:   make(map[character.MagicSchool]*character.MagicSkill),
			Equipment:      make(map[items.EquipSlot]items.Item),
		}
		if len(cs.Conditions) > 0 {
			m.Conditions = make([]character.Condition, len(cs.Conditions))
			for i, c := range cs.Conditions {
				m.Conditions[i] = character.Condition(c)
			}
		}
		for _, s := range cs.Skills {
			m.Skills[character.SkillType(s.Type)] = &character.Skill{Level: s.Level, Mastery: character.SkillMastery(s.Mastery)}
		}
		for _, me := range cs.MagicSchools {
			mk := character.MagicSchool(me.School)
			ms := &character.MagicSkill{Level: me.Level, Mastery: character.SkillMastery(me.Mastery)}
			if len(me.KnownSpells) > 0 {
				ms.KnownSpells = make([]spells.SpellID, len(me.KnownSpells))
				for i, s := range me.KnownSpells {
					ms.KnownSpells[i] = spells.SpellID(s)
				}
			}
			m.MagicSchools[mk] = ms
		}
		for _, eq := range cs.Equipment {
			m.Equipment[items.EquipSlot(eq.Slot)] = eq.Item
		}
		g.party.Members = append(g.party.Members, m)
	}

	// Restore monsters
	if g.world != nil {
		g.world.Monsters = make([]*monster.Monster3D, 0, len(save.Monsters))
		for _, ms := range save.Monsters {
			key := ms.Key
			if key == "" {
				key = findMonsterKeyByName(ms.Name)
			}
			if key == "" {
				continue
			}
			m := monster.NewMonster3DFromConfig(ms.X, ms.Y, key, g.config)
			m.HitPoints = ms.HitPoints
			g.world.Monsters = append(g.world.Monsters, m)
		}
		// Re-register with collision system
		g.collisionSystem = collision.NewCollisionSystem(g.world, float64(g.config.World.TileSize))
		g.collisionSystem.RegisterEntity(collision.NewEntity("player", g.camera.X, g.camera.Y, 16, 16, collision.CollisionTypePlayer, false))
		g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
	}

	// Restore NPC visited flags across maps
	if wm != nil {
		for _, ns := range save.NPCStates {
			if w, ok := wm.LoadedMaps[ns.MapKey]; ok {
				for _, npc := range w.NPCs {
					if npc.Name == ns.Name {
						npc.Visited = ns.Visited
					}
				}
			}
		}
	}

	// Restore mode
	g.turnBasedMode = save.TurnBased

	return nil
}

func findMonsterKeyByName(name string) string {
	if monster.MonsterConfig == nil {
		return ""
	}
	for key, def := range monster.MonsterConfig.Monsters {
		if def.Name == name {
			return key
		}
	}
	return ""
}
