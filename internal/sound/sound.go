package sound

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"ugataima/internal/damage"

	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/audio/vorbis"
	"gopkg.in/yaml.v3"
)

// Catalog is the data-driven sound registry. Game code refers only to the
// keys in Sounds; file names, variants, routing, and playback limits live in
// assets/audio.yaml.
type Catalog struct {
	SampleRate            int                        `yaml:"sample_rate"`
	Buses                 map[string]float64         `yaml:"buses"`
	SchoolSounds          map[string]string          `yaml:"school_sounds"`
	OffensiveSchoolSounds map[string]string          `yaml:"offensive_school_sounds"`
	WeaponCategorySounds  map[string]string          `yaml:"weapon_category_sounds"`
	Music                 MusicDefinition            `yaml:"music"`
	Sounds                map[string]SoundDefinition `yaml:"sounds"`

	samples     map[string][][]byte
	musicAssets map[string]musicAsset
}

// SoundDefinition configures one logical sound event.
type SoundDefinition struct {
	Files        []string `yaml:"files"`
	Bus          string   `yaml:"bus"`
	Volume       float64  `yaml:"volume"`
	CooldownMS   int      `yaml:"cooldown_ms"`
	MaxInstances int      `yaml:"max_instances"`
}

// MusicDefinition configures location transitions and seamless loop handoffs.
type MusicDefinition struct {
	CrossfadeMS     int                             `yaml:"crossfade_ms"`
	BossCrossfadeMS int                             `yaml:"boss_crossfade_ms"`
	LoopCrossfadeMS int                             `yaml:"loop_crossfade_ms"`
	BossTrack       string                          `yaml:"boss_track"`
	Tracks          map[string]MusicTrackDefinition `yaml:"tracks"`
}

// MusicTrackDefinition maps one streamed music file to one or more biomes.
type MusicTrackDefinition struct {
	File   string   `yaml:"file"`
	Biomes []string `yaml:"biomes"`
	Volume float64  `yaml:"volume"`
}

type musicAsset struct {
	// data is the shared backing store for the lazy Vorbis streams owned by the
	// two music players. It must stay alive for the lifetime of those players.
	data     []byte
	duration time.Duration
}

type decodedStream interface {
	io.Reader
	SampleRate() int
}

// CatalogContractError marks static catalog/content violations separately from
// runtime audio device/player failures, which may degrade to silent gameplay.
type CatalogContractError struct {
	err error
}

func (e *CatalogContractError) Error() string { return e.err.Error() }
func (e *CatalogContractError) Unwrap() error { return e.err }

func IsCatalogContractError(err error) bool {
	var contractErr *CatalogContractError
	return errors.As(err, &contractErr)
}

// LoadCatalog parses, validates, and pre-decodes every short sound effect.
// Pre-decoding keeps gameplay playback free of disk IO and decoder work.
func LoadCatalog(path string) (_ *Catalog, err error) {
	defer func() {
		if err != nil && !IsCatalogContractError(err) {
			err = &CatalogContractError{err: err}
		}
	}()

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open sound catalog: %w", err)
	}
	defer f.Close()

	var catalog Catalog
	decoder := yaml.NewDecoder(f)
	decoder.KnownFields(true)
	if err := decoder.Decode(&catalog); err != nil {
		return nil, &CatalogContractError{err: fmt.Errorf("decode sound catalog: %w", err)}
	}
	if err := validateCatalog(&catalog); err != nil {
		return nil, &CatalogContractError{err: err}
	}

	baseDir := filepath.Dir(path)
	catalog.samples, err = decodeCatalogSoundsF32(baseDir, catalog.SampleRate, catalog.Sounds)
	if err != nil {
		return nil, err
	}
	catalog.musicAssets = make(map[string]musicAsset, len(catalog.Music.Tracks))
	for _, key := range sortedMusicKeys(catalog.Music.Tracks) {
		def := catalog.Music.Tracks[key]
		fullPath, err := catalogAssetPath(baseDir, def.File)
		if err != nil {
			return nil, fmt.Errorf("music %q: %w", key, err)
		}
		raw, err := os.ReadFile(fullPath)
		if err != nil {
			return nil, fmt.Errorf("music %q file %q: %w", key, def.File, err)
		}
		stream, err := vorbis.DecodeF32(bytes.NewReader(raw))
		if err != nil {
			return nil, fmt.Errorf("music %q file %q: %w", key, def.File, err)
		}
		if stream.SampleRate() != catalog.SampleRate {
			return nil, fmt.Errorf("music %q file %q sample rate is %d, want %d", key, def.File, stream.SampleRate(), catalog.SampleRate)
		}
		if stream.Length() <= 0 {
			return nil, fmt.Errorf("music %q file %q has unknown or empty decoded length", key, def.File)
		}
		bytesPerSecond := int64(catalog.SampleRate * 2 * 4)
		duration := time.Duration(float64(stream.Length()) / float64(bytesPerSecond) * float64(time.Second))
		loopFade := time.Duration(catalog.Music.LoopCrossfadeMS) * time.Millisecond
		if duration <= loopFade {
			return nil, fmt.Errorf("music %q duration %v must be longer than loop_crossfade_ms (%v)", key, duration, loopFade)
		}
		catalog.musicAssets[key] = musicAsset{data: raw, duration: duration}
	}
	return &catalog, nil
}

