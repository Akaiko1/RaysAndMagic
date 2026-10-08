package game

import (
	"github.com/hajimehoshi/ebiten/v2"
	"image/color"
	"math"
	"os"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/monster"
)

// Bespoke FX are wired YAML->renderer by name. Shipped content must pass the
// boot validators (a typo panics there), and only an entry that can show the
// effect may author it.
func TestShippedFxStylesResolve(t *testing.T) {
	if _, err := config.LoadWeaponConfig("../../assets/weapons.yaml"); err != nil {
		t.Fatalf("load weapons: %v", err)
	}
	if _, err := config.LoadSpellConfig("../../assets/spells.yaml"); err != nil {
		t.Fatalf("load spells: %v", err)
	}
	validateWeaponFxStyles()     // must not panic on shipped content
	validateProjectileFxStyles() // must not panic on shipped content

	// The procedural catalog must stay attached to real items, and every
	// shipped melee weapon must retain its own authored body selection.
	t.Run("weapon bodies", func(t *testing.T) {
		for key := range weaponBodyModels {
			if def, ok := config.GlobalWeapons.Weapons[key]; !ok || def == nil || def.Graphics == nil {
				t.Errorf("procedural body %q has no drawable weapon definition", key)
			}
		}
		for key, def := range config.GlobalWeapons.Weapons {
			if def.Melee != nil {
				if _, ok := weaponBodyModels[key]; !ok {
					t.Errorf("melee weapon %q has no procedural body", key)
				}
			}
		}
	})

	// misfit names why the entry cannot show its style; empty = fine.
	type styled struct{ key, style, misfit string }
	cases := []struct {
		name    string
		entries func() []styled
	}{
		{name: "weapon slash_fx", entries: func() (out []styled) {
			for key, def := range config.GlobalWeapons.Weapons {
				if def.Graphics != nil && def.Graphics.SlashFx != "" {
					misfit := ""
					if def.Melee == nil {
						misfit = "no melee config"
					}
					out = append(out, styled{key, def.Graphics.SlashFx, misfit})
				}
			}
			return out
		}},
		{name: "weapon projectile_fx", entries: func() (out []styled) {
			for key, def := range config.GlobalWeapons.Weapons {
				if def.Graphics != nil && def.Graphics.ProjectileFx != "" {
					out = append(out, styled{key, def.Graphics.ProjectileFx, ""})
				}
			}
			return out
		}},
		{name: "spell projectile_fx", entries: func() (out []styled) {
			for key, def := range config.GlobalSpells.Spells {
				if def.Graphics != nil && def.Graphics.ProjectileFx != "" {
					misfit := ""
					if !def.IsProjectile {
						misfit = "no projectile"
					}
					out = append(out, styled{key, def.Graphics.ProjectileFx, misfit})
				}
			}
			return out
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			entries := tc.entries()
			if len(entries) == 0 {
				t.Fatal("no shipped entry authors this field: the validator ran on nothing")
			}
			for _, e := range entries {
				if e.misfit != "" {
					t.Errorf("%q authors style %q but has %s", e.key, e.style, e.misfit)
				}
			}
		})
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
		"drakefang_blade":   "dragon_fang",
		"wyrmcleaver":       "dragon_jaws",
		"ember_egg_mace":    "dragon_ember_egg",
		"broodspike":        "dragon_broodspike",
		"tarn_trident":      "dragon_tarn",
		"hatchling_fang":    "dragon_hatchling",
		"scalebreaker_maul": "dragon_roar",
		"vibro_blade":       "tech_vibro",
	}
	ranged := map[string]string{
		"wyrmspine_bow":       "dragon_wing",
		"nest_arbalest":       "dragon_nest",
		"verdant_eye_scepter": "dragon_eye",
		"suppressor_gun":      "tech_suppressor",
		"longlance_rifle":     "tech_longlance",
		"compound_bow":        "tech_compound_bow",
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

// Observe actual signature dispatch, including opacity and size. A bool that
// production ignores cannot prove the renderer drew the correct projection.
func TestProjectileFxDispatchSeparatesSizeAndOpacity(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		side, wantSide, wantFace float64
		independent              bool
	}{
		{name: "outgoing", wantFace: 1}, {name: "near axis", side: .02, wantFace: 1},
		{name: "oblique", side: math.Sqrt(.5), wantSide: .5, wantFace: .5},
		{name: "lateral", side: 1, wantSide: 1},
		{name: "eye", side: math.Sqrt(.5), wantFace: 1, independent: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &Renderer{game: &MMGame{}}
			style := "clock_pistol"
			if tc.independent {
				style = "dragon_eye"
			}
			previous := weaponProjectileFxStyles[style]
			defer func() { weaponProjectileFxStyles[style] = previous }()
			side, face := 0.0, 0.0
			calls := 0
			const size = 20.0
			const critical = 1.2
			check := func(got float64) {
				t.Helper()
				calls++
				if math.Abs(got-size*math.Sqrt(critical)) > 1e-9 {
					t.Errorf("blend changed critical size to %g", got)
				}
			}
			weaponProjectileFxStyles[style] = weaponProjectileFxStyle{
				side: func(_ *Renderer, _ *ebiten.Image, _, _, s, dx, dy, alpha float64, _ int) {
					check(s)
					side = alpha
					nx, ny := projectilePerpendicular(dx, dy)
					if math.Abs(math.Hypot(nx, ny)-1) > 1e-9 {
						t.Error("cross-section shrank")
					}
				},
				headOn: func(_ *Renderer, _ *ebiten.Image, _, _, s, alpha float64, _ int) { check(s); face = alpha }, viewIndependent: previous.viewIndependent,
			}
			r.drawWeaponProjectileFxForView(style, nil, 0, 0, size, projectileView{side: tc.side}, critical, 0)
			wantCalls := 0
			if tc.wantSide > 0 {
				wantCalls++
			}
			if tc.wantFace > 0 {
				wantCalls++
			}
			if calls != wantCalls || math.Abs(side-tc.wantSide) > 1e-9 || math.Abs(face-tc.wantFace) > 1e-9 {
				t.Fatalf("calls=%d side=%g face=%g", calls, side, face)
			}
		})
	}
}

