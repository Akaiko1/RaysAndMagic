package game

import (
	"testing"

	"ugataima/internal/config"
)

// Bespoke weapon effects are wired YAML->renderer by name; these tests pin the
// contract so a typo cannot silently fall back to a stock effect.
func TestSlashFxStylesResolve(t *testing.T) {
	if _, err := config.LoadWeaponConfig("../../assets/weapons.yaml"); err != nil {
		t.Fatalf("load weapons: %v", err)
	}

	validateWeaponFxStyles() // must not panic on shipped content

	styled := map[string]string{}
	for key, def := range config.GlobalWeapons.Weapons {
		if def.Graphics != nil && def.Graphics.SlashFx != "" {
			styled[key] = def.Graphics.SlashFx
			if def.Melee == nil {
				t.Errorf("weapon %q has slash_fx but no melee config", key)
			}
		}
	}
	for _, key := range []string{
		"muramasa", "tonbogiri", "kage_kunai", "idol_breakers_maul",
		"silver_sword", "gold_sword", "agility_katar", "gorehorn_greataxe", "serpent_fang", "naginata",
		"gladius", "arena_labrys", "morningstar", "hasta", "trident", "parry_dagger", "lion_warhammer", "bronze_cesti",
	} {
		if styled[key] == "" {
			t.Errorf("weapon %q lost its slash_fx style", key)
		}
	}
}

func TestArenaWeaponProjectileFxStylesResolve(t *testing.T) {
	if _, err := config.LoadWeaponConfig("../../assets/weapons.yaml"); err != nil {
		t.Fatalf("load weapons: %v", err)
	}

	validateWeaponFxStyles() // validates both weapon FX fields

	for _, key := range []string{"arena_shortbow", "arbalest", "lanista_scepter"} {
		def := config.GlobalWeapons.Weapons[key]
		if def.Graphics == nil || def.Graphics.ProjectileFx == "" {
			t.Errorf("weapon %q lost its projectile_fx style", key)
		}
	}
}

func TestProjectileFxStylesResolve(t *testing.T) {
	if _, err := config.LoadSpellConfig("../../assets/spells.yaml"); err != nil {
		t.Fatalf("load spells: %v", err)
	}

	validateProjectileFxStyles() // must not panic on shipped content

	styled := map[string]string{}
	for key, def := range config.GlobalSpells.Spells {
		if def.Graphics != nil && def.Graphics.ProjectileFx != "" {
			styled[key] = def.Graphics.ProjectileFx
			if !def.IsProjectile {
				t.Errorf("spell %q has projectile_fx but is not a projectile", key)
			}
		}
	}
	for _, key := range []string{"fireball", "lightning", "harm", "psychic_shock", "starburst", "disintegrate"} {
		if styled[key] == "" {
			t.Errorf("spell %q lost its projectile_fx style", key)
		}
	}
}

func TestValidateWeaponFxStylesRejectsUnknown(t *testing.T) {
	if _, err := config.LoadWeaponConfig("../../assets/weapons.yaml"); err != nil {
		t.Fatalf("load weapons: %v", err)
	}
	def := config.GlobalWeapons.Weapons["muramasa"]
	orig := def.Graphics.SlashFx
	def.Graphics.SlashFx = "no_such_style"
	defer func() {
		def.Graphics.SlashFx = orig
		if recover() == nil {
			t.Fatal("expected panic on unknown slash_fx style")
		}
	}()
	validateWeaponFxStyles()
}

func TestValidateWeaponFxStylesRejectsUnknownProjectile(t *testing.T) {
	if _, err := config.LoadWeaponConfig("../../assets/weapons.yaml"); err != nil {
		t.Fatalf("load weapons: %v", err)
	}
	def := config.GlobalWeapons.Weapons["arena_shortbow"]
	orig := def.Graphics.ProjectileFx
	def.Graphics.ProjectileFx = "no_such_style"
	defer func() {
		def.Graphics.ProjectileFx = orig
		if recover() == nil {
			t.Fatal("expected panic on unknown weapon projectile_fx style")
		}
	}()
	validateWeaponFxStyles()
}
