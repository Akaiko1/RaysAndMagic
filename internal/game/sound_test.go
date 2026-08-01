package game

import (
	"slices"
	"testing"
	"time"

	"ugataima/internal/config"
	"ugataima/internal/damage"
	monsterPkg "ugataima/internal/monster"
)

func TestBeginAudioSliderDragConsumesBufferedClick(t *testing.T) {
	g := &MMGame{
		audioSliderDrag: -1,
		mouseLeftClicks: []queuedClick{{x: 10, y: 20, at: time.Now().UnixMilli()}},
	}

	g.beginAudioSliderDrag(2)

	if g.audioSettingsSelection != 2 || g.audioSliderDrag != 2 {
		t.Fatalf("slider state = selection %d, drag %d; want 2, 2", g.audioSettingsSelection, g.audioSliderDrag)
	}
	if len(g.mouseLeftClicks) != 0 {
		t.Fatalf("buffered click count = %d, want 0", len(g.mouseLeftClicks))
	}
}

func TestRequiredSoundSchoolsFollowDamageCatalog(t *testing.T) {
	want := make([]string, 0, len(damage.Types())-1)
	for _, school := range damage.Types() {
		if school != damage.Physical {
			want = append(want, school.String())
		}
	}
	if got := RequiredSoundSchools(); !slices.Equal(got, want) {
		t.Fatalf("RequiredSoundSchools() = %v, want %v", got, want)
	}
}

func TestWorldSoundGainUsesViewDistance(t *testing.T) {
	g := &MMGame{camera: &FirstPersonCamera{X: 100, Y: 100, ViewDist: 500}}
	if got := g.worldSoundGain(100, 100); got != 1 {
		t.Fatalf("source gain = %v, want 1", got)
	}
	if got := g.worldSoundGain(400, 500); got != 0 {
		t.Fatalf("view-boundary gain = %v, want 0", got)
	}
	if got := g.worldSoundGain(250, 300); got != 0.25 {
		t.Fatalf("half-distance gain = %v, want 0.25", got)
	}
	if g.playMonsterSound(soundMonsterHit, (*monsterPkg.Monster3D)(nil)) {
		t.Fatal("nil monster should not play a world sound")
	}
}

