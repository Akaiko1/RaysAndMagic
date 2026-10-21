package sound

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"ugataima/internal/config"
	"ugataima/internal/damage"

	"github.com/hajimehoshi/ebiten/v2/audio"
	"gopkg.in/yaml.v3"
)

// The shipped catalog decodes every sound and track, routes every magic school
// and every non-magic ranged weapon category, and gives each map biome exactly
// one soundtrack (the boss track stays biome-free, see validateCatalog).
func TestProjectCatalogLoads(t *testing.T) {
	assets := filepath.Join("..", "..", "assets")
	catalog, err := LoadCatalog(filepath.Join(assets, "audio.yaml"))
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}
	for key, variants := range catalog.samples {
		if len(variants) == 0 {
			t.Fatalf("sound %q has no decoded variants", key)
		}
	}
	for key := range catalog.Music.Tracks {
		asset, ok := catalog.musicAssets[key]
		if !ok || asset.duration <= 0 {
			t.Fatalf("music %q is not decoded or has invalid duration %v", key, asset.duration)
		}
		if loopFade := time.Duration(catalog.Music.LoopCrossfadeMS) * time.Millisecond; asset.loop.end-asset.loop.start < 2*loopFade || asset.loop.end > asset.duration {
			t.Fatalf("music %q loop window %+v does not fit two loop fades of %v inside %v", key, asset.loop, loopFade, asset.duration)
		}
	}

	for _, school := range damage.Types() {
		if school != damage.Physical && catalog.SchoolSounds[school.String()] == "" {
			t.Errorf("school %q has no sound mapping", school)
		}
	}
	if _, err := config.LoadWeaponConfig(filepath.Join(assets, "weapons.yaml")); err != nil {
		t.Fatalf("load weapons: %v", err)
	}
	categories := config.RangedWeaponSoundCategories()
	if len(categories) == 0 {
		t.Fatal("no ranged weapon categories in the weapon catalog")
	}
	for _, category := range categories {
		if catalog.WeaponCategorySounds[category] == "" {
			t.Errorf("weapon category %q has no sound route", category)
		}
	}

	raw, err := os.ReadFile(filepath.Join(assets, "map_configs.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var mapConfigs config.MapConfigs
	if err := yaml.Unmarshal(raw, &mapConfigs); err != nil {
		t.Fatal(err)
	}
	if len(mapConfigs.Biomes) == 0 {
		t.Fatal("map_configs.yaml authors no biomes")
	}
	routed := map[string]string{}
	for trackKey, definition := range catalog.Music.Tracks {
		for _, biome := range definition.Biomes {
			if _, known := mapConfigs.Biomes[biome]; !known {
				t.Errorf("music %q names unknown biome %q", trackKey, biome)
			}
			if previous, dup := routed[biome]; dup {
				t.Errorf("biome %q routed to both %q and %q", biome, previous, trackKey)
			}
			routed[biome] = trackKey
		}
	}
	if len(routed) == 0 {
		t.Fatal("no music track routes any biome")
	}
}

func TestVariantPlayerCountsCoverVariantsAndConcurrency(t *testing.T) {
	tests := []struct {
		variants, maxInstances int
		want                   []int
	}{
		{variants: 1, maxInstances: 2, want: []int{2}},
		{variants: 6, maxInstances: 4, want: []int{1, 1, 1, 1, 1, 1}},
		{variants: 2, maxInstances: 5, want: []int{3, 2}},
	}
	for _, test := range tests {
		got := variantPlayerCounts(test.variants, test.maxInstances)
		if !slices.Equal(got, test.want) {
			t.Errorf("variantPlayerCounts(%d, %d) = %v, want %v", test.variants, test.maxInstances, got, test.want)
		}
	}
}

func TestValidateMusicBiomesRejectsUnknownCatalogBiome(t *testing.T) {
	manager := &Manager{catalog: &Catalog{}, musicBiome: map[string]string{"forrest": "forest_theme"}}
	err := manager.ValidateMusicBiomes([]string{"forest", "desert"})
	if err == nil || !strings.Contains(err.Error(), "forrest") {
		t.Fatalf("ValidateMusicBiomes error = %v, want unknown biome", err)
	}
}

func TestValidateCatalogRejectsInvalidBossMusicContract(t *testing.T) {
	tests := []struct {
		name        string
		bossTrack   string
		bossFadeMS  int
		bossBiomes  []string
		wantMessage string
	}{
		{name: "unknown track", bossTrack: "missing", bossFadeMS: 900, wantMessage: "unknown track"},
		{name: "missing crossfade", bossTrack: "boss", wantMessage: "boss_crossfade_ms"},
		{name: "boss assigned to biome", bossTrack: "boss", bossFadeMS: 900, bossBiomes: []string{"forest"}, wantMessage: "must not define biomes"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			catalog := &Catalog{
				SampleRate: 48000,
				Buses:      map[string]float64{"master": 1, "sfx": 1, "music": 1},
				Sounds: map[string]SoundDefinition{
					"hit": {Files: []string{"hit.ogg"}, Bus: "sfx", Volume: 1, MaxInstances: 1},
				},
				Music: MusicDefinition{
					CrossfadeMS:     2500,
					BossCrossfadeMS: test.bossFadeMS,
					LoopCrossfadeMS: 3000,
					BossTrack:       test.bossTrack,
					Tracks: map[string]MusicTrackDefinition{
						"forest": {File: "forest.ogg", Biomes: []string{"forest"}, Volume: 1},
						"boss":   {File: "boss.ogg", Biomes: test.bossBiomes, Volume: 1},
					},
				},
			}
			if err := validateCatalog(catalog); err == nil || !strings.Contains(err.Error(), test.wantMessage) {
				t.Fatalf("validateCatalog error = %v, want %q", err, test.wantMessage)
			}
		})
	}
}

