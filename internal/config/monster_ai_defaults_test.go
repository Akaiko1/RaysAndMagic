package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// A monster built without a config runs on DefaultMonsterAI, so its
// values must be the shipped monster_ai; a loaded config must carry every AI
// key it covers (one shared check, so one zero key stands for all).
func TestMonsterAIDefaultsMatchShippedConfig(t *testing.T) {
	previous := GlobalConfig
	t.Cleanup(func() { GlobalConfig = previous })
	data, err := os.ReadFile("../../config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig("../../config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	def, shipped := reflect.ValueOf(DefaultMonsterAI()), reflect.ValueOf(cfg.MonsterAI)
	for i := 0; i < def.NumField(); i++ {
		if field := def.Field(i); !field.IsZero() && !reflect.DeepEqual(field.Interface(), shipped.Field(i).Interface()) {
			t.Errorf("DefaultMonsterAI.%s = %v, config.yaml has %v", def.Type().Field(i).Name, field.Interface(), shipped.Field(i).Interface())
		}
	}
	changed := strings.Replace(string(data), "attack_cooldown_seconds: 1", "attack_cooldown_seconds: 0", 1)
	if changed == string(data) {
		t.Fatal("fixture setting absent")
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(changed), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(path); err == nil || !strings.Contains(err.Error(), "monster_ai.attack_cooldown_seconds") {
		t.Fatalf("a zero monster_ai timer: err=%v, want the attack_cooldown_seconds check", err)
	}
}
