package sound

import (
	"maps"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2/audio"
)

func TestProjectCatalogLoads(t *testing.T) {
	catalog, err := LoadCatalog(filepath.Join("..", "..", "assets", "audio.yaml"))
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}
	for key, variants := range catalog.samples {
		if len(variants) == 0 {
			t.Fatalf("sound %q has no decoded variants", key)
		}
	}
	if len(catalog.Music.Tracks) != 17 {
		t.Fatalf("music track count = %d, want 17", len(catalog.Music.Tracks))
	}
	if catalog.Music.BossTrack != "boss_fight" {
		t.Fatalf("boss track = %q, want boss_fight", catalog.Music.BossTrack)
	}
	wantMusicBiomes := map[string]string{
		"forest": "forest", "desert": "desert", "water": "water", "church": "church",
		"clock_tower_workshop": "clock_tower", "clock_tower_gearworks": "clock_tower", "clock_tower_belfry": "clock_tower",
		"arena": "arena", "pyramid": "pyramid", "lich_nexus": "lich_nexus", "culverts": "culverts",
		"japanese_castle": "japanese_castle", "city": "city", "elf_city": "elf_city", "nomad_city": "nomad_city",
		"highlands": "highlands", "dragon_cliffs": "dragon_cliffs", "jungle": "jungle",
	}
	gotMusicBiomes := make(map[string]string, len(wantMusicBiomes))
	for trackKey, definition := range catalog.Music.Tracks {
		for _, biome := range definition.Biomes {
			gotMusicBiomes[biome] = trackKey
		}
	}
	if !maps.Equal(gotMusicBiomes, wantMusicBiomes) {
		t.Fatalf("music biome routes = %v, want %v", gotMusicBiomes, wantMusicBiomes)
	}
	for key, asset := range catalog.musicAssets {
		if asset.duration <= 0 {
			t.Fatalf("music %q has invalid duration %v", key, asset.duration)
		}
		if loopFade := time.Duration(catalog.Music.LoopCrossfadeMS) * time.Millisecond; asset.duration <= loopFade {
			t.Fatalf("music %q duration %v is not longer than loop fade %v", key, asset.duration, loopFade)
		}
	}
	for _, key := range []string{"monster_hit", "party_hit"} {
		if got := len(catalog.Sounds[key].Files); got != 6 {
			t.Fatalf("sound %q variant count = %d, want 6", key, got)
		}
	}
	for _, school := range []string{"fire", "water", "air", "earth", "light", "body", "spirit", "dark", "mind"} {
		if catalog.SchoolSounds[school] == "" {
			t.Errorf("school %q has no sound mapping", school)
		}
	}
	for _, school := range []string{"body", "spirit"} {
		if got := catalog.OffensiveSchoolSounds[school]; got != "spell_dark_psychic" {
			t.Errorf("offensive school %q route = %q, want spell_dark_psychic", school, got)
		}
	}
	for category, want := range map[string]string{"bow": "bow_release", "blaster": "blaster_shot", "dagger": "melee_swing"} {
		if got := catalog.WeaponCategorySounds[category]; got != want {
			t.Errorf("weapon category %q route = %q, want %q", category, got, want)
		}
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
		duration:   10 * time.Minute,
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
			trackKey:     "forest",
			player:       player,
			gain:         1,
			fadeFrom:     1,
			fadeTo:       0,
			fadeStarted:  now.Add(-time.Second),
			fadeDuration: time.Millisecond,
			stopAfter:    true,
		}},
		volumes: VolumeSettings{Master: 1, Music: 1},
	}

	manager.Update()
	if player.playing {
		t.Fatal("completed fade-out left music playing")
	}
	if player.rewinds != 0 || player.position != pausedAt {
		t.Fatalf("fade-out changed player to position %v after %d rewinds, want %v and no rewind", player.position, player.rewinds, pausedAt)
	}
	if track.resume != player {
		t.Fatal("completed biome fade did not retain its resume player")
	}

	manager.SetMusicBiome("forest")
	if !player.playing {
		t.Fatal("returning to biome did not resume music")
	}
	if player.rewinds != 0 || player.position != pausedAt {
		t.Fatalf("biome return started at %v after %d rewinds, want resume at %v", player.position, player.rewinds, pausedAt)
	}
}