func TestValidateWeaponCategoriesRejectsMissingRoute(t *testing.T) {
	manager := &Manager{catalog: &Catalog{WeaponCategorySounds: map[string]string{"bow": "bow_release"}}}
	err := manager.ValidateWeaponCategories([]string{"bow", "throwing"})
	if err == nil || !strings.Contains(err.Error(), "throwing") {
		t.Fatalf("ValidateWeaponCategories error = %v, want missing throwing route", err)
	}
}

func TestClosePersistsLatestVolumes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audio_settings.json")
	manager := &Manager{
		settingsPath: path,
		volumes:      VolumeSettings{Master: 0.75, SFX: 0.4, Music: 0.6},
	}
	manager.Close()
	if got := loadVolumeSettings(path); got != manager.volumes {
		t.Fatalf("saved settings = %+v, want %+v", got, manager.volumes)
	}
}

func TestLoadCatalogRejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audio.yaml")
	data := []byte("sample_rate: 48000\nbuses:\n  master: 1\n  sfx: 1\nsounds:\n  hit:\n    files: [hit.ogg]\n    bus: sfx\n    volume: 1\n    cooldown_ms: 0\n    max_instances: 1\n    typo: true\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCatalog(path); err == nil || !strings.Contains(err.Error(), "field typo not found") {
		t.Fatalf("LoadCatalog error = %v, want unknown-field rejection", err)
	} else if !IsCatalogContractError(err) {
		t.Fatalf("LoadCatalog error type = %T, want CatalogContractError", err)
	}
}

func TestLoadCatalogMarksMissingAssetsAsContractErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audio.yaml")
	data := []byte("sample_rate: 48000\nbuses:\n  master: 1\n  sfx: 1\nsounds:\n  hit:\n    files: [missing.ogg]\n    bus: sfx\n    volume: 1\n    cooldown_ms: 0\n    max_instances: 1\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := LoadCatalog(path)
	if err == nil || !strings.Contains(err.Error(), "missing.ogg") {
		t.Fatalf("LoadCatalog error = %v, want missing asset", err)
	}
	if !IsCatalogContractError(err) {
		t.Fatalf("LoadCatalog error type = %T, want CatalogContractError", err)
	}
}