const maxParallelSoundDecoders = 8

type soundDecodeResult struct {
	variants [][]byte
	err      error
}

func decodeCatalogSoundsF32(baseDir string, sampleRate int, sounds map[string]SoundDefinition) (map[string][][]byte, error) {
	keys := sortedSoundKeys(sounds)
	results := make([]soundDecodeResult, len(keys))
	jobs := make(chan int, len(keys))
	for i := range keys {
		jobs <- i
	}
	close(jobs)

	var workers sync.WaitGroup
	workers.Add(min(len(keys), maxParallelSoundDecoders))
	for range min(len(keys), maxParallelSoundDecoders) {
		go func() {
			defer workers.Done()
			for i := range jobs {
				key := keys[i]
				results[i].variants, results[i].err = decodeSoundVariantsF32(baseDir, sampleRate, key, sounds[key])
			}
		}()
	}
	workers.Wait()

	samples := make(map[string][][]byte, len(keys))
	for i, key := range keys {
		if results[i].err != nil {
			return nil, results[i].err
		}
		samples[key] = results[i].variants
	}
	return samples, nil
}

func decodeSoundVariantsF32(baseDir string, sampleRate int, key string, def SoundDefinition) ([][]byte, error) {
	variants := make([][]byte, len(def.Files))
	for i, relativePath := range def.Files {
		fullPath, err := catalogAssetPath(baseDir, relativePath)
		if err != nil {
			return nil, fmt.Errorf("sound %q: %w", key, err)
		}
		pcm, decodedSampleRate, err := decodeFileF32(fullPath)
		if err != nil {
			return nil, fmt.Errorf("sound %q file %q: %w", key, relativePath, err)
		}
		if decodedSampleRate != sampleRate {
			return nil, fmt.Errorf("sound %q file %q sample rate is %d, want %d", key, relativePath, decodedSampleRate, sampleRate)
		}
		if len(pcm) == 0 {
			return nil, fmt.Errorf("sound %q file %q decoded to no samples", key, relativePath)
		}
		variants[i] = pcm
	}
	return variants, nil
}

