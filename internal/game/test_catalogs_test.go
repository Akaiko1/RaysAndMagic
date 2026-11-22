package game

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/monster"
)

// Only immutable SHIPPED fixture inputs are cached. Loader/validation tests
// still call the production loaders directly. Every fixture gets a deep copy;
// the cached prototype is never installed as a mutable process global.
// Items/weapons/level-up data deliberately use their loaders: those publish
// private indexes/state that cannot be restored by assigning a public pointer.
var (
	testConfigCatalog    = cachedTestCatalog(config.LoadConfig)
	testSpellCatalog     = cachedTestCatalog(config.LoadSpellConfig)
	testTrapCatalog      = cachedTestCatalog(config.LoadTrapConfig)
	testMonsterCatalog   = cachedTestCatalog(monster.LoadMonsterConfig)
	testAlchemyCatalog   = cachedTestCatalog(config.LoadAlchemyConfig)
	testTechniqueCatalog = cachedTestCatalog(func(path string) (*config.TechniqueConfig, error) {
		err := config.LoadTechniques(path)
		return config.GlobalTechniques, err
	})
)

func cachedTestCatalog[T any](load func(string) (*T, error)) func(testing.TB, string) *T {
	var mu sync.Mutex
	prototypes := make(map[string]*T)
	return func(t testing.TB, path string) *T {
		t.Helper()
		absolute, err := filepath.Abs(path)
		if err != nil {
			t.Fatal(err)
		}
		mu.Lock()
		defer mu.Unlock()
		prototype := prototypes[absolute]
		if prototype == nil {
			loaded, err := load(absolute)
			if err != nil {
				t.Fatalf("load fixture %s: %v", path, err)
			}
			// The loader publishes loaded globally; do not retain that object.
			prototype = cloneTestCatalog(reflect.ValueOf(loaded)).Interface().(*T)
			prototypes[absolute] = prototype
		}
		return cloneTestCatalog(reflect.ValueOf(prototype)).Interface().(*T)
	}
}

// Catalogs are acyclic plain data, not live games or resource managers. Reject
// private/runtime state instead of silently sharing it or losing it in JSON.
func cloneTestCatalog(v reflect.Value) reflect.Value {
	switch v.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
	}
	switch v.Kind() {
	case reflect.Pointer:
		out := reflect.New(v.Type().Elem())
		out.Elem().Set(cloneTestCatalog(v.Elem()))
		return out
	case reflect.Map:
		out := reflect.MakeMapWithSize(v.Type(), v.Len())
		iter := v.MapRange()
		for iter.Next() {
			out.SetMapIndex(cloneTestCatalog(iter.Key()), cloneTestCatalog(iter.Value()))
		}
		return out
	case reflect.Slice, reflect.Array:
		out := reflect.New(v.Type()).Elem()
		if v.Kind() == reflect.Slice {
			out = reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		}
		for i := 0; i < v.Len(); i++ {
			out.Index(i).Set(cloneTestCatalog(v.Index(i)))
		}
		return out
	case reflect.Struct:
		out := reflect.New(v.Type()).Elem()
		for i := 0; i < v.NumField(); i++ {
			if !v.Type().Field(i).IsExported() {
				panic(fmt.Sprintf("test catalog %s has private field %s", v.Type(), v.Type().Field(i).Name))
			}
			out.Field(i).Set(cloneTestCatalog(v.Field(i)))
		}
		return out
	case reflect.Bool, reflect.String, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64:
		return v
	default:
		panic(fmt.Sprintf("unsupported test catalog field %s", v.Type()))
	}
}

// Case table: cold/warm fixtures, pointer/map/slice/array edits and added map
// entries. Drive the shared fixture entry point so this catches aliasing in
// the installed globals, not just in a copy utility. Save/load is unchanged:
// only test setup is cached, never a live game or a save-derived value.
func TestCatalogFixturesAreIsolated(t *testing.T) {
	loadTestConfig(t)
	// Freeze the oracle independently of the copy implementation: keeping
	// pointers here would let a shared-map bug corrupt both got and want.
	snapshot := func() []byte {
		t.Helper()
		data, err := json.Marshal([]any{config.GlobalConfig, config.GlobalSpells,
			monster.MonsterConfig, config.GlobalTrapConfig, config.GlobalTechniques, config.GlobalAlchemy})
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	want := snapshot()
	for round := range 2 {
		t.Run(fmt.Sprint(round), func(t *testing.T) {
			cfg := loadTestConfig(t)
			if !bytes.Equal(snapshot(), want) {
				t.Fatal("catalogs inherited another fixture's edits")
			}
			if cfg != config.GlobalConfig {
				t.Fatal("fixture and global config disagree")
			}
			cfg.Graphics.SizeClasses[config.SizeClassPerson]++
			cfg.Display.Fonts[0].Label = "fixture edit"
			config.GlobalSpells.Spells["fire_aura"].Mechanic.PowerByMastery[0]++
			delete(config.GlobalSpells.Spells, "fireball")
			monster.MonsterConfig.Monsters["fixture_only"] = monster.MonsterDefinition{}
			for _, trap := range config.GlobalTrapConfig.Traps {
				trap.BorderColor[0]++
			}
			config.GlobalTechniques.Techniques[0].SPCost[0]++
			config.GlobalAlchemy.Recipes[0].Ingredients[0].Alternatives[0].Items[0] = "fixture edit"
		})
	}
	loadTestConfig(t) // Do not leave the last deliberately corrupted fixture live.
}