func TestCatalogAssetPathRejectsEscape(t *testing.T) {
	if _, err := catalogAssetPath("/tmp/assets", "../secret.ogg"); err == nil {
		t.Fatal("catalogAssetPath accepted a path outside the catalog directory")
	}
}

func TestValidateCatalogRequiresMusicBusForTracks(t *testing.T) {
	catalog := &Catalog{
		SampleRate: 48000,
		Buses:      map[string]float64{"master": 1, "sfx": 1},
		Sounds: map[string]SoundDefinition{
			"hit": {
				Files:        []string{"hit.ogg"},
				Bus:          "sfx",
				Volume:       1,
				MaxInstances: 1,
			},
		},
		Music: MusicDefinition{
			CrossfadeMS:     100,
			LoopCrossfadeMS: 100,
			Tracks: map[string]MusicTrackDefinition{
				"forest": {
					File:   "forest.ogg",
					Biomes: []string{"forest"},
					Volume: 1,
				},
			},
		},
	}

	err := validateCatalog(catalog)
	if err == nil || !strings.Contains(err.Error(), "music bus") {
		t.Fatalf("validateCatalog error = %v, want missing music bus error", err)
	}
}

func TestValidateCatalogRejectsUnknownSchoolSound(t *testing.T) {
	catalog := &Catalog{
		SampleRate:   48000,
		Buses:        map[string]float64{"master": 1, "sfx": 1},
		SchoolSounds: map[string]string{"fire": "missing"},
		Sounds: map[string]SoundDefinition{
			"hit": {
				Files:        []string{"hit.ogg"},
				Bus:          "sfx",
				Volume:       1,
				MaxInstances: 1,
			},
		},
	}

	err := validateCatalog(catalog)
	if err == nil || !strings.Contains(err.Error(), "unknown sound") {
		t.Fatalf("validateCatalog error = %v, want unknown school sound error", err)
	}
}

func TestValidateCatalogRejectsUnknownSchoolKey(t *testing.T) {
	catalog := &Catalog{
		SampleRate:   48000,
		Buses:        map[string]float64{"master": 1, "sfx": 1},
		SchoolSounds: map[string]string{"flame": "hit"},
		Sounds: map[string]SoundDefinition{
			"hit": {Files: []string{"hit.ogg"}, Bus: "sfx", Volume: 1, MaxInstances: 1},
		},
	}
	if err := validateCatalog(catalog); err == nil || !strings.Contains(err.Error(), "unknown magic school") {
		t.Fatalf("validateCatalog error = %v, want unknown school rejection", err)
	}
}

func TestValidateCatalogRejectsMusicBusForSound(t *testing.T) {
	catalog := &Catalog{
		SampleRate: 48000,
		Buses:      map[string]float64{"master": 1, "music": 1},
		Sounds: map[string]SoundDefinition{
			"stinger": {Files: []string{"stinger.ogg"}, Bus: "music", Volume: 1, MaxInstances: 1},
		},
	}
	if err := validateCatalog(catalog); err == nil || !strings.Contains(err.Error(), "invalid bus") {
		t.Fatalf("validateCatalog error = %v, want music bus rejection", err)
	}
}

func TestFailedVariantPickPreservesLastHeardVariant(t *testing.T) {
	manager := &Manager{
		lastVariant: map[string]int{"hit": 1},
		random:      rand.New(rand.NewSource(1)),
	}
	if _, player := manager.pickIdleVariantPlayerLocked("hit", [][]*audio.Player{{}, {}}); player != nil {
		t.Fatal("empty pools unexpectedly returned a player")
	}
	if got := manager.lastVariant["hit"]; got != 1 {
		t.Fatalf("last variant changed after failed play to %d, want 1", got)
	}
}

func TestVolumeSettingsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audio_settings.json")
	manager := &Manager{settingsPath: path}
	want := VolumeSettings{Master: 0.35, SFX: 0.6, Music: 0.8}
	if err := manager.saveVolumeSettings(want); err != nil {
		t.Fatalf("saveVolumeSettings: %v", err)
	}
	got := loadVolumeSettings(path)
	if got != want {
		t.Fatalf("settings = %+v, want %+v", got, want)
	}
}

func TestMissingVolumeSettingsDefaultToHalf(t *testing.T) {
	got := loadVolumeSettings(filepath.Join(t.TempDir(), "missing.json"))
	want := VolumeSettings{Master: 0.5, SFX: 0.5, Music: 0.5}
	if got != want {
		t.Fatalf("settings = %+v, want %+v", got, want)
	}
}

func TestSetVolumeSkipsUnchangedValue(t *testing.T) {
	manager := &Manager{
		catalog:     &Catalog{Buses: map[string]float64{"master": 1, "sfx": 1}},
		pools:       map[string][][]*audio.Player{},
		musicTracks: map[string]*musicTrack{},
		volumes:     VolumeSettings{Master: 0.5, SFX: 0.5, Music: 0.5},
	}
	if manager.SetVolume(VolumeMaster, 0.5) {
		t.Fatal("unchanged volume reported a mutation")
	}
	if !manager.SetVolume(VolumeMaster, 0.6) {
		t.Fatal("changed volume was ignored")
	}
}

func TestEqualPowerGain(t *testing.T) {
	if got := equalPowerGain(1, 0, 0); got != 1 {
		t.Fatalf("fade start = %f, want 1", got)
	}
	if got := equalPowerGain(1, 0, 1); got != 0 {
		t.Fatalf("fade end = %f, want 0", got)
	}
	want := math.Sqrt(0.5)
	if got := equalPowerGain(1, 0, 0.5); math.Abs(got-want) > 1e-9 {
		t.Fatalf("fade midpoint = %f, want %f", got, want)
	}
}

func TestMusicReturnsToPausedBiomePositionAfterFadeCompletes(t *testing.T) {
	const pausedAt = 3 * time.Minute
	player := &fakeMusicPlayer{playing: true, position: pausedAt}
	track := &musicTrack{
		definition: MusicTrackDefinition{Volume: 1},
		loop:       musicLoop{end: 10 * time.Minute},
		players:    []musicPlayer{player},
	}
	now := time.Now()
	manager := &Manager{
		catalog: &Catalog{
			Buses: map[string]float64{"master": 1, "music": 1},
			Music: MusicDefinition{CrossfadeMS: 2500, LoopCrossfadeMS: 3000},
		},
		musicTracks: map[string]*musicTrack{"forest": track},
		musicBiome:  map[string]string{"forest": "forest"},
		musicVoices: []*musicVoice{{
			trackKey:  "forest",
			player:    player,
			fade:      ramp{value: 1, from: 1, to: 0, started: now.Add(-time.Second), duration: time.Millisecond},
			stopAfter: true,
		}},
		volumes: VolumeSettings{Master: 1, Music: 1},
	}

	manager.Update()
	if player.playing {
		t.Fatal("completed fade-out left music playing")
	}
	if player.seeks != 0 || player.position != pausedAt {
		t.Fatalf("fade-out changed player to position %v after %d seeks, want %v and no seek", player.position, player.seeks, pausedAt)
	}
	if track.resume != player {
		t.Fatal("completed biome fade did not retain its resume player")
	}

	manager.SetMusicState("forest", false)
	if !player.playing {
		t.Fatal("returning to biome did not resume music")
	}
	if player.seeks != 0 || player.position != pausedAt {
		t.Fatalf("biome return started at %v after %d seeks, want resume at %v", player.position, player.seeks, pausedAt)
	}
}

