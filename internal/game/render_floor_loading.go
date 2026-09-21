package game

import (
	"context"
	"image"
	"path/filepath"

	"ugataima/internal/graphics"
	"ugataima/internal/storage"

	"github.com/hajimehoshi/ebiten/v2"
)

type preparedFloor struct {
	pixels                    *image.RGBA
	groups                    map[string]floorTextureGroup
	count, width, height, mip int
}

type floorPreparation struct {
	key      string
	cancel   context.CancelFunc
	result   <-chan preparedFloor
	prepared *preparedFloor
	image    *ebiten.Image
	row      int
}

func (r *Renderer) cancelFloorPreparation() {
	if p := r.floorPreparation; p != nil {
		p.cancel()
		if p.image != nil {
			p.image.Deallocate()
		}
		r.floorPreparation = nil
	}
}

func (r *Renderer) startFloorPreparation(key string, groups map[string][]string) {
	if r.floorPreparation != nil && r.floorPreparation.key == key {
		return
	}
	r.cancelFloorPreparation()
	// Capture paths and data on the owner. The worker never reads the mutable
	// world manager, renderer, or a later test/application working directory.
	paths := make(map[string][]string, len(groups))
	for group, names := range groups {
		for _, name := range names {
			path, err := filepath.Abs(resolveNamedPNG("assets/sprites/floor", name))
			if err == nil {
				paths[group] = append(paths[group], path)
			}
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan preparedFloor, 1)
	cache := graphics.PixelCache{Dir: storage.RenderCacheDir()}
	r.floorPreparation = &floorPreparation{key: key, cancel: cancel, result: result}
	go func() {
		defer close(result)
		defer cache.Prune()
		if ctx.Err() != nil {
			return
		}
		textures, mapping := prepareFloorTextureGroups(paths)
		if ctx.Err() != nil {
			return
		}
		pixels, width, height, mip := prepareCachedFloorAtlas(ctx, cache, textures)
		select {
		case result <- preparedFloor{pixels: pixels, groups: mapping, count: len(textures), width: width, height: height, mip: mip}:
		case <-ctx.Done():
		}
	}()
}

func (r *Renderer) advanceFloorPreparation(maxBytes int) {
	p := r.floorPreparation
	if p == nil {
		return
	}
	if p.prepared == nil {
		select {
		case prepared, ok := <-p.result:
			if !ok {
				r.cancelFloorPreparation()
				return
			}
			p.prepared = &prepared
			if prepared.pixels != nil {
				p.image = ebiten.NewImage(prepared.pixels.Bounds().Dx(), prepared.pixels.Bounds().Dy())
			}
		default:
			return
		}
	}
	prepared := p.prepared
	if prepared.pixels != nil {
		cpu := prepared.pixels
		rows := min(cpu.Bounds().Dy()-p.row, max(1, maxBytes/cpu.Stride))
		graphics.WritePixelsRegion(p.image, image.Rect(0, p.row, cpu.Bounds().Dx(), p.row+rows), cpu.Pix[p.row*cpu.Stride:(p.row+rows)*cpu.Stride])
		p.row += rows
		if p.row < cpu.Bounds().Dy() {
			return
		}
	}
	if r.floorTexAtlas != nil {
		r.floorTexAtlas.Deallocate()
	}
	r.floorTexAtlas, p.image = p.image, nil
	r.floorTexGroups, r.floorTexturesKey = prepared.groups, p.key
	r.floorTexCount, r.floorTexTileW, r.floorTexTileH, r.floorTexMaxMip = prepared.count, prepared.width, prepared.height, prepared.mip
	r.buildFloorColorMap(r.game.world.Width, r.game.world.Height)
	if gl := r.game.gameLoop; gl != nil && gl.loading != nil && r.floorTexAtlas != nil {
		gl.loading.uploads = append(gl.loading.uploads, r.floorTexAtlas)
	}
	r.cancelFloorPreparation()
}