func TestBossMusicStartsFreshThenBiomeResumes(t *testing.T) {
	const pausedAt = 3 * time.Minute
	forestPlayer := &fakeMusicPlayer{playing: true, position: pausedAt}
	bossPlayer := &fakeMusicPlayer{position: 2 * time.Minute}
	forestTrack := &musicTrack{
		definition: MusicTrackDefinition{Volume: 1},
		duration:   10 * time.Minute,
		players:    []musicPlayer{forestPlayer},
	}
	bossTrack := &musicTrack{
		definition: MusicTrackDefinition{Volume: 1},
		duration:   10 * time.Minute,
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
		musicVoices:  []*musicVoice{{trackKey: "forest", player: forestPlayer, gain: 1}},
		volumes:      VolumeSettings{Master: 1, Music: 1},
	}

	manager.SetMusicState("forest", true)
	if manager.desiredMusic != "boss" || !bossPlayer.playing || bossPlayer.position != 0 || bossPlayer.rewinds != 1 {
		t.Fatalf("boss start = desired %q, position %v, rewinds %d, playing %t", manager.desiredMusic, bossPlayer.position, bossPlayer.rewinds, bossPlayer.playing)
	}
	if len(manager.musicVoices) != 2 || manager.musicVoices[0].fadeDuration != 900*time.Millisecond {
		t.Fatal("boss entry did not use the authored short crossfade")
	}
	for _, voice := range manager.musicVoices {
		voice.fadeStarted = time.Now().Add(-time.Second)
	}
	manager.Update()
	if forestTrack.resume != forestPlayer || forestPlayer.position != pausedAt {
		t.Fatal("boss entry did not preserve the location track position")
	}

	manager.SetMusicState("forest", false)
	if manager.desiredMusic != "forest" || !forestPlayer.playing || forestPlayer.position != pausedAt || forestPlayer.rewinds != 0 {
		t.Fatalf("biome resume = desired %q, position %v, rewinds %d, playing %t", manager.desiredMusic, forestPlayer.position, forestPlayer.rewinds, forestPlayer.playing)
	}
	for _, voice := range manager.musicVoices {
		if voice.trackKey == "boss" && (!voice.stopAfter || voice.fadeDuration != 2500*time.Millisecond) {
			t.Fatal("boss exit did not use the normal location crossfade")
		}
	}
}

func TestLoopCrossfadeStartsStandbyPlayerFromBeginning(t *testing.T) {
	const trackDuration = 10 * time.Minute
	current := &fakeMusicPlayer{playing: true, position: trackDuration - time.Second}
	standby := &fakeMusicPlayer{position: 4 * time.Minute}
	track := &musicTrack{
		definition: MusicTrackDefinition{Volume: 1},
		duration:   trackDuration,
		players:    []musicPlayer{current, standby},
	}
	manager := &Manager{
		catalog: &Catalog{
			Buses: map[string]float64{"master": 1, "music": 1},
			Music: MusicDefinition{CrossfadeMS: 2500, LoopCrossfadeMS: 3000},
		},
		musicTracks:  map[string]*musicTrack{"forest": track},
		desiredMusic: "forest",
		musicVoices:  []*musicVoice{{trackKey: "forest", player: current, gain: 1}},
		volumes:      VolumeSettings{Master: 1, Music: 1},
	}

	manager.beginLoopCrossfadeLocked(time.Now())
	if standby.rewinds != 1 || standby.position != 0 || !standby.playing {
		t.Fatalf("loop standby state = position %v, rewinds %d, playing %t; want fresh playing start", standby.position, standby.rewinds, standby.playing)
	}
	if len(manager.musicVoices) != 2 || !manager.musicVoices[0].stopAfter {
		t.Fatal("loop crossfade did not hand off between two voices")
	}
}

type fakeMusicPlayer struct {
	playing  bool
	position time.Duration
	rewinds  int
	volume   float64
}

func (p *fakeMusicPlayer) Close() error             { return nil }
func (p *fakeMusicPlayer) IsPlaying() bool          { return p.playing }
func (p *fakeMusicPlayer) Pause()                   { p.playing = false }
func (p *fakeMusicPlayer) Play()                    { p.playing = true }
func (p *fakeMusicPlayer) Position() time.Duration  { return p.position }
func (p *fakeMusicPlayer) SetVolume(volume float64) { p.volume = volume }
func (p *fakeMusicPlayer) Rewind() error {
	p.rewinds++
	p.position = 0
	return nil
}