func TestBossMusicStartsFreshThenBiomeResumes(t *testing.T) {
	const pausedAt = 3 * time.Minute
	forestPlayer := &fakeMusicPlayer{playing: true, position: pausedAt}
	bossPlayer := &fakeMusicPlayer{position: 2 * time.Minute}
	forestTrack := &musicTrack{
		definition: MusicTrackDefinition{Volume: 1},
		loop:       musicLoop{end: 10 * time.Minute},
		players:    []musicPlayer{forestPlayer},
	}
	// The boss track has an authored intro: a new fight still starts at the top.
	bossTrack := &musicTrack{
		definition: MusicTrackDefinition{Volume: 1},
		loop:       musicLoop{start: 30 * time.Second, end: 10 * time.Minute},
		players:    []musicPlayer{bossPlayer},
	}
	manager := &Manager{
		catalog: &Catalog{
			Buses: map[string]float64{"master": 1, "music": 1},
			Music: MusicDefinition{
				CrossfadeMS: 2500, BossCrossfadeMS: 900, LoopCrossfadeMS: 3000, BossTrack: "boss",
			},
		},
		musicTracks:  map[string]*musicTrack{"forest": forestTrack, "boss": bossTrack},
		musicBiome:   map[string]string{"forest": "forest"},
		desiredMusic: "forest",
		musicVoices:  []*musicVoice{{trackKey: "forest", player: forestPlayer, fade: ramp{value: 1}}},
		volumes:      VolumeSettings{Master: 1, Music: 1},
	}

	manager.SetMusicState("forest", true)
	if manager.desiredMusic != "boss" || !bossPlayer.playing || bossPlayer.position != 0 || bossPlayer.seeks != 1 {
		t.Fatalf("boss start = desired %q, position %v, seeks %d, playing %t", manager.desiredMusic, bossPlayer.position, bossPlayer.seeks, bossPlayer.playing)
	}
	if len(manager.musicVoices) != 2 || manager.musicVoices[0].fade.duration != 900*time.Millisecond {
		t.Fatal("boss entry did not use the authored short crossfade")
	}
	for _, voice := range manager.musicVoices {
		voice.fade.started = time.Now().Add(-time.Second)
	}
	manager.Update()
	if forestTrack.resume != forestPlayer || forestPlayer.position != pausedAt {
		t.Fatal("boss entry did not preserve the location track position")
	}

	manager.SetMusicState("forest", false)
	if manager.desiredMusic != "forest" || !forestPlayer.playing || forestPlayer.position != pausedAt || forestPlayer.seeks != 0 {
		t.Fatalf("biome resume = desired %q, position %v, seeks %d, playing %t", manager.desiredMusic, forestPlayer.position, forestPlayer.seeks, forestPlayer.playing)
	}
	for _, voice := range manager.musicVoices {
		if voice.trackKey == "boss" && (!voice.stopAfter || voice.fade.duration != 2500*time.Millisecond) {
			t.Fatal("boss exit did not use the normal location crossfade")
		}
	}
}

