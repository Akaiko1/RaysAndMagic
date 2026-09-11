package game

import (
	"context"
	"fmt"
	"image"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/graphics"
)

func TestRenderRegistryAllocationAndDependencyOwnership(t *testing.T) {
	for _, kind := range []renderResourceKind{renderResourceProcessed, renderResourceStandee, renderResourceWall, renderResourceSky} {
		t.Run(fmt.Sprintf("kind=%d", kind), func(t *testing.T) {
			reg := newRenderResourceRegistry()
			released := map[*ebiten.Image]int{}
			reg.release = func(img *ebiten.Image) { released[img]++; img.Deallocate() }
			source, derived := ebiten.NewImage(8, 8), ebiten.NewImage(4, 4)
			root := sourceResourceID(mapRenderSourceKey{name: "shared"})
			child := renderResourceID{kind: kind, name: "child"}
			reg.put(root, []*ebiten.Image{source}, nil, nil)
			// Repeating an alias within and across records must not inflate accounting
			// or release it while another retained record still owns it.
			reg.put(child, []*ebiten.Image{source, derived, derived}, []renderResourceID{root}, nil)
			ownerA, ownerB := renderResourceOwner{region: "a"}, renderResourceOwner{region: "b"}
			reg.retain(root, ownerA)
			reg.retain(child, ownerB)
			if got := reg.estimatedGPUBytes(); got != (8*8+4*4)*4 {
				t.Fatalf("alias-inflated GPU bytes: %d", got)
			}
			delete(reg.records[root].owners, ownerA)
			reg.releaseResources(map[renderResourceID]struct{}{root: {}})
			if len(released) != 0 {
				t.Fatal("partial region release freed a dependency still in use")
			}
			delete(reg.records[child].owners, ownerB)
			reg.releaseResources(map[renderResourceID]struct{}{root: {}})
			reg.releaseResources(map[renderResourceID]struct{}{root: {}, child: {}})
			if len(reg.records) != 0 || reg.estimatedGPUBytes() != 0 || released[source] != 1 || released[derived] != 1 {
				t.Fatalf("final release records=%d bytes=%d release=%v", len(reg.records), reg.estimatedGPUBytes(), released)
			}
		})
	}
}

func TestRenderRegistryRetainsVisibleSkiesUntilFadeEnds(t *testing.T) {
	current, previous, unused := ebiten.NewImage(4, 4), ebiten.NewImage(4, 4), ebiten.NewImage(4, 4)
	r := &Renderer{game: &MMGame{skyPanorama: current, skyPanoramaPrev: previous, skyPanoramaCache: map[string]*ebiten.Image{"day": current, "night": previous, "other": unused}}}
	r.deallocateUnusedSkyPanoramas(nil)
	if len(r.game.skyPanoramaCache) != 2 {
		t.Fatal("current or fading sky lost its owner")
	}
	r.game.skyPanoramaPrev = nil
	r.deallocateUnusedSkyPanoramas(nil)
	if len(r.game.skyPanoramaCache) != 1 || r.game.skyPanoramaCache["day"] != current {
		t.Fatal("fade completion failed to release previous sky")
	}
	r.game.skyPanorama = nil
	r.deallocateUnusedSkyPanoramas(nil)
	if len(r.game.skyPanoramaCache) != 0 || r.mapRenderRegistry.estimatedGPUBytes() != 0 {
		t.Fatal("sky ownership leaked")
	}
}

func TestRenderTaskRejectsObsoleteGenerationAtPublicationAndSubmission(t *testing.T) {
	for _, invalid := range []string{"cancel", "generation", "world"} {
		for _, phase := range []string{"before_commit", "partial_commit", "queued_upload"} {
			t.Run(invalid+"/"+phase, func(t *testing.T) {
				cfg := loadTestConfig(t)
				g := newTestGame(cfg, newTestWorldSized(cfg, 8, 8))
				g.sprites = graphics.NewSpriteManager()
				r := &Renderer{game: g}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				task := &mapRenderPrewarmTask{mapKey: "same-map", generation: r.mapRenderGeneration, world: g.world, ctx: ctx, cancel: cancel, cpuImages: make(map[*ebiten.Image]*image.RGBA)}
				task.prewarmer = newMapRenderPrewarmer(r, task)
				request := graphics.SpriteResourceRequest{Name: "test-late"}
				prepared := make(chan graphics.PreparedSpriteResource, 1)
				prepared <- graphics.PreparedSpriteResource{Request: request, CPU: image.NewRGBA(image.Rect(0, 0, 1024, 1024)), Found: true}
				close(prepared)
				task.preparedSprites = prepared
				if phase != "before_commit" {
					r.drainPreparedMapRenderResource(task)
				}
				if phase == "partial_commit" {
					task.spriteCommit.Advance(4)
				}
				upload := ebiten.NewImage(8, 8)
				defer upload.Deallocate()
				if phase == "queued_upload" {
					r.queueMapRenderUpload(upload, task)
				}
				switch invalid {
				case "cancel":
					r.cancelMapRenderPrewarmTask(task)
				case "generation":
					r.mapRenderGeneration++
				case "world":
					g.world = newTestWorldSized(cfg, 8, 8)
				}
				for i := 0; i < 64; i++ {
					r.drainPreparedMapRenderResource(task)
				}
				r.cancelMapRenderPrewarmTask(task)
				// Repeated cancellation must not affect a replacement of the same map.
				replacement := &mapRenderPrewarmTask{mapKey: task.mapKey, generation: r.mapRenderGeneration, world: g.world}
				next := ebiten.NewImage(2, 2)
				defer next.Deallocate()
				r.queueMapRenderUpload(next, replacement)
				r.cancelMapRenderPrewarmTask(task)
				if len(g.sprites.ResourceImages(request)) != 0 {
					t.Fatal("obsolete decode published a source")
				}
				if len(r.mapRenderUploadQueue) != 1 || r.mapRenderUploadQueue[0].task != replacement {
					t.Fatal("obsolete task retained uploads or cancelled its replacement")
				}
				if task.spriteCommit != nil || task.preparedSprites != nil {
					t.Fatal("cancel retained private commit or decoded queue")
				}
			})
		}
	}
}