func validateCatalog(catalog *Catalog) error {
	if catalog.SampleRate <= 0 {
		return fmt.Errorf("sound catalog sample_rate must be > 0")
	}
	if len(catalog.Buses) == 0 {
		return fmt.Errorf("sound catalog buses must not be empty")
	}
	if _, ok := catalog.Buses["master"]; !ok {
		return fmt.Errorf("sound catalog must define the master bus")
	}
	for bus, volume := range catalog.Buses {
		if strings.TrimSpace(bus) == "" {
			return fmt.Errorf("sound catalog contains an empty bus name")
		}
		if volume < 0 || volume > 1 {
			return fmt.Errorf("sound bus %q volume must be between 0 and 1", bus)
		}
	}
	if len(catalog.Sounds) == 0 {
		return fmt.Errorf("sound catalog sounds must not be empty")
	}
	for _, key := range sortedSoundKeys(catalog.Sounds) {
		def := catalog.Sounds[key]
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("sound catalog contains an empty sound key")
		}
		if len(def.Files) == 0 {
			return fmt.Errorf("sound %q must define at least one file", key)
		}
		if _, ok := catalog.Buses[def.Bus]; !ok || def.Bus == "master" || def.Bus == "music" {
			return fmt.Errorf("sound %q references invalid bus %q", key, def.Bus)
		}
		if def.Volume <= 0 || def.Volume > 1 {
			return fmt.Errorf("sound %q volume must be > 0 and <= 1", key)
		}
		if def.CooldownMS < 0 {
			return fmt.Errorf("sound %q cooldown_ms must be >= 0", key)
		}
		if def.MaxInstances <= 0 {
			return fmt.Errorf("sound %q max_instances must be > 0", key)
		}
		for _, file := range def.Files {
			if strings.TrimSpace(file) == "" {
				return fmt.Errorf("sound %q contains an empty file path", key)
			}
		}
	}
	if len(catalog.SchoolSounds) > 0 {
		normalized, err := normalizeSoundRouteMap("school_sounds", catalog.SchoolSounds, catalog.Sounds)
		if err != nil {
			return err
		}
		catalog.SchoolSounds = normalized
	}
	if len(catalog.OffensiveSchoolSounds) > 0 {
		normalized, err := normalizeSoundRouteMap("offensive_school_sounds", catalog.OffensiveSchoolSounds, catalog.Sounds)
		if err != nil {
			return err
		}
		catalog.OffensiveSchoolSounds = normalized
	}
	if len(catalog.WeaponCategorySounds) > 0 {
		normalized, err := normalizeSoundRouteMap("weapon_category_sounds", catalog.WeaponCategorySounds, catalog.Sounds)
		if err != nil {
			return err
		}
		catalog.WeaponCategorySounds = normalized
	}
	if err := validateCatalogSchoolKeys(catalog); err != nil {
		return err
	}
	if len(catalog.Music.Tracks) > 0 {
		if _, ok := catalog.Buses["music"]; !ok {
			return fmt.Errorf("sound catalog with music tracks must define the music bus")
		}
		if catalog.Music.CrossfadeMS <= 0 {
			return fmt.Errorf("music crossfade_ms must be > 0")
		}
		if catalog.Music.LoopCrossfadeMS <= 0 {
			return fmt.Errorf("music loop_crossfade_ms must be > 0")
		}
		bossTrack := strings.TrimSpace(catalog.Music.BossTrack)
		catalog.Music.BossTrack = bossTrack
		if bossTrack != "" {
			if _, ok := catalog.Music.Tracks[bossTrack]; !ok {
				return fmt.Errorf("music boss_track references unknown track %q", bossTrack)
			}
			if catalog.Music.BossCrossfadeMS <= 0 {
				return fmt.Errorf("music boss_crossfade_ms must be > 0 when boss_track is set")
			}
		} else if catalog.Music.BossCrossfadeMS < 0 {
			return fmt.Errorf("music boss_crossfade_ms must be >= 0")
		}
		seenBiomes := make(map[string]string)
		for _, key := range sortedMusicKeys(catalog.Music.Tracks) {
			def := catalog.Music.Tracks[key]
			if strings.TrimSpace(key) == "" {
				return fmt.Errorf("music contains an empty track key")
			}
			if strings.TrimSpace(def.File) == "" {
				return fmt.Errorf("music %q must define a file", key)
			}
			if strings.ToLower(filepath.Ext(def.File)) != ".ogg" {
				return fmt.Errorf("music %q file must be Ogg/Vorbis", key)
			}
			if def.Volume <= 0 || def.Volume > 1 {
				return fmt.Errorf("music %q volume must be > 0 and <= 1", key)
			}
			if key == bossTrack && len(def.Biomes) > 0 {
				return fmt.Errorf("boss music %q must not define biomes", key)
			}
			if key != bossTrack && len(def.Biomes) == 0 {
				return fmt.Errorf("music %q must define at least one biome", key)
			}
			for i, rawBiome := range def.Biomes {
				biome := strings.ToLower(strings.TrimSpace(rawBiome))
				if biome == "" {
					return fmt.Errorf("music %q contains an empty biome", key)
				}
				if previous, exists := seenBiomes[biome]; exists {
					return fmt.Errorf("music biome %q is assigned to both %q and %q", biome, previous, key)
				}
				seenBiomes[biome] = key
				def.Biomes[i] = biome
			}
			catalog.Music.Tracks[key] = def
		}
	}
	return nil
}

func normalizeSoundRouteMap(field string, routes map[string]string, sounds map[string]SoundDefinition) (map[string]string, error) {
	normalized := make(map[string]string, len(routes))
	for rawRoute, soundKey := range routes {
		route := strings.ToLower(strings.TrimSpace(rawRoute))
		if route == "" {
			return nil, fmt.Errorf("sound catalog contains an empty %s key", field)
		}
		if _, exists := normalized[route]; exists {
			return nil, fmt.Errorf("sound catalog contains duplicate %s key %q", field, route)
		}
		if _, exists := sounds[soundKey]; !exists {
			return nil, fmt.Errorf("%s %q references unknown sound %q", field, route, soundKey)
		}
		normalized[route] = soundKey
	}
	return normalized, nil
}

func validateCatalogSchoolKeys(catalog *Catalog) error {
	known := make(map[string]struct{}, len(damage.Types())-1)
	for _, school := range damage.Types() {
		if school != damage.Physical {
			known[school.String()] = struct{}{}
		}
	}
	for field, routes := range map[string]map[string]string{
		"school_sounds":           catalog.SchoolSounds,
		"offensive_school_sounds": catalog.OffensiveSchoolSounds,
	} {
		for school := range routes {
			if _, ok := known[school]; !ok {
				return fmt.Errorf("%s contains unknown magic school %q", field, school)
			}
		}
	}
	return nil
}