// The loop handoff starts at the loop end less the loop crossfade, never
// before, and the repeat starts at the loop start: the top of the file when no
// window is authored, past the intro when one is. A player already in the
// outro hands off at once.
func TestMusicLoopWindow(t *testing.T) {
	const fade = 3 * time.Second
	whole := musicLoop{end: 10 * time.Minute}
	window := musicLoop{start: 12 * time.Second, end: 9 * time.Minute}
	for _, tc := range []struct {
		name        string
		loop        musicLoop
		position    time.Duration
		wantHandoff bool
		wantStart   time.Duration
	}{
		{"whole file, before the handoff", whole, whole.end - fade - time.Second, false, 0},
		{"whole file, at the handoff", whole, whole.end - fade, true, 0},
		{"window, before the handoff", window, window.end - fade - time.Second, false, 0},
		{"window, at the handoff", window, window.end - fade, true, window.start},
		{"window, in the outro", window, window.end + time.Second, true, window.start},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current := &fakeMusicPlayer{playing: true, position: tc.position}
			standby := &fakeMusicPlayer{position: 4 * time.Minute}
			manager := &Manager{
				catalog: &Catalog{
					Buses: map[string]float64{"master": 1, "music": 1},
					Music: MusicDefinition{CrossfadeMS: 2500, LoopCrossfadeMS: int(fade / time.Millisecond)},
				},
				musicTracks: map[string]*musicTrack{"forest": {
					definition: MusicTrackDefinition{Volume: 1},
					loop:       tc.loop,
					players:    []musicPlayer{current, standby},
				}},
				desiredMusic: "forest",
				musicVoices:  []*musicVoice{{trackKey: "forest", player: current, fade: ramp{value: 1}}},
				volumes:      VolumeSettings{Master: 1, Music: 1},
			}

			manager.beginLoopCrossfadeLocked(time.Now())
			if !tc.wantHandoff {
				if standby.playing || len(manager.musicVoices) != 1 || manager.musicVoices[0].stopAfter {
					t.Fatal("loop handed off before the loop end")
				}
				return
			}
			if !standby.playing || standby.seeks != 1 || standby.position != tc.wantStart {
				t.Fatalf("repeat = position %v, seeks %d, playing %t; want a playing start at %v", standby.position, standby.seeks, standby.playing, tc.wantStart)
			}
			if len(manager.musicVoices) != 2 || !manager.musicVoices[0].stopAfter || manager.musicVoices[0].fade.duration != fade {
				t.Fatal("loop crossfade did not hand off between two voices over the loop fade")
			}
		})
	}
}

// Returning to a biome resumes its track where it paused, unless too little is
// left before the loop end to be heard: then it restarts at the loop start, not
// at the intro.
func TestMusicResumeNearTheLoopEndRestartsTheLoop(t *testing.T) {
	window := musicLoop{start: 12 * time.Second, end: 9 * time.Minute}
	for _, tc := range []struct {
		name      string
		pausedAt  time.Duration
		wantStart time.Duration
		wantSeeks int
	}{
		{"mid-loop", 3 * time.Minute, 3 * time.Minute, 0},
		{"in the intro", 5 * time.Second, 5 * time.Second, 0},
		{"inside the last loop fade", window.end - time.Second, window.start, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			player := &fakeMusicPlayer{position: tc.pausedAt}
			manager := &Manager{
				catalog: &Catalog{
					Buses: map[string]float64{"master": 1, "music": 1},
					Music: MusicDefinition{CrossfadeMS: 2500, LoopCrossfadeMS: 3000},
				},
				musicTracks: map[string]*musicTrack{"forest": {
					definition: MusicTrackDefinition{Volume: 1},
					loop:       window,
					players:    []musicPlayer{player},
					resume:     player,
				}},
				musicBiome: map[string]string{"forest": "forest"},
				volumes:    VolumeSettings{Master: 1, Music: 1},
			}
			manager.SetMusicState("forest", false)
			if !player.playing || player.position != tc.wantStart || player.seeks != tc.wantSeeks {
				t.Fatalf("resume = position %v, seeks %d, playing %t; want %v with %d seeks", player.position, player.seeks, player.playing, tc.wantStart, tc.wantSeeks)
			}
		})
	}
}