// Drive the projectile pass through the actual presentation/shake scopes.
// Assert the projection submitted to the spell shader, not a helper return.
func TestSpellProjectileRenderingIgnoresCameraShake(t *testing.T) {
	g, _, _, _, _ := mouseCombatHarness(t, false)
	g.camera.Angle = 0
	g.camera.ViewDist = 5000
	g.screenShake = 2.2
	r := g.gameLoop.renderer
	dst := ebiten.NewImage(640, 480)
	defer dst.Deallocate()
	ts := float64(g.config.GetTileSize())
	for _, spell := range []string{"fireball", "lightning"} {
		g.magicProjectiles = []MagicProjectile{{Active: true, X: g.camera.X + ts/4, Y: g.camera.Y, VelX: -3, SpellType: spell, Owner: ProjectileOwnerMonster}}
		for frame := int64(0); frame < 2; frame++ {
			g.frameCount = frame
			undoView := g.swapCameraPose(cameraPose{g.camera.X, g.camera.Y + .7, .02})
			undoShake := g.beginScreenShakeSwap()
			r.drawMagicProjectiles(dst)
			undoShake()
			undoView()
			for _, v := range r.weaponMaterialQuad {
				if v.Custom2 != 1 {
					t.Fatalf("%s frame %d: shake turned end-on body sideways: %g", spell, frame, v.Custom2)
				}
			}
		}
	}
}

func TestProjectileProjectionRoutesHeadOnAndBothSideDirections(t *testing.T) {
	r := &Renderer{game: &MMGame{camera: &FirstPersonCamera{X: 64, Y: 96, Angle: .7}}}
	tests := []struct {
		name                  string
		x, y, vx, vy, wantDir float64
		wantFound             bool
	}{
		{name: "head on", x: 10, vx: 1},
		{name: "screen right", x: 10, vy: 1, wantDir: 1, wantFound: true},
		{name: "screen left", x: 10, vy: -1, wantDir: -1, wantFound: true},
		{name: "aim right", x: 10, y: 4, vx: 5, vy: 2},
		{name: "aim left", x: 10, y: -4, vx: 5, vy: -2},
		{name: "slow crossfire", x: 10, vy: .0001, wantDir: 1, wantFound: true},
		{name: "camera moved off ray", x: 10, y: -4, vx: 1, wantDir: 1, wantFound: true},
		{name: "stationary", x: 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			view := r.projectileView(r.game.camera.X+tt.x, r.game.camera.Y+tt.y, tt.vx, tt.vy)
			dir, found := view.screenDir()
			if found != tt.wantFound || dir != tt.wantDir {
				t.Fatalf("view=%+v direction=(%v,%v), want (%v,%v)", view, dir, found, tt.wantDir, tt.wantFound)
			}
		})
	}
}

