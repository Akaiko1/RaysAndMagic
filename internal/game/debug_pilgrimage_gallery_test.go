//go:build debug

package game

import (
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/character"
	"ugataima/internal/quests"
	"ugataima/internal/storage"
)

// Opt-in native Draw evidence for dialogue, all idle frames, journal rewards
// and their real hover tooltip at the normal and resized window dimensions.
func TestDebugSim_PilgrimagePresentation(t *testing.T) {
	requireStandeeGPU(t)
	out := os.Getenv("RAM_PILGRIMAGE_QA_DIR")
	if out == "" {
		t.Skip("set RAM_PILGRIMAGE_QA_DIR")
	}
	storage.SetDataRootForTesting(t.TempDir())
	defer storage.SetDataRootForTesting("")
	g := bootGameplayPreviewGame(t)
	defer g.Shutdown()
	if err := g.ApplyTestScenario("assets/test_scenarios.yaml", "pilgrimage"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	save := func(name string, shot image.Image) {
		t.Helper()
		f, err := os.Create(filepath.Join(out, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, shot)
		closeErr := f.Close()
		if err != nil || closeErr != nil {
			t.Fatalf("write %v %v", err, closeErr)
		}
	}
	var mira *character.NPC
	for _, npc := range g.world.NPCs {
		if npc.Key == "sister_mira" {
			mira = npc
			break
		}
	}
	if mira == nil {
		t.Fatal("Mira missing")
	}
	g.setPartyPosition(mira.X, mira.Y+2*g.config.GetTileSize())
	g.snapFacing(-math.Pi / 2)
	for _, size := range [][2]int{{1920, 1080}, {1024, 768}} {
		w, h := g.gameLoop.Layout(size[0], size[1])
		captureGameplayPreviewFrame(t, g, w, h)
		runOnDrawFrame(func(*ebiten.Image) { (&InputHandler{game: g}).openNPCInteraction(mira) })
		save(fmt.Sprintf("mira_dialogue_%dx%d", w, h), captureGameplayPreviewFrame(t, g, w, h))
		runOnDrawFrame(func(*ebiten.Image) { g.closeConversation() })
	}
	w, h := g.gameLoop.Layout(1920, 1080)
	captureGameplayPreviewFrame(t, g, w, h)
	for i := 0; i < 4; i++ {
		runOnDrawFrame(func(*ebiten.Image) {
			g.frameCount = int64(i * animationTicksPerFrame(g.config.GetTPS(), NPCIdleAnimationFPS))
		})
		save(fmt.Sprintf("mira_idle_%d", i+1), captureGameplayPreviewFrame(t, g, w, h))
	}
	runOnDrawFrame(func(*ebiten.Image) {
		for _, id := range []string{"unbroken_mountain", "unbroken_desert", "unbroken_jungle"} {
			g.activateQuest(id)
			q := g.questManager.GetQuest(id)
			q.CurrentCount = q.Target()
			q.Completed = true
			q.Status = quests.QuestStatusCompleted
		}
		g.menuOpen = true
		g.currentTab = TabQuests
		g.gameLoop.ui.questPage = 0
	})
	pointer := installFakePointer(t)
	for _, size := range [][2]int{{1920, 1080}, {1024, 768}} {
		w, h := g.gameLoop.Layout(size[0], size[1])
		pointer.moveTo(0, 0)
		save(fmt.Sprintf("journal_%dx%d", w, h), captureGameplayPreviewFrame(t, g, w, h))
		content := computeTabbedMenuLayout(w, gameplayViewportBottom(g)).content
		layout := computeQuestContentLayout(content, nil, 0)
		all := g.questManager.GetAllQuests()
		sortQuestJournal(all)
		copies := make([]questCardCopy, len(all))
		for i, q := range all {
			copies[i] = questCardCopyForQuest(q, layout.cardW, layout.maxDescRows)
		}
		layout = computeQuestContentLayout(content, copies, 0)
		icon := questRewardIconRect(layout.rows[0], copies[0], 0)
		pointer.moveTo(icon.x+icon.w/2, icon.y+icon.h/2)
		save(fmt.Sprintf("journal_tooltip_%dx%d", w, h), captureGameplayPreviewFrame(t, g, w, h))
		if g.gameLoop.ui.tooltipIcon == "" {
			t.Fatal("journal tooltip absent in native Draw")
		}
	}
}