// A heard ducking cue lowers the music by its depth over the attack (an even
// dB slope), holds while the cue plays, then releases; a silenced cue leaves
// the music alone. The dip follows the deepest cue still sounding: an
// overlapping cue extends the hold, a deeper one deepens the dip from where
// it is (no jump), a cue during the release pulls the music back down to its
// own depth (rising to it at the pace of the release under way), and when a
// short deep cue ends inside a long shallow one the music rises to the
// shallow depth instead of holding the deep one.
func TestMusicDuck(t *testing.T) {
	player := &fakeMusicPlayer{playing: true}
	manager := &Manager{
		catalog: &Catalog{
			Buses: map[string]float64{"master": 1, "music": 1},
			Music: MusicDefinition{DuckAttackMS: 100, DuckReleaseMS: 400},
		},
		musicTracks: map[string]*musicTrack{"forest": {definition: MusicTrackDefinition{Volume: 1}}},
		musicVoices: []*musicVoice{{trackKey: "forest", player: player, fade: ramp{value: 1}}},
		volumes:     VolumeSettings{Master: 1, Music: 1},
	}
	t0 := time.Now()
	at := func(ms int) time.Time { return t0.Add(time.Duration(ms) * time.Millisecond) }
	level := func() float64 { return 20 * math.Log10(player.volume) }
	expect := func(step string, want float64) {
		t.Helper()
		if got := level(); math.Abs(got-want) > 0.01 {
			t.Fatalf("%s: music at %.2f dB, want %.2f dB", step, got, want)
		}
	}

	manager.refreshMusicVolumesLocked()
	manager.updateDuckLocked(at(0))
	expect("no duck", 0)
	cue := SoundDefinition{DuckMusicDB: 6}
	manager.duckForCueLocked(cue, 0, at(1000), at(0))
	manager.updateDuckLocked(at(100))
	expect("a silenced cue leaves the music alone", 0)
	manager.duckForCueLocked(cue, 0.5, at(1000), at(0))
	manager.updateDuckLocked(at(50))
	expect("mid-attack is half the depth in dB", -3)
	manager.updateDuckLocked(at(100))
	expect("attack done", -6)
	manager.updateDuckLocked(at(900))
	expect("held while the cue plays", -6)

	manager.duckMusicLocked(6, at(1500), at(900))
	manager.updateDuckLocked(at(1200))
	expect("overlapping cue extends the hold", -6)

	manager.duckMusicLocked(12, at(1600), at(1200))
	manager.updateDuckLocked(at(1200))
	expect("deeper cue starts from the current dip", -6)
	manager.updateDuckLocked(at(1300))
	expect("deeper cue reached", -12)

	manager.updateDuckLocked(at(1600))
	expect("release starts at the hold end", -12)
	manager.updateDuckLocked(at(1800))
	expect("mid-release is half the depth in dB", -6)
	manager.updateDuckLocked(at(2000))
	expect("released", 0)

	manager.duckMusicLocked(6, at(2600), at(2100))
	manager.updateDuckLocked(at(2200))
	expect("a new cue ducks again", -6)
	manager.updateDuckLocked(at(2700))
	manager.duckMusicLocked(6, at(3200), at(2800))
	manager.updateDuckLocked(at(2900))
	expect("a cue during the release pulls the music back down", -6)

	manager.updateDuckLocked(at(3200))
	manager.updateDuckLocked(at(3600))
	expect("released again", 0)
	manager.duckMusicLocked(12, at(4000), at(3700))
	manager.updateDuckLocked(at(3800))
	expect("a deep cue", -12)
	manager.updateDuckLocked(at(4000))
	manager.updateDuckLocked(at(4100))
	expect("its release under way", -9)
	manager.duckMusicLocked(6, at(5000), at(4100))
	manager.updateDuckLocked(at(4100))
	expect("a shallower cue starts from the current dip", -9)
	manager.updateDuckLocked(at(4150))
	expect("the music keeps the release pace", -7.5)
	manager.updateDuckLocked(at(4200))
	expect("and settles at the cue's own depth", -6)

	manager.updateDuckLocked(at(5000))
	manager.updateDuckLocked(at(5400))
	expect("released after the shallow cue", 0)
	manager.duckMusicLocked(12, at(6000), at(5500))
	manager.duckMusicLocked(3, at(9000), at(5700))
	manager.updateDuckLocked(at(5800))
	expect("a short deep cue over a long shallow one", -12)
	manager.updateDuckLocked(at(6000))
	manager.updateDuckLocked(at(6150))
	expect("the deep cue ends: the music rises", -7.5)
	manager.updateDuckLocked(at(8000))
	expect("the long shallow cue keeps its own depth", -3)
	manager.updateDuckLocked(at(9000))
	manager.updateDuckLocked(at(9400))
	expect("all cues over", 0)
}

