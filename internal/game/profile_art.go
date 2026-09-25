package game

import (
	"image"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	xdraw "golang.org/x/image/draw"
)

// Profile panoramas are tiny independent thumbnails. Decoding happens on one
// worker, never in Draw; browsing lifetime records does not fill the world's
// GPU panorama residency cache with every region the player has visited.
type profileArt struct {
	requested map[string]bool
	images    map[string]*ebiten.Image
	requests  chan string
	results   chan profileArtResult
	stop      chan struct{}
	wg        sync.WaitGroup
}
type profileArtResult struct {
	key    string
	pixels *image.RGBA
}

func newProfileArt() *profileArt {
	a := &profileArt{requested: map[string]bool{}, images: map[string]*ebiten.Image{}, requests: make(chan string, 32), results: make(chan profileArtResult, 8), stop: make(chan struct{})}
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		for {
			select {
			case <-a.stop:
				return
			case name := <-a.requests:
				result := profileArtResult{key: name}
				texture := name
				if variant := skyVariantName(name, false); skyTextureExists(variant) {
					texture = variant
				}
				if src, err := decodePNG(resolveNamedPNG("assets/sprites/sky", texture)); err == nil {
					b := src.Bounds()
					size := min(b.Dx(), b.Dy())
					cx, cy := b.Min.X+b.Dx()/2, b.Min.Y+b.Dy()/2
					crop := image.Rect(cx-size/2, cy-size/2, cx-size/2+size, cy-size/2+size)
					result.pixels = image.NewRGBA(image.Rect(0, 0, 128, 128))
					xdraw.CatmullRom.Scale(result.pixels, result.pixels.Bounds(), src, crop, xdraw.Src, nil)
				}
				select {
				case a.results <- result:
				case <-a.stop:
					return
				}
			}
		}
	}()
	return a
}

func (a *profileArt) thumbnail(key string) *ebiten.Image {
	for {
		select {
		case result := <-a.results:
			if result.pixels != nil {
				a.images[result.key] = ebiten.NewImageFromImage(result.pixels)
			}
		default:
			if !a.requested[key] {
				select {
				case a.requests <- key:
					a.requested[key] = true
				default:
				}
			}
			return a.images[key]
		}
	}
}
func (a *profileArt) close() {
	if a == nil {
		return
	}
	close(a.stop)
	a.wg.Wait()
	for _, img := range a.images {
		img.Deallocate()
	}
}
