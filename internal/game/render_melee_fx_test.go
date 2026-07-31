package game

import (
	"math"
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

// The endgame sets ship complete: every Drakeforged and Pursuer weapon authors
// its own flourish, melee AND ranged (bows, blasters, the staff). A stock
// category swing or a bare school orb on one of these reads as unfinished
// content next to its neighbours, so the whole roster is pinned by key.
func TestEndgameWeaponSetsAuthorBespokeFx(t *testing.T) {
	if _, err := config.LoadWeaponConfig("../../assets/weapons.yaml"); err != nil {
		t.Fatalf("load weapons: %v", err)
	}
	validateWeaponFxStyles()

	melee := map[string]string{
		"drakefang_blade":     "dragon_fang",
		"wyrmcleaver":         "dragon_jaws",
		"ember_egg_mace":      "dragon_ember_egg",
		"broodspike":          "dragon_broodspike",
		"tarn_trident":        "dragon_tarn",
		"hatchling_fang":      "dragon_hatchling",
		"scalebreaker_maul":   "dragon_roar",
		"verdant_eye_scepter": "dragon_eye",
		"vibro_blade":         "tech_vibro",
	}
	ranged := map[string]string{
		"wyrmspine_bow":   "dragon_wing",
		"nest_arbalest":   "dragon_nest",
		"suppressor_gun":  "tech_suppressor",
		"longlance_rifle": "tech_longlance",
		"compound_bow":    "tech_compound_bow",
	}

	for key, want := range melee {
		def := config.GlobalWeapons.Weapons[key]
		if def == nil || def.Graphics == nil {
			t.Errorf("weapon %q missing or has no graphics block", key)
			continue
		}
		if def.Graphics.SlashFx != want {
			t.Errorf("weapon %q slash_fx = %q, want %q", key, def.Graphics.SlashFx, want)
		}
		if _, ok := meleeFxStyleDraw[want]; !ok {
			t.Errorf("style %q has no renderer", want)
		}
	}
	for key, want := range ranged {
		def := config.GlobalWeapons.Weapons[key]
		if def == nil || def.Graphics == nil {
			t.Errorf("weapon %q missing or has no graphics block", key)
			continue
		}
		if def.Graphics.ProjectileFx != want {
			t.Errorf("weapon %q projectile_fx = %q, want %q", key, def.Graphics.ProjectileFx, want)
		}
		fx, ok := weaponProjectileFxStyles[want]
		if !ok || fx.side == nil || fx.headOn == nil {
			t.Errorf("style %q has no renderer", want)
		}
	}

	// Every style in both sets must be distinct - a copied registry line would
	// give two weapons the same signature, which is the bug being fixed here.
	seen := map[string]string{}
	for key, style := range melee {
		if prev, dup := seen[style]; dup {
			t.Errorf("style %q shared by %q and %q", style, prev, key)
		}
		seen[style] = key
	}
	for key, style := range ranged {
		if prev, dup := seen[style]; dup {
			t.Errorf("style %q shared by %q and %q", style, prev, key)
		}
		seen[style] = key
	}
}

func TestProjectileFxRegistryDefinesSideAndHeadOnRenderers(t *testing.T) {
	for style, fx := range weaponProjectileFxStyles {
		if fx.side == nil {
			t.Errorf("projectile FX style %q has no side-on renderer", style)
		}
		if fx.headOn == nil {
			t.Errorf("projectile FX style %q has no head-on renderer", style)
		}
	}
}

func TestBlasterProjectileFxUsesHeadOnPathAlongCameraAxis(t *testing.T) {
	r := &Renderer{game: &MMGame{camera: &FirstPersonCamera{Angle: 0}}}

	// Facing east at angle zero makes an eastbound shot head-on. An empty style
	// keeps this routing test independent of Ebiten drawing state.
	if !r.drawBlasterWeaponProjectileFx("", nil, 0, 0, 1, 1, 0, 1, 1) {
		t.Fatal("camera-axis shot did not select head-on projectile FX")
	}
	if r.drawBlasterWeaponProjectileFx("", nil, 0, 0, 1, 0, 1, 1, 1) {
		t.Fatal("lateral shot incorrectly selected head-on projectile FX")
	}
}

func TestProjectileProjectionRoutesHeadOnAndBothSideDirections(t *testing.T) {
	r := &Renderer{game: &MMGame{camera: &FirstPersonCamera{Angle: 0}}}

	tests := []struct {
		name      string
		vx, vy    float64
		wantDir   float64
		wantFound bool
	}{
		{name: "head on", vx: 1, vy: 0, wantFound: false},
		{name: "screen right", vx: 0, vy: 1, wantDir: 1, wantFound: true},
		{name: "screen left", vx: 0, vy: -1, wantDir: -1, wantFound: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDir, gotFound := r.projectileScreenDir(tt.vx, tt.vy)
			if gotFound != tt.wantFound || gotDir != tt.wantDir {
				t.Fatalf("projectileScreenDir(%v, %v) = (%v, %v), want (%v, %v)",
					tt.vx, tt.vy, gotDir, gotFound, tt.wantDir, tt.wantFound)
			}
		})
	}
}