func TestRenderResourceTravelSoakPlateaus(t *testing.T) {
	sprites := graphics.NewSpriteManager()
	r := &Renderer{game: &MMGame{sprites: sprites}, processedSpriteCache: make(map[processedSpriteKey]*ebiten.Image)}
	peak := int64(0)
	start := time.Now()
	// Two neighboring regions share one source. Revisit, drop the first region,
	// then teleport away. The source and derivative must disappear only at the
	// final release and the next visit must start with no retained allocations.
	for visit := 0; visit < 40; visit++ {
		req := graphics.SpriteResourceRequest{Name: "travel-source"}
		sprites.CommitPreparedResource(graphics.PreparedSpriteResource{Request: req, CPU: image.NewRGBA(image.Rect(0, 0, 32, 32)), Found: true})
		source := sprites.ResourceImages(req)[0]
		key := makeStandeeCoreKey("travel", source, true)
		prepared := prepareStandeePixels(image.NewRGBA(image.Rect(0, 0, 32, 32)), 0, false)
		r.commitPreparedStandeePixels(key, source, prepared)
		for _, region := range []string{"outdoor", "dungeon"} {
			manifest := testMapRenderResources(key)
			manifest.sources[mapRenderSourceKey{name: req.Name}] = struct{}{}
			r.commitMapRenderResidency(region, manifest)
		}
		bytes := r.mapRenderRegistry.estimatedGPUBytes()
		if visit == 0 {
			peak = bytes
		} else if bytes != peak {
			t.Fatalf("visit %d retained %d bytes, first visit %d", visit, bytes, peak)
		}
		r.evictMapRenderResidencyOutside(map[string]struct{}{"dungeon": {}})
		if len(sprites.ResourceImages(req)) != 1 {
			t.Fatal("border crossing evicted a shared source")
		}
		r.evictMapRenderResidencyOutside(nil)
		if r.mapRenderRegistry.estimatedGPUBytes() != 0 || len(r.mapRenderRegistry.records) != 0 || len(sprites.ResourceImages(req)) != 0 {
			t.Fatalf("visit %d leaked allocation/cache ownership", visit)
		}
	}
	t.Logf("40 visits: estimated peak GPU pixels=%d bytes, final=0, elapsed=%s", peak, time.Since(start))
}

func TestRenderUploadSubmissionRejectsOldGeneration(t *testing.T) {
	r := &Renderer{}
	task := &mapRenderPrewarmTask{mapKey: "same"}
	img := ebiten.NewImage(4, 4)
	defer img.Deallocate()
	r.queueMapRenderUpload(img, task)
	r.mapRenderGeneration++
	dst := &recordingMapRenderUploadDestination{}
	r.submitMapRenderPrewarmUploads(dst)
	if len(dst.images) != 0 || len(r.mapRenderUploadQueue) != 0 {
		t.Fatal("obsolete generation reached Draw submission")
	}
}

func TestRenderRegistryPendingCommitRetainsLazyWinner(t *testing.T) {
	sm := graphics.NewSpriteManager()
	request := graphics.SpriteResourceRequest{Name: "pending"}
	prepared := graphics.PreparedSpriteResource{Request: request, CPU: image.NewRGBA(image.Rect(0, 0, 8, 8)), Found: true}
	sm.CommitPreparedResource(prepared)
	source := sm.ResourceImages(request)[0]
	r := &Renderer{game: &MMGame{sprites: sm}}
	task := &mapRenderPrewarmTask{mapKey: "next", spriteCommit: sm.BeginPreparedResourceCommit(prepared), spriteCommitReq: request}
	task.prewarmer = newMapRenderPrewarmer(r, task)
	r.mapRenderResourcePrewarmActive = task
	old := testMapRenderResources()
	old.sources[mapRenderSourceKey{name: request.Name}] = struct{}{}
	r.deallocateMapRenderRegion(old, nil)
	if got := sm.ResourceImages(request); len(got) != 1 || got[0] != source {
		t.Fatal("pending lazy winner lost source ownership during region release")
	}
	r.cancelMapRenderPrewarmTask(task)
	r.mapRenderResourcePrewarmActive = nil
	r.deallocateMapRenderRegion(old, nil)
	if len(sm.ResourceImages(request)) != 0 {
		t.Fatal("cancelled pending commit kept source alive")
	}
}