func TestProjectileHeadOnDistinguishesIncomingAndOutgoing(t *testing.T) {
	for _, bearing := range []float64{0, .35, -.35, math.Pi / 2} {
		for _, incoming := range []bool{false, true} {
			r := &Renderer{game: &MMGame{camera: &FirstPersonCamera{Angle: .7}}}
			x, y := math.Cos(bearing), math.Sin(bearing)
			vx, vy := x, y
			if incoming {
				vx, vy = -vx, -vy
			}
			view := r.projectileView(10*x, 10*y, vx, vy)
			if view.incoming() != incoming || math.Abs(view.side) > 1e-6 {
				t.Fatalf("bearing %g incoming %v: view=%+v", bearing, incoming, view)
			}
			axis := view.cameraAxis([3]float64{view.side, 0, view.depth})
			wantX := -vx*math.Sin(r.game.camera.Angle) + vy*math.Cos(r.game.camera.Angle)
			wantZ := vx*math.Cos(r.game.camera.Angle) + vy*math.Sin(r.game.camera.Angle)
			if math.Abs(axis[0]-wantX) > 1e-6 || math.Abs(axis[2]-wantZ) > 1e-6 {
				t.Fatalf("volume axis=%v, want camera-space (%g, 0, %g)", axis, wantX, wantZ)
			}
		}
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

// The registries and the authoring must not drift APART either. validateWeaponFxStyles
// only fails on a style with no renderer; the other direction - a renderer no weapon
// names - is silent, and staticcheck cannot see it because the map entry counts as a
// use. That is how the melee "dragon_eye" survived its weapon being re-pointed at the
// projectile registry: 94 lines of unreachable FX and one name meaning two effects.
func TestEveryRegisteredWeaponFxStyleIsAuthored(t *testing.T) {
	if _, err := config.LoadWeaponConfig("../../assets/weapons.yaml"); err != nil {
		t.Fatalf("load weapons: %v", err)
	}
	slash, projectile := map[string]bool{}, map[string]bool{}
	for _, def := range config.GlobalWeapons.Weapons {
		if def == nil || def.Graphics == nil {
			continue
		}
		slash[def.Graphics.SlashFx] = true
		projectile[def.Graphics.ProjectileFx] = true
	}
	for style := range meleeFxStyleDraw {
		if !slash[style] {
			t.Errorf("melee fx style %q has a renderer but no weapon authors slash_fx: %s", style, style)
		}
	}
	for style := range weaponProjectileFxStyles {
		if !projectile[style] {
			t.Errorf("weapon projectile fx style %q has a renderer but no weapon authors projectile_fx: %s", style, style)
		}
	}
}

// Check the projection actually consumed by the arrow pass, including its
// previous shaft sample. Flight physics remain fixed when the view changes.
func TestProjectilePresentationAnchors(t *testing.T) {
	g, _, _, _, _ := mouseCombatHarness(t, false)
	g.renderHelper = NewRenderingHelper(g)
	g.showPartyStats = true
	g.camera.Angle, g.camera.ViewDist = 0, 5000
	r := g.gameLoop.renderer
	r.whiteImg = ebiten.NewImage(1, 1)
	r.whiteImg.Fill(color.White)
	defer r.whiteImg.Deallocate()
	dst := ebiten.NewImage(g.worldWidth(), g.worldHeight())
	defer dst.Deallocate()
	ts := g.config.GetTileSize()
	from := monster.NewMonster3DFromConfig(g.camera.X+2*ts, g.camera.Y-.5*ts, "rat", g.config)
	to := monster.NewMonster3DFromConfig(g.camera.X+4*ts, g.camera.Y+.5*ts, "rat", g.config)
	g.world.Monsters = []*monster.Monster3D{from, to}
	def := lookupWeaponConfigByKey("hunting_bow")
	for _, wide := range []bool{false, true} {
		g.combatPreferences.WideView = wide
		launch := g.combat.continuationLaunch(from, to)
		vx, vy := launch.dx*4, launch.dy*4
		sample := func(x, y float64) projectileProjection {
			p, _, ok := r.projectLaunchedEntity(launch, x, y, vx, vy, def.Graphics.BaseSize, def.Graphics.MinSize, def.Graphics.MaxSize)
			if !ok {
				t.Fatal("fixture sample not visible")
			}
			return p
		}
		for _, m := range []*monster.Monster3D{from, to} {
			p := sample(m.X, m.Y)
			a := visualAnchorFor(m)
			sx, ground, size, _ := g.renderHelper.CalculateMonsterSpriteMetricsF(m.X, m.Y, math.Hypot(m.X-g.camera.X, m.Y-g.camera.Y), a.sizeTiles)
			depth := m.X - g.camera.X
			want := a.bottom(g, depth, ground, size) - size/2
			if math.Abs(float64(p.screenY)+float64(p.size)/2-want) > 2 || math.Abs(float64(p.screenX)-sx) > 1 {
				t.Fatalf("wide=%v: projectile missed body at source/target", wide)
			}
		}
		// A short continuation must finish shedding its source offset by the
		// destination, even when that destination is less than one tile away.
		tx, ty := to.X, to.Y
		to.X, to.Y = from.X+ts/4, from.Y
		short := g.combat.continuationLaunch(from, to)
		wantOffset := visualAnchorFor(to).centerOffset(g, to.X-g.camera.X)
		if math.Abs(short.screenOffsetY(g, to.X, to.Y)-wantOffset) > 1e-6 {
			t.Fatal("short continuation retained its departure anchor at the target")
		}
		to.X, to.Y = tx, ty
		x, y := from.X+launch.dx*ts*.5, from.Y+launch.dy*ts*.5
		now, prev := sample(x, y), sample(x-vx*3, y-vy*3)
		wantAngle := math.Atan2(float64(now.screenY-prev.screenY)+float64(now.size-prev.size)/2, float64(now.screenX-prev.screenX))
		g.arrows = []Arrow{{Active: true, X: x, Y: y, VelX: vx, VelY: vy, BowKey: "hunting_bow", Launch: launch, LifeTime: def.Physics.GetLifetimeFrames(), Owner: ProjectileOwnerPlayer}}
		r.drawArrows(dst)
		if !g.arrows[0].RenderAngleSet || math.Abs(g.arrows[0].RenderAngle-wantAngle) > 1e-6 {
			t.Fatalf("wide=%v: shaft diverged from anchored flight: got %g want %g", wide, g.arrows[0].RenderAngle, wantAngle)
		}
		original := sample(to.X, to.Y)
		pierced := launch.piercing(g.combat, to)
		for _, state := range []string{"dead", "moved"} {
			hp, tx := to.HitPoints, to.X
			if state == "dead" {
				to.HitPoints = 0
			} else {
				to.X += ts
			}
			p := sample(launch.aimX, launch.aimY)
			base, _ := r.projectMovingEntity(launch.aimX, launch.aimY, def.Graphics.BaseSize, def.Graphics.MinSize, def.Graphics.MaxSize)
			if p != base {
				t.Fatalf("%s target still bends flight", state)
			}
			to.HitPoints, to.X = hp, tx
		}
		to.HitPoints = 0
		launch = pierced
		if got := sample(launch.aimX, launch.aimY); got != original {
			t.Fatal("piercing kill lost its body anchor")
		}
		x, y = launch.aimX+launch.dx*2*ts, launch.aimY+launch.dy*2*ts
		base, _ := r.projectMovingEntity(x, y, def.Graphics.BaseSize, def.Graphics.MinSize, def.Graphics.MaxSize)
		if got := sample(x, y); got != base {
			t.Fatal("pierce did not return to its physical ray")
		}
		to.HitPoints = to.MaxHitPoints
		if os.Getenv("RAM_DEBUG_SIM") != "" {
			launch = g.combat.continuationLaunch(from, to)
			g.magicProjectiles = []MagicProjectile{{Active: true, X: from.X + launch.dx*ts*.5, Y: from.Y + launch.dy*ts*.5, VelX: launch.dx * 2, VelY: launch.dy * 2, SpellType: "firebolt", Launch: launch, Owner: ProjectileOwnerPlayer}}
			pixels := make([]byte, dst.Bounds().Dx()*dst.Bounds().Dy()*4)
			runOnDrawFrame(func(_ *ebiten.Image) { dst.Clear(); r.drawMagicProjectiles(dst); dst.ReadPixels(pixels) })
			lit := 0
			for i := 3; i < len(pixels); i += 4 {
				if pixels[i] > 0 {
					lit++
				}
			}
			if lit == 0 {
				t.Fatal("spell presentation fixture did not draw")
			}
			// Every anchored sample is below this strip. Detached ghosts leave
			// glow here, even if a single tiny ghost rounds to zero alpha.
			for y := int(g.viewHorizon()) - 12; y <= int(g.viewHorizon())+12; y++ {
				for x := 0; x < dst.Bounds().Dx(); x++ {
					if pixels[(y*dst.Bounds().Dx()+x)*4+3] > 0 {
						t.Fatalf("wide=%v: ghost detached to the horizon", wide)
					}
				}
			}
		}
	}
}