func TestProjectileHeadOnDistinguishesIncomingAndOutgoing(t *testing.T) {
	tests := []struct {
		name     string
		angle    float64
		vx, vy   float64
		incoming bool
	}{
		{name: "east outgoing", angle: 0, vx: 1, vy: 0},
		{name: "east incoming", angle: 0, vx: -1, vy: 0, incoming: true},
		{name: "north outgoing", angle: math.Pi / 2, vx: 0, vy: 1},
		{name: "north incoming", angle: math.Pi / 2, vx: 0, vy: -1, incoming: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Renderer{game: &MMGame{camera: &FirstPersonCamera{Angle: tt.angle}}}
			if got := r.projectileMovesTowardCamera(tt.vx, tt.vy); got != tt.incoming {
				t.Fatalf("projectileMovesTowardCamera(%v, %v) = %v, want %v",
					tt.vx, tt.vy, got, tt.incoming)
			}
		})
	}
}

func TestBowHandConvergenceEndsAfterThreeTiles(t *testing.T) {
	const tileSize = 64.0
	tests := []struct {
		distance float64
		want     float64
	}{
		{distance: 0, want: 1},
		{distance: tileSize, want: 20.0 / 27.0},
		{distance: 1.5 * tileSize, want: 0.5},
		{distance: 2 * tileSize, want: 7.0 / 27.0},
		{distance: 3 * tileSize, want: 0},
		{distance: 4 * tileSize, want: 0},
	}
	for _, tt := range tests {
		if got := bowHandConvergence(tt.distance, tileSize); math.Abs(got-tt.want) > 0.0001 {
			t.Errorf("bowHandConvergence(%v, %v) = %v, want %v",
				tt.distance, tileSize, got, tt.want)
		}
	}
}

func TestArrowWrapperAccumulatesPathIndependentOfPosition(t *testing.T) {
	arrow := &Arrow{X: 100, Y: 200}
	wrapper := &ArrowWrapper{Arrow: arrow}

	wrapper.SetPosition(103, 204)
	wrapper.SetPosition(103, 216)

	const want = 17.0
	if math.Abs(arrow.DistanceTraveled-want) > 0.0001 {
		t.Fatalf("arrow path = %v, want %v", arrow.DistanceTraveled, want)
	}
}

func TestArrowFallbackAngleMirrorsLateralDirection(t *testing.T) {
	right := arrowFallbackScreenAngle(1)
	left := arrowFallbackScreenAngle(-1)
	if math.Cos(right) <= 0 {
		t.Fatalf("right-moving fallback angle %v points left", right)
	}
	if math.Cos(left) >= 0 {
		t.Fatalf("left-moving fallback angle %v points right", left)
	}
	if math.Abs(math.Sin(right)-math.Sin(left)) > 0.0001 {
		t.Fatalf("fallback pitches differ: right=%v left=%v", right, left)
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
