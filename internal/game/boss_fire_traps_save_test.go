package game

import (
	"testing"

	"ugataima/internal/world"
)

func TestBossFireTrapSaveCoordinatesRoundTripWorldModes(t *testing.T) {
	t.Chdir("../..")

	const mapKey = "dragon_cliffs"
	const localTX, localTY = 12, 8

	unifiedGame, unifiedWM, _ := bootOpenWorldGame(t, true)
	startX, startY, ok := unifiedWM.OpenWorldRegionStart(mapKey)
	if !ok {
		t.Fatalf("%s region has no start", mapKey)
	}
	unifiedGame.camera.X, unifiedGame.camera.Y = startX, startY
	unifiedGame.syncOpenWorldRegion()

	worldTX, worldTY := unifiedWM.ProjectTile(mapKey, localTX, localTY)
	unifiedGame.bossFireTraps = []bossFireTrap{{
		MapKey: mapKey,
		TX:     worldTX,
		TY:     worldTY,
	}}

	assertTrap := func(label string, traps []bossFireTrap, wantTX, wantTY int) {
		t.Helper()
		if len(traps) != 1 {
			t.Fatalf("%s trap count = %d, want 1", label, len(traps))
		}
		if got := traps[0]; got.MapKey != mapKey || got.TX != wantTX || got.TY != wantTY {
			t.Fatalf("%s trap = %+v, want map=%s tile=(%d,%d)",
				label, got, mapKey, wantTX, wantTY)
		}
	}

	save := unifiedGame.buildSave(unifiedWM)
	assertTrap("unified save", save.BossFireTraps, localTX, localTY)

	if err := unifiedGame.applySave(unifiedWM, &save); err != nil {
		t.Fatalf("unified reload: %v", err)
	}
	assertTrap("unified reload", unifiedGame.bossFireTraps, worldTX, worldTY)

	// The immediately preceding save format had no map_key and stored runtime
	// coordinates. A unified-world load must not project those coordinates a
	// second time.
	legacyUnifiedSave := save
	legacyUnifiedSave.BossFireTraps = []bossFireTrap{{TX: worldTX, TY: worldTY}}
	if err := unifiedGame.applySave(unifiedWM, &legacyUnifiedSave); err != nil {
		t.Fatalf("legacy unified reload: %v", err)
	}
	assertTrap("legacy unified reload", unifiedGame.bossFireTraps, worldTX, worldTY)

	splitGame, splitWM, _ := bootOpenWorldGame(t, false)
	if err := splitGame.applySave(splitWM, &save); err != nil {
		t.Fatalf("split load: %v", err)
	}
	assertTrap("split load", splitGame.bossFireTraps, localTX, localTY)

	legacySplitSave := save
	legacySplitSave.BossFireTraps = []bossFireTrap{{TX: localTX, TY: localTY}}
	if err := splitGame.applySave(splitWM, &legacySplitSave); err != nil {
		t.Fatalf("legacy split reload: %v", err)
	}
	assertTrap("legacy split reload", splitGame.bossFireTraps, localTX, localTY)

	splitSave := splitGame.buildSave(splitWM)
	assertTrap("split save", splitSave.BossFireTraps, localTX, localTY)

	world.GlobalWorldManager = unifiedWM
	if err := unifiedGame.applySave(unifiedWM, &splitSave); err != nil {
		t.Fatalf("unified load of split save: %v", err)
	}
	assertTrap("unified load of split save", unifiedGame.bossFireTraps, worldTX, worldTY)
}