func sortedSoundKeys(sounds map[string]SoundDefinition) []string {
	keys := make([]string, 0, len(sounds))
	for key := range sounds {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedMusicKeys(tracks map[string]MusicTrackDefinition) []string {
	keys := make([]string, 0, len(tracks))
	for key := range tracks {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func catalogAssetPath(baseDir, relativePath string) (string, error) {
	if filepath.IsAbs(relativePath) {
		return "", fmt.Errorf("file path %q must be relative", relativePath)
	}
	fullPath := filepath.Clean(filepath.Join(baseDir, filepath.FromSlash(relativePath)))
	rel, err := filepath.Rel(baseDir, fullPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("file path %q escapes the catalog directory", relativePath)
	}
	return fullPath, nil
}

func decodeFileF32(path string) ([]byte, int, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, err
	}
	var stream decodedStream
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ogg":
		// Keep the float32 path: Ebitengine recommends NewPlayerF32 for new code,
		// and pre-decoded PCM avoids decoder and disk latency on first combat use.
		stream, err = vorbis.DecodeF32(bytes.NewReader(raw))
	default:
		return nil, 0, fmt.Errorf("sound file must be Ogg/Vorbis, got %q", filepath.Ext(path))
	}
	if err != nil {
		return nil, 0, err
	}
	pcm, err := io.ReadAll(stream)
	if err != nil {
		return nil, 0, err
	}
	return pcm, stream.SampleRate(), nil
}

// VolumeChannel identifies one user-facing volume slider.
type VolumeChannel string

const (
	VolumeMaster VolumeChannel = "master"
	VolumeSFX    VolumeChannel = "sfx"
	VolumeMusic  VolumeChannel = "music"
)

// VolumeSettings are player preferences, separate from authored asset levels.
type VolumeSettings struct {
	Master float64 `json:"master"`
	SFX    float64 `json:"sfx"`
	Music  float64 `json:"music"`
}

func defaultVolumeSettings() VolumeSettings {
	// Product default: every user-facing slider starts at 50%. Master and the
	// selected channel intentionally compose like a conventional mixer.
	return VolumeSettings{Master: 0.5, SFX: 0.5, Music: 0.5}
}

type musicTrack struct {
	definition MusicTrackDefinition
	duration   time.Duration
	players    []musicPlayer
	resume     musicPlayer
}

type musicVoice struct {
	trackKey     string
	player       musicPlayer
	gain         float64
	fadeFrom     float64
	fadeTo       float64
	fadeStarted  time.Time
	fadeDuration time.Duration
	stopAfter    bool
}

type musicPlayer interface {
	Close() error
	IsPlaying() bool
	Pause()
	Play()
	Position() time.Duration
	Rewind() error
	SetVolume(float64)
}

type musicStartMode uint8

const (
	musicStartResume musicStartMode = iota
	musicStartFresh
)

// Manager owns the single Ebiten audio context, reusable short-sound players,
// and the two-voice music crossfader.
type Manager struct {
	mu           sync.Mutex
	context      *audio.Context
	catalog      *Catalog
	pools        map[string][][]*audio.Player
	playerGains  map[*audio.Player]float64
	musicTracks  map[string]*musicTrack
	musicVoices  []*musicVoice
	musicBiome   map[string]string
	desiredMusic string
	volumes      VolumeSettings
	settingsPath string
	lastPlayed   map[string]time.Time
	lastVariant  map[string]int
	random       *rand.Rand
	closed       bool
}

var global *Manager

// LoadGlobal loads the catalog, restores user volume preferences, and installs
// the process-wide sound manager.
func LoadGlobal(path, settingsPath string) (*Manager, error) {
	catalog, err := LoadCatalog(path)
	if err != nil {
		return nil, err
	}
	context := audio.CurrentContext()
	if context == nil {
		context = audio.NewContext(catalog.SampleRate)
	} else if context.SampleRate() != catalog.SampleRate {
		return nil, fmt.Errorf("audio context sample rate is %d, catalog requires %d", context.SampleRate(), catalog.SampleRate)
	}
	volumes := loadVolumeSettings(settingsPath)
	manager := &Manager{
		context:      context,
		catalog:      catalog,
		pools:        make(map[string][][]*audio.Player, len(catalog.Sounds)),
		playerGains:  make(map[*audio.Player]float64),
		musicTracks:  make(map[string]*musicTrack, len(catalog.Music.Tracks)),
		musicBiome:   make(map[string]string),
		volumes:      volumes,
		settingsPath: settingsPath,
		lastPlayed:   make(map[string]time.Time),
		lastVariant:  make(map[string]int),
		random:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	manager.preparePlayers()
	if err := manager.prepareMusicPlayers(); err != nil {
		manager.Close()
		return nil, err
	}
	global = manager
	return manager, nil
}

// preparePlayers creates all short-sound players during startup. Every variant
// gets at least one player, while extra players are shared across variants up
// to the event's total concurrency cap.
func (m *Manager) preparePlayers() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, key := range sortedSoundKeys(m.catalog.Sounds) {
		def := m.catalog.Sounds[key]
		variants := m.catalog.samples[key]
		variantPools := make([][]*audio.Player, len(variants))
		volume := m.soundVolumeLocked(def, 1)
		counts := variantPlayerCounts(len(variants), def.MaxInstances)
		for variant, pcm := range variants {
			players := make([]*audio.Player, 0, counts[variant])
			for range counts[variant] {
				player := m.context.NewPlayerF32FromBytes(pcm)
				player.SetVolume(volume)
				m.playerGains[player] = 1
				players = append(players, player)
			}
			variantPools[variant] = players
		}
		m.pools[key] = variantPools
	}
}

func variantPlayerCounts(variantCount, maxInstances int) []int {
	if variantCount <= 0 {
		return nil
	}
	// A Player owns one immutable PCM source. Keep one ready player for every
	// variant, then add capacity only when the concurrency limit is larger.
	// The global playingCountLocked cap still limits simultaneous playback.
	total := max(variantCount, maxInstances)
	counts := make([]int, variantCount)
	for i := range total {
		counts[i%variantCount]++
	}
	return counts
}

func (m *Manager) prepareMusicPlayers() error {
	for _, key := range sortedMusicKeys(m.catalog.Music.Tracks) {
		def := m.catalog.Music.Tracks[key]
		asset := m.catalog.musicAssets[key]
		track := &musicTrack{definition: def, duration: asset.duration}
		for range 2 {
			stream, err := vorbis.DecodeF32(bytes.NewReader(asset.data))
			if err != nil {
				return fmt.Errorf("prepare music %q: %w", key, err)
			}
			player, err := m.context.NewPlayerF32(stream)
			if err != nil {
				return fmt.Errorf("prepare music player %q: %w", key, err)
			}
			player.SetVolume(0)
			track.players = append(track.players, player)
		}
		m.musicTracks[key] = track
		for _, biome := range def.Biomes {
			m.musicBiome[biome] = key
		}
	}
	return nil
}

func loadVolumeSettings(path string) VolumeSettings {
	settings := defaultVolumeSettings()
	if strings.TrimSpace(path) == "" {
		return settings
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return settings
	}
	var saved VolumeSettings
	if err := json.Unmarshal(raw, &saved); err != nil {
		return settings
	}
	settings.Master = clampVolume(saved.Master)
	settings.SFX = clampVolume(saved.SFX)
	settings.Music = clampVolume(saved.Music)
	return settings
}

func clampVolume(volume float64) float64 {
	if math.IsNaN(volume) || volume < 0 {
		return 0
	}
	if volume > 1 {
		return 1
	}
	return volume
}

func (m *Manager) saveVolumeSettings(settings VolumeSettings) error {
	if strings.TrimSpace(m.settingsPath) == "" {
		return nil
	}
	dir := filepath.Dir(m.settingsPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	tmp, err := os.CreateTemp(dir, ".audio_settings_*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	ok := false
	defer func() {
		_ = tmp.Close()
		if !ok {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return err
	}
	if _, err := tmp.Write(raw); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, m.settingsPath); err != nil {
		return err
	}
	ok = true
	return nil
}

// Volume returns the persisted user volume for one slider.
func (m *Manager) Volume(channel VolumeChannel) float64 {
	if m == nil {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	switch channel {
	case VolumeMaster:
		return m.volumes.Master
	case VolumeSFX:
		return m.volumes.SFX
	case VolumeMusic:
		return m.volumes.Music
	default:
		return 0
	}
}

// SetVolume updates one user slider and applies it to ready players immediately.
// Call SaveVolumes at the end of a keyboard adjustment or pointer drag.
func (m *Manager) SetVolume(channel VolumeChannel, volume float64) bool {
	if m == nil {
		return false
	}
	volume = clampVolume(volume)
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return false
	}
	changed := false
	switch channel {
	case VolumeMaster:
		changed = m.volumes.Master != volume
		m.volumes.Master = volume
	case VolumeSFX:
		changed = m.volumes.SFX != volume
		m.volumes.SFX = volume
	case VolumeMusic:
		changed = m.volumes.Music != volume
		m.volumes.Music = volume
	default:
		m.mu.Unlock()
		return false
	}
	if !changed {
		m.mu.Unlock()
		return false
	}
	m.refreshPlayerVolumesLocked()
	m.mu.Unlock()
	return true
}

// SaveVolumes atomically persists the current user volume settings.
func (m *Manager) SaveVolumes() {
	if m == nil {
		return
	}
	m.mu.Lock()
	settings := m.volumes
	m.mu.Unlock()
	if err := m.saveVolumeSettings(settings); err != nil {
		fmt.Fprintf(os.Stderr, "[sound] failed to save volume settings: %v\n", err)
	}
}

func (m *Manager) soundVolumeLocked(def SoundDefinition, gain float64) float64 {
	// Effects is the user-facing channel for every non-music cue. The catalog's
	// ui/sfx bus remains an authored trim group, not another persisted slider.
	return m.catalog.Buses["master"] * m.catalog.Buses[def.Bus] * def.Volume * m.volumes.Master * m.volumes.SFX * clampVolume(gain)
}

func (m *Manager) musicVolumeLocked(trackKey string, gain float64) float64 {
	track := m.musicTracks[trackKey]
	if track == nil {
		return 0
	}
	return m.catalog.Buses["master"] * m.catalog.Buses["music"] * track.definition.Volume * m.volumes.Master * m.volumes.Music * gain
}

func (m *Manager) refreshPlayerVolumesLocked() {
	for key, variantPools := range m.pools {
		def := m.catalog.Sounds[key]
		for _, players := range variantPools {
			for _, player := range players {
				player.SetVolume(m.soundVolumeLocked(def, m.playerGains[player]))
			}
		}
	}
	for _, voice := range m.musicVoices {
		voice.player.SetVolume(m.musicVolumeLocked(voice.trackKey, voice.gain))
	}
}

// Global returns the loaded manager, or nil in tests/tools that do not load it.
func Global() *Manager {
	return global
}

// ValidateSoundKeys checks the contract between gameplay events and the loaded
// data-driven catalog before the game starts.
func (m *Manager) ValidateSoundKeys(keys []string) error {
	if m == nil {
		return fmt.Errorf("sound manager is unavailable")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return fmt.Errorf("sound manager is closed")
	}
	missing := make([]string, 0)
	for _, key := range keys {
		if _, ok := m.catalog.Sounds[key]; !ok {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("sound catalog missing required gameplay keys: %s", strings.Join(missing, ", "))
	}
	return nil
}

// ValidateSchoolSounds checks that every magic school used by gameplay has a
// catalog route to a validated sound event.
func (m *Manager) ValidateSchoolSounds(schools []string) error {
	if m == nil {
		return fmt.Errorf("sound manager is unavailable")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return fmt.Errorf("sound manager is closed")
	}
	missing := make([]string, 0)
	for _, rawSchool := range schools {
		school := strings.ToLower(strings.TrimSpace(rawSchool))
		if m.catalog.SchoolSounds[school] == "" {
			missing = append(missing, school)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("sound catalog missing magic school routes: %s", strings.Join(missing, ", "))
	}
	return nil
}

// ValidateWeaponCategories checks that every non-magic projectile weapon
// category used by gameplay has an explicit catalog route.
func (m *Manager) ValidateWeaponCategories(categories []string) error {
	if m == nil {
		return fmt.Errorf("sound manager is unavailable")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return fmt.Errorf("sound manager is closed")
	}
	missing := make([]string, 0)
	for _, rawCategory := range categories {
		category := strings.ToLower(strings.TrimSpace(rawCategory))
		if m.catalog.WeaponCategorySounds[category] == "" {
			missing = append(missing, category)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("sound catalog missing ranged weapon category routes: %s", strings.Join(missing, ", "))
	}
	return nil
}

// ValidateMusicBiomes rejects catalog typos while allowing known biomes to
// intentionally have no soundtrack.
func (m *Manager) ValidateMusicBiomes(knownBiomes []string) error {
	if m == nil {
		return fmt.Errorf("sound manager is unavailable")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return fmt.Errorf("sound manager is closed")
	}
	known := make(map[string]struct{}, len(knownBiomes))
	for _, biome := range knownBiomes {
		known[strings.ToLower(strings.TrimSpace(biome))] = struct{}{}
	}
	unknown := make([]string, 0)
	for biome := range m.musicBiome {
		if _, ok := known[biome]; !ok {
			unknown = append(unknown, biome)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return fmt.Errorf("sound catalog references unknown music biomes: %s", strings.Join(unknown, ", "))
	}
	return nil
}

// PlaySchoolWithGain resolves normal or offensive school routing and applies a
// per-play gain used by distance-attenuated world sounds.
func (m *Manager) PlaySchoolWithGain(school string, offensive bool, gain float64) bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return false
	}
	normalized := strings.ToLower(strings.TrimSpace(school))
	key := ""
	if offensive {
		key = m.catalog.OffensiveSchoolSounds[normalized]
	}
	if key == "" {
		key = m.catalog.SchoolSounds[normalized]
	}
	m.mu.Unlock()
	if key == "" {
		return false
	}
	return m.PlayWithGain(key, gain)
}

// PlayWeaponCategoryWithGain resolves an explicitly authored ranged weapon
// category route and applies a per-play gain.
func (m *Manager) PlayWeaponCategoryWithGain(category string, gain float64) bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return false
	}
	key := m.catalog.WeaponCategorySounds[strings.ToLower(strings.TrimSpace(category))]
	m.mu.Unlock()
	if key == "" {
		return false
	}
	return m.PlayWithGain(key, gain)
}

// Play starts one variant of key if its cooldown and concurrency cap allow it.
func (m *Manager) Play(key string) bool {
	return m.PlayWithGain(key, 1)
}

// PlayWithGain starts one variant with an additional per-play gain.
func (m *Manager) PlayWithGain(key string, gain float64) bool {
	if m == nil {
		return false
	}
	gain = clampVolume(gain)
	if gain <= 0 {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return false
	}
	def, ok := m.catalog.Sounds[key]
	if !ok {
		return false
	}
	now := time.Now()
	if last := m.lastPlayed[key]; !last.IsZero() && now.Sub(last) < time.Duration(def.CooldownMS)*time.Millisecond {
		return false
	}
	if m.playingCountLocked(key) >= def.MaxInstances {
		return false
	}
	variantPools := m.pools[key]
	_, player := m.pickIdleVariantPlayerLocked(key, variantPools)
	if player == nil {
		return false
	}
	if err := player.Rewind(); err != nil {
		return false
	}
	m.playerGains[player] = gain
	player.SetVolume(m.soundVolumeLocked(def, gain))
	player.Play()
	m.lastPlayed[key] = now
	return true
}

func (m *Manager) pickIdleVariantPlayerLocked(key string, variantPools [][]*audio.Player) (int, *audio.Player) {
	if len(variantPools) == 0 {
		return 0, nil
	}
	start := m.pickVariantLocked(key, len(variantPools))
	for offset := range len(variantPools) {
		variant := (start + offset) % len(variantPools)
		if player := firstIdlePlayer(variantPools[variant]); player != nil {
			m.lastVariant[key] = variant
			return variant, player
		}
	}
	return 0, nil
}

func (m *Manager) playingCountLocked(key string) int {
	count := 0
	for _, players := range m.pools[key] {
		for _, player := range players {
			if player.IsPlaying() {
				count++
			}
		}
	}
	return count
}

func firstIdlePlayer(players []*audio.Player) *audio.Player {
	for _, player := range players {
		if !player.IsPlaying() {
			return player
		}
	}
	return nil
}

func (m *Manager) pickVariantLocked(key string, count int) int {
	if count <= 1 {
		return 0
	}
	last, seen := m.lastVariant[key]
	pick := m.random.Intn(count)
	if seen && pick == last {
		pick = (pick + 1 + m.random.Intn(count-1)) % count
	}
	return pick
}

// SetMusicBiome crossfades to the track assigned to biome. An unassigned biome
// fades music to silence. Gameplay uses SetMusicState so boss combat can
// temporarily override the location track.
func (m *Manager) SetMusicBiome(biome string) {
	m.SetMusicState(biome, false)
}

// SetMusicState atomically selects the location or boss-combat track. Boss
// music starts fresh for a new encounter; a voice that is still fading out is
// reused so a momentary aggro transition does not restart it. Location tracks
// resume from their saved positions after the boss fight ends.
func (m *Manager) SetMusicState(biome string, bossCombat bool) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return
	}
	trackKey := m.musicBiome[strings.ToLower(strings.TrimSpace(biome))]
	fadeDuration := time.Duration(m.catalog.Music.CrossfadeMS) * time.Millisecond
	startMode := musicStartResume
	if bossCombat && m.catalog.Music.BossTrack != "" {
		trackKey = m.catalog.Music.BossTrack
		fadeDuration = time.Duration(m.catalog.Music.BossCrossfadeMS) * time.Millisecond
		startMode = musicStartFresh
	}
	if trackKey == m.desiredMusic {
		return
	}
	m.desiredMusic = trackKey
	now := time.Now()
	var reused *musicVoice
	var bestRemaining time.Duration
	track := m.musicTracks[trackKey]
	if track != nil {
		for _, voice := range m.musicVoices {
			if voice.trackKey != trackKey || !voice.player.IsPlaying() {
				continue
			}
			remaining := track.duration - voice.player.Position()
			if remaining > bestRemaining {
				reused = voice
				bestRemaining = remaining
			}
		}
	}
	for _, voice := range m.musicVoices {
		if voice == reused {
			continue
		}
		m.fadeMusicVoiceLocked(voice, 0, fadeDuration, true, now)
	}
	if reused != nil {
		m.fadeMusicVoiceLocked(reused, 1, fadeDuration, false, now)
		return
	}
	if trackKey != "" {
		_ = m.startMusicVoiceLocked(trackKey, fadeDuration, now, startMode)
	}
}

func (m *Manager) startMusicVoiceLocked(trackKey string, fadeDuration time.Duration, now time.Time, mode musicStartMode) *musicVoice {
	track := m.musicTracks[trackKey]
	if track == nil {
		return nil
	}
	inUse := func(candidate musicPlayer) bool {
		for _, voice := range m.musicVoices {
			if voice.player == candidate {
				return true
			}
		}
		return false
	}
	var player musicPlayer
	if mode == musicStartResume && track.resume != nil && !inUse(track.resume) {
		player = track.resume
	}
	for _, candidate := range track.players {
		if player == nil && !inUse(candidate) {
			player = candidate
			break
		}
	}
	if player == nil {
		return nil
	}
	player.Pause()
	loopFade := time.Duration(m.catalog.Music.LoopCrossfadeMS) * time.Millisecond
	if mode == musicStartFresh || player.Position() >= track.duration-loopFade {
		if err := player.Rewind(); err != nil {
			return nil
		}
	}
	if track.resume == player {
		track.resume = nil
	}
	voice := &musicVoice{
		trackKey:     trackKey,
		player:       player,
		gain:         0,
		fadeFrom:     0,
		fadeTo:       1,
		fadeStarted:  now,
		fadeDuration: fadeDuration,
	}
	player.SetVolume(0)
	player.Play()
	m.musicVoices = append(m.musicVoices, voice)
	return voice
}

func (m *Manager) fadeMusicVoiceLocked(voice *musicVoice, target float64, duration time.Duration, stopAfter bool, now time.Time) {
	m.advanceMusicVoiceLocked(voice, now)
	voice.fadeFrom = voice.gain
	voice.fadeTo = target
	voice.fadeStarted = now
	voice.fadeDuration = duration
	voice.stopAfter = stopAfter
}

func equalPowerGain(from, to, progress float64) float64 {
	if progress <= 0 {
		return from
	}
	if progress >= 1 {
		return to
	}
	return math.Sqrt((1-progress)*from*from + progress*to*to)
}

func (m *Manager) advanceMusicVoiceLocked(voice *musicVoice, now time.Time) bool {
	if voice.fadeDuration <= 0 {
		return true
	}
	progress := float64(now.Sub(voice.fadeStarted)) / float64(voice.fadeDuration)
	gain := equalPowerGain(voice.fadeFrom, voice.fadeTo, progress)
	if gain != voice.gain {
		voice.gain = gain
		voice.player.SetVolume(m.musicVolumeLocked(voice.trackKey, voice.gain))
	}
	if progress < 1 {
		return false
	}
	voice.fadeDuration = 0
	voice.fadeFrom = voice.fadeTo
	return true
}

func (m *Manager) beginLoopCrossfadeLocked(now time.Time) {
	if m.desiredMusic == "" {
		return
	}
	var current *musicVoice
	for _, voice := range m.musicVoices {
		if voice.trackKey == m.desiredMusic && !voice.stopAfter {
			current = voice
			break
		}
	}
	if current == nil {
		_ = m.startMusicVoiceLocked(m.desiredMusic, time.Duration(m.catalog.Music.CrossfadeMS)*time.Millisecond, now, musicStartFresh)
		return
	}
	track := m.musicTracks[m.desiredMusic]
	loopFade := time.Duration(m.catalog.Music.LoopCrossfadeMS) * time.Millisecond
	if track == nil || track.duration <= loopFade || current.player.Position() < track.duration-loopFade {
		return
	}
	if m.startMusicVoiceLocked(m.desiredMusic, loopFade, now, musicStartFresh) == nil {
		return
	}
	m.fadeMusicVoiceLocked(current, 0, loopFade, true, now)
}

// Update advances music envelopes and starts the standby player before a track
// reaches its end. Short SFX players remain pooled and need no per-tick work.
func (m *Manager) Update() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return
	}
	now := time.Now()
	kept := m.musicVoices[:0]
	for _, voice := range m.musicVoices {
		fadeComplete := m.advanceMusicVoiceLocked(voice, now)
		if fadeComplete && voice.stopAfter {
			voice.player.Pause()
			if voice.trackKey != m.desiredMusic {
				m.rememberMusicResumeLocked(voice)
			}
			continue
		}
		kept = append(kept, voice)
	}
	m.musicVoices = kept
	m.beginLoopCrossfadeLocked(now)
}

