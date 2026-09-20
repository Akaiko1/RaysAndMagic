package game

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/quests"
	"ugataima/internal/world"

	"gopkg.in/yaml.v3"
)

type contentGuideExample struct {
	guide, body string
	index       int
}

// Read the published examples themselves, not a second copy in test fixtures.
// Strict decoding also catches obsolete/unknown fields that runtime YAML
// decoding can ignore. Production loaders still own semantic validation.
func TestContentGuideExamples(t *testing.T) {
	guides := []string{
		"how_to_add_a_new_weapon.md",
		"how_to_add_a_new_spell.md",
		"how_to_add_a_new_monster.md",
		"how_to_add_a_new_npc.md",
		"how_to_add_a_new_tile.md",
		"docs/adding-items-and-loot.md",
		"docs/adding-quests.md",
		"docs/adding-maps.md",
	}
	var examples []contentGuideExample
	for _, guide := range guides {
		raw, err := os.ReadFile(filepath.Join("../..", guide))
		if err != nil {
			t.Fatal(err)
		}
		chunks := strings.Split(string(raw), "```yaml\n")[1:]
		if len(chunks) == 0 {
			t.Fatalf("%s lost its YAML examples", guide)
		}
		for i, chunk := range chunks {
			body, _, ok := strings.Cut(chunk, "```")
			if !ok || strings.TrimSpace(body) == "" {
				t.Fatalf("%s example %d has an empty or unterminated fence", guide, i+1)
			}
			examples = append(examples, contentGuideExample{guide, body, i + 1})
		}
	}

	previousNPCs, previousLoots := character.NPCConfigInstance, config.GlobalLoots
	previousTiles, previousWorld := world.GlobalTileManager, world.GlobalWorldManager
	t.Cleanup(func() {
		// Catalog loaders also rebuild private display-name indexes.
		loadTestConfig(t)
		character.NPCConfigInstance, config.GlobalLoots = previousNPCs, previousLoots
		world.GlobalTileManager, world.GlobalWorldManager = previousTiles, previousWorld
	})
	world.GlobalTileManager, world.GlobalWorldManager = nil, nil

	questCatalog, err := quests.LoadQuestConfig("../../assets/quests.yaml")
	if err != nil {
		t.Fatal(err)
	}
	// The quest guide installs its NPC and objective together. Include its
	// documented definitions when boot validates the NPC's dialogue links.
	for _, example := range examples {
		var root map[string]yaml.Node
		if err := yaml.Unmarshal([]byte(example.body), &root); err != nil {
			t.Fatal(err)
		}
		if _, ok := root["quests"]; ok {
			var definitions quests.QuestConfig
			strictGuideYAML(t, example.body, &definitions)
			for key, def := range definitions.Quests {
				questCatalog.Quests[key] = def
			}
		}
	}

	for _, example := range examples {
		t.Run(fmt.Sprintf("%s/%d", example.guide, example.index), func(t *testing.T) {
			cfg := loadTestConfig(t)
			if _, err := config.LoadLootTables("../../assets/loots.yaml"); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "example.yaml")
			if err := os.WriteFile(path, []byte(example.body), 0o600); err != nil {
				t.Fatal(err)
			}
			var root map[string]yaml.Node
			if err := yaml.Unmarshal([]byte(example.body), &root); err != nil {
				t.Fatal(err)
			}
			var schema any
			var load func() error
			switch {
			case hasGuideRoot(root, "weapons"):
				schema = &config.WeaponSystemConfig{}
				load = func() error {
					mergeGuideCatalog(t, path, "../../assets/weapons.yaml")
					_, err := config.LoadWeaponConfig(path)
					return err
				}
			case hasGuideRoot(root, "spells"):
				schema = &config.SpellSystemConfig{}
				load = func() error { _, err := config.LoadSpellConfig(path); return err }
			case hasGuideRoot(root, "items"):
				schema = &config.ItemSystemConfig{}
				load = func() error {
					mergeGuideCatalog(t, path, "../../assets/items.yaml")
					_, err := config.LoadItemConfig(path)
					return err
				}
			case hasGuideRoot(root, "loots"):
				schema = &config.LootTablesConfig{}
				load = func() error { _, err := config.LoadLootTables(path); return err }
			case hasGuideRoot(root, "monsters"):
				schema = &monster.MonsterYAMLConfig{}
				load = func() error { _, err := monster.LoadMonsterConfig(path); return err }
			case hasGuideRoot(root, "tiles"):
				schema = &config.TileConfig{}
				load = func() error {
					return world.NewTileManager(cfg.Graphics.SizeClasses).LoadTileConfig(path)
				}
			case hasGuideRoot(root, "special_tiles"):
				schema = &config.SpecialTileConfig{}
				load = func() error {
					return world.NewTileManager(cfg.Graphics.SizeClasses).LoadSpecialTileConfig(path)
				}
			case hasGuideRoot(root, "npcs"):
				schema = &character.NPCConfig{}
				load = func() error {
					if err := character.LoadNPCConfig(path); err != nil {
						return err
					}
					npcs := character.NPCConfigInstance.NPCs
					if err := ValidateNPCRenderCategories(npcs); err != nil {
						return err
					}
					if err := ValidateNPCVisualSizes(npcs, cfg.Graphics.SizeClasses); err != nil {
						return err
					}
					if err := ValidateNPCCommerce(npcs); err != nil {
						return err
					}
					if err := ValidateDoorNPCs(npcs); err != nil {
						return err
					}
					return (&MMGame{config: cfg}).validateQuestWorldReferences(quests.NewQuestManager(questCatalog))
				}
			case hasGuideRoot(root, "quests"):
				schema = &quests.QuestConfig{}
				load = func() error {
					_, err := quests.LoadQuestConfig(path)
					if err != nil {
						return err
					}
					character.NPCConfigInstance = nil
					return (&MMGame{config: cfg}).validateQuestWorldReferences(quests.NewQuestManager(questCatalog))
				}
			case hasGuideRoot(root, "maps"):
				schema = &config.MapConfigs{}
				load = func() error { return world.NewWorldManager(cfg).LoadMapConfigs(path) }
			default:
				t.Fatal("example has no supported catalog root; add its production loader to this test")
			}
			strictGuideYAML(t, example.body, schema)
			if err := load(); err != nil {
				t.Fatalf("production validation: %v", err)
			}
		})
	}
}

// Item/weapon sets can reference both catalogs. Integrate example additions
// as the guide instructs, retaining the shipped pieces and set definitions.
func mergeGuideCatalog(t *testing.T, examplePath, catalogPath string) {
	t.Helper()
	read := func(path string) map[string]map[string]yaml.Node {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var catalog map[string]map[string]yaml.Node
		if err := yaml.Unmarshal(raw, &catalog); err != nil {
			t.Fatal(err)
		}
		return catalog
	}
	base, additions := read(catalogPath), read(examplePath)
	for section, entries := range additions {
		if base[section] == nil {
			base[section] = make(map[string]yaml.Node)
		}
		for key, entry := range entries {
			base[section][key] = entry
		}
	}
	raw, err := yaml.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(examplePath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func hasGuideRoot(root map[string]yaml.Node, key string) bool {
	_, ok := root[key]
	return ok
}

func strictGuideYAML(t *testing.T, body string, target any) {
	t.Helper()
	decoder := yaml.NewDecoder(strings.NewReader(body))
	decoder.KnownFields(true)
	if err := decoder.Decode(target); err != nil {
		t.Fatalf("strict schema validation: %v", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("expected one YAML document, got trailing document/error: %v", err)
	}
}