// The loop window is checked against the real file length at load.
func TestLoadCatalogValidatesLoopWindow(t *testing.T) {
	dir := t.TempDir()
	assets := filepath.Join("..", "..", "assets", "audio")
	for _, f := range []string{"music/arena.ogg", "combat/bow_release_01.ogg"} {
		raw, err := os.ReadFile(filepath.Join(assets, f))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, filepath.Base(f)), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name       string
		start, end int
		wantErr    string
		wantLoop   musicLoop
	}{
		{"no window", 0, 0, "", musicLoop{end: -1}},
		{"window", 1000, 50000, "", musicLoop{start: time.Second, end: 50 * time.Second}},
		{"end past the file", 0, 60000, "past the end", musicLoop{}},
		{"window shorter than two loop fades", 50000, 55000, "two loop crossfades", musicLoop{}},
		{"start after end", 50000, 40000, "two loop crossfades", musicLoop{}},
		{"negative start", -1, 0, "must be >= 0", musicLoop{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, "audio.yaml")
			data := fmt.Sprintf("sample_rate: 48000\nbuses:\n  master: 1\n  sfx: 1\n  music: 1\n"+
				"music:\n  crossfade_ms: 2500\n  loop_crossfade_ms: 3000\n  tracks:\n    arena:\n      file: arena.ogg\n      biomes: [arena]\n      volume: 1\n      loop_start_ms: %d\n      loop_end_ms: %d\n"+
				"sounds:\n  bow:\n    files: [bow_release_01.ogg]\n    bus: sfx\n    volume: 1\n    cooldown_ms: 0\n    max_instances: 1\n", tc.start, tc.end)
			if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
			catalog, err := LoadCatalog(path)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) || !IsCatalogContractError(err) {
					t.Fatalf("LoadCatalog error = %v, want a contract error with %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			asset := catalog.musicAssets["arena"]
			want := tc.wantLoop
			if want.end < 0 {
				want.end = asset.duration
			}
			if asset.loop != want {
				t.Fatalf("loop = %+v, want %+v", asset.loop, want)
			}
		})
	}
}

type fakeMusicPlayer struct {
	playing  bool
	position time.Duration
	seeks    int
	volume   float64
}

func (p *fakeMusicPlayer) Close() error             { return nil }
func (p *fakeMusicPlayer) IsPlaying() bool          { return p.playing }
func (p *fakeMusicPlayer) Pause()                   { p.playing = false }
func (p *fakeMusicPlayer) Play()                    { p.playing = true }
func (p *fakeMusicPlayer) Position() time.Duration  { return p.position }
func (p *fakeMusicPlayer) SetVolume(volume float64) { p.volume = volume }
func (p *fakeMusicPlayer) SetPosition(position time.Duration) error {
	p.seeks++
	p.position = position
	return nil
}

// A ducking cue needs an authored attack and release, and a depth that dips
// the music without muting it.
func TestValidateCatalogDuckContract(t *testing.T) {
	for _, tc := range []struct {
		name            string
		duckDB          float64
		attack, release int
		wantErr         string
	}{
		{"no duck needs no envelope", 0, 0, 0, ""},
		{"duck with an envelope", 6, 150, 600, ""},
		{"duck without an attack", 6, 0, 600, "duck_attack_ms"},
		{"duck without a release", 6, 150, 0, "duck_release_ms"},
		{"negative depth", -1, 150, 600, "duck_music_db"},
		{"a mute, not a dip", maxDuckMusicDB + 1, 150, 600, "duck_music_db"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			catalog := &Catalog{
				SampleRate: 48000,
				Buses:      map[string]float64{"master": 1, "ui": 1},
				Music:      MusicDefinition{DuckAttackMS: tc.attack, DuckReleaseMS: tc.release},
				Sounds: map[string]SoundDefinition{
					"level_up": {Files: []string{"level_up.ogg"}, Bus: "ui", Volume: 1, MaxInstances: 1, DuckMusicDB: tc.duckDB},
				},
			}
			err := validateCatalog(catalog)
			if tc.wantErr == "" && err != nil {
				t.Fatalf("validateCatalog: %v", err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("validateCatalog error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}
