package game

import (
	"github.com/hajimehoshi/ebiten/v2"
	"image"
	"math"
)

// One 256 KiB seamless field replaces repeated fragment-stage fractal noise.
// It is generated once, shared by every flame, and never read back from the GPU.
func (r *Renderer) ensureFireNoise() *ebiten.Image {
	if r.fireNoise != nil {
		return r.fireNoise
	}
	const size = 256
	im := image.NewRGBA(image.Rect(0, 0, size, size))
	noise := func(x, y float64, period int) float64 {
		ix, iy := int(math.Floor(x)), int(math.Floor(y))
		fx, fy := x-float64(ix), y-float64(iy)
		fx, fy = fx*fx*(3-2*fx), fy*fy*(3-2*fy)
		hash := func(x, y int) float64 {
			v := uint32((x%period+period)%period)*374761393 + uint32((y%period+period)%period)*668265263 + 7919
			v = (v ^ (v >> 13)) * 1274126177
			v ^= v >> 16
			return float64(v&65535) / 65535
		}
		a, b, c, d := hash(ix, iy), hash(ix+1, iy), hash(ix, iy+1), hash(ix+1, iy+1)
		return (a+(b-a)*fx)*(1-fy) + (c+(d-c)*fx)*fy
	}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			u, v := float64(x)*8/size, float64(y)*8/size
			base := noise(u, v, 8)
			detail := base*.56 + noise(u*2+13, v*2+9, 16)*.29 + noise(u*4+7, v*4+19, 32)*.15
			i := (y*size + x) * 4
			im.Pix[i], im.Pix[i+1], im.Pix[i+2], im.Pix[i+3] = uint8(base*255), uint8(detail*255), 0, 255
		}
	}
	r.fireNoise = ebiten.NewImageFromImage(im)
	return r.fireNoise
}