func TestBossMusicCombatActiveUsesExplicitBossCombatState(t *testing.T) {
	summon := &monsterPkg.Monster3D{HitPoints: 1, Bound: true}
	tests := []struct {
		name     string
		monsters []*monsterPkg.Monster3D
		want     bool
	}{
		{
			name:     "boss fighting party",
			monsters: []*monsterPkg.Monster3D{{Boss: true, HitPoints: 1, IsEngagingPlayer: true, State: monsterPkg.StateAlert}},
			want:     true,
		},
		{
			name:     "boss fighting summon",
			monsters: []*monsterPkg.Monster3D{{Boss: true, HitPoints: 1, AIFoe: summon, State: monsterPkg.StatePursuing}},
			want:     true,
		},
		{
			name:     "boss temporarily fleeing",
			monsters: []*monsterPkg.Monster3D{{Boss: true, HitPoints: 1, State: monsterPkg.StateFleeing}},
			want:     true,
		},
		{
			name:     "one of several bosses remains active",
			monsters: []*monsterPkg.Monster3D{{Boss: true, HitPoints: 0}, {Boss: true, HitPoints: 1, AIFoe: summon}},
			want:     true,
		},
		{
			name:     "calm boss",
			monsters: []*monsterPkg.Monster3D{{Boss: true, HitPoints: 1, State: monsterPkg.StateIdle}},
		},
		{
			name:     "dormant boss with stale engagement",
			monsters: []*monsterPkg.Monster3D{{Boss: true, HitPoints: 1, BossDormant: true, IsEngagingPlayer: true, State: monsterPkg.StateFleeing}},
		},
		{
			name:     "warded boss with stale engagement",
			monsters: []*monsterPkg.Monster3D{{Boss: true, HitPoints: 1, BossWarded: true, IsEngagingPlayer: true, State: monsterPkg.StateFleeing}},
		},
		{
			name:     "evasive boss with stale engagement",
			monsters: []*monsterPkg.Monster3D{{Boss: true, HitPoints: 1, BossEvasive: true, IsEngagingPlayer: true, State: monsterPkg.StateFleeing}},
		},
		{
			name:     "dead fleeing boss",
			monsters: []*monsterPkg.Monster3D{{Boss: true, HitPoints: 0, IsEngagingPlayer: true, State: monsterPkg.StateFleeing}},
		},
		{
			name:     "arena champion is not a yaml boss",
			monsters: []*monsterPkg.Monster3D{{ChampionKey: "weapon_master", HitPoints: 1, IsEngagingPlayer: true}},
		},
		{
			name:     "ordinary monster",
			monsters: []*monsterPkg.Monster3D{{HitPoints: 1, IsEngagingPlayer: true}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := bossMusicCombatActive(test.monsters); got != test.want {
				t.Fatalf("bossMusicCombatActive() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestMagicRangedWeaponCategoriesUseMagicRoute(t *testing.T) {
	if !config.IsMagicRangedWeapon(&config.WeaponDefinitionConfig{Category: " staff ", Range: 6}) {
		t.Fatal("staff category should ignore surrounding whitespace")
	}
	if !config.IsMagicRangedWeapon(&config.WeaponDefinitionConfig{Category: " BOOK ", Range: 6}) {
		t.Fatal("book category should ignore case and surrounding whitespace")
	}
}

func TestMagicRangedWeaponAttackUsesOffensiveSchoolRoute(t *testing.T) {
	school, offensive := magicRangedWeaponAttackSoundRoute(&config.WeaponDefinitionConfig{ProjectileSchool: "spirit"})
	if school != "spirit" || !offensive {
		t.Fatalf("magic route = (%q, %t), want (spirit, true)", school, offensive)
	}
}

func TestRequiredGameplaySoundKeysAreRegisteredOnce(t *testing.T) {
	seen := make(map[string]bool, gameplaySoundCount)
	for _, key := range RequiredSoundKeys() {
		if key == "" {
			t.Fatal("required gameplay sound contains an empty key")
		}
		if seen[key] {
			t.Fatalf("required gameplay sound %q is registered twice", key)
		}
		seen[key] = true
	}
}

func TestCloseAudioSettingsRestoresOwningMenu(t *testing.T) {
	for _, test := range []struct {
		name      string
		entryMode EntryMenuMode
		mainMode  MainMenuMode
	}{
		{name: "entry", entryMode: EntryMenuSettings, mainMode: MenuMain},
		{name: "pause", entryMode: EntryMenuRoot, mainMode: MenuSettings},
	} {
		t.Run(test.name, func(t *testing.T) {
			g := &MMGame{
				entryMenuMode:      test.entryMode,
				mainMenuMode:       test.mainMode,
				audioSliderDrag:    1,
				audioSettingsDirty: true,
				mouseLeftClicks:    []queuedClick{{x: 100, y: 200, at: time.Now().UnixMilli()}},
			}

			g.closeAudioSettings()

			if g.entryMenuMode != EntryMenuRoot || g.mainMenuMode != MenuMain {
				t.Fatalf("restored modes = entry %d, main %d", g.entryMenuMode, g.mainMenuMode)
			}
			if g.audioSliderDrag != -1 || g.audioSettingsDirty {
				t.Fatalf("audio state = drag %d, dirty %t", g.audioSliderDrag, g.audioSettingsDirty)
			}
			if len(g.mouseLeftClicks) != 0 {
				t.Fatalf("buffered click count = %d, want 0", len(g.mouseLeftClicks))
			}
		})
	}
}

func TestRequiredWeaponSoundCategoriesFollowWeaponCatalog(t *testing.T) {
	loadTestConfig(t)
	want := []string{"blaster", "bow", "dagger"}
	if got := RequiredWeaponSoundCategories(); !slices.Equal(got, want) {
		t.Fatalf("RequiredWeaponSoundCategories() = %v, want %v", got, want)
	}
}