func (m *Manager) rememberMusicResumeLocked(voice *musicVoice) {
	track := m.musicTracks[voice.trackKey]
	if track == nil || voice.player == nil {
		return
	}
	candidateRemaining := musicResumeRemaining(track, voice.player, m.catalog.Music.LoopCrossfadeMS)
	savedRemaining := musicResumeRemaining(track, track.resume, m.catalog.Music.LoopCrossfadeMS)
	if track.resume == nil || candidateRemaining > savedRemaining {
		track.resume = voice.player
	}
}

func musicResumeRemaining(track *musicTrack, player musicPlayer, loopCrossfadeMS int) time.Duration {
	if track == nil || player == nil {
		return 0
	}
	position := player.Position()
	loopFade := time.Duration(loopCrossfadeMS) * time.Millisecond
	if position < 0 || position >= track.duration-loopFade {
		return 0
	}
	return track.duration - position
}

// Close releases every pooled player. The process-wide context is owned by
// Ebiten and intentionally remains alive until process exit.
func (m *Manager) Close() {
	if m == nil {
		return
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	for _, variantPools := range m.pools {
		for _, players := range variantPools {
			for _, player := range players {
				_ = player.Close()
			}
		}
	}
	for _, track := range m.musicTracks {
		for _, player := range track.players {
			_ = player.Close()
		}
	}
	m.pools = nil
	m.playerGains = nil
	m.musicTracks = nil
	m.musicVoices = nil
	m.closed = true
	if global == m {
		global = nil
	}
	settings := m.volumes
	m.mu.Unlock()
	if err := m.saveVolumeSettings(settings); err != nil {
		fmt.Fprintf(os.Stderr, "[sound] failed to save volume settings on close: %v\n", err)
	}
}
