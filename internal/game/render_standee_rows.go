package game

import (
	"encoding/binary"
	"image"
)

// A token is drawn as strips that span its art from top to bottom, yet most
// art leaves much of that frame transparent (a tree's corners, the air beside
// its trunk). The strips are cut down to the rows their art can colour, so the
// shaders never run where they would output nothing. Nothing the art colours
// is lost; under Panini the extra cuts can move a texel edge by one pixel
// column, as any vertex on its piecewise-linear projection does.

// standeeRowBands is how many column bands a token's art is cut into;
// standeeRowGroup is how many of them one summary entry spans, so a wide
// lookup reads a handful of entries.
const (
	standeeRowBands = 64
	standeeRowGroup = 8
)

// A strip at least standeeTrimMinColumns wide is cut into pieces that follow
// the art's outline: at most standeeTrimMaxPieces, each at least
// standeeTrimPieceColumns wide; a narrower strip has too few pixels to repay
// the lookup. They set how closely the mesh hugs the outline against the
// vertices it costs, never what is drawn. standeeTrimMargin is the slack kept around the outline, in
// screen pixels: it covers the anisotropic taps (under half a pixel), pixel
// rounding and Panini's piecewise-linear edges.
const (
	standeeTrimMinColumns   = 16
	standeeTrimPieceColumns = 24
	standeeTrimMaxPieces    = 16
	standeeTrimMargin       = 2.0
)

// standeeTapReach is how far the outermost anisotropic tap of a minified
// sticker sits from its sample, in footprints less a texel and a half
// (nearStickerAt in standee_sampling.kageinc, sheetNearSticker).
const standeeTapReach = 0.375

// standeeLevelRows bounds what one mip level of a token's art can colour: for
// each column band, the stretch of v (0 at the art's top, 1 at its bottom) in
// which a sample reads a texel that is not fully transparent. The reach of a
// bilinear fetch is included. top > bottom marks a band nothing reads.
type standeeLevelRows struct {
	top, bottom           [standeeRowBands]float32
	groupTop, groupBottom [standeeRowBands / standeeRowGroup]float32
	// Level 0 only: the walls where the solid art of its rows starts (0) and
	// ends (1), and the core colour they show, straight RGB; endTone[3] is 0
	// until the tone is known.
	endWalls [2][]standeeEndWall
	endTone  [4]float32
}

// standeeEndWall is where a stretch of the art's rows, v0..v1 (0 at its top,
// 1 at its bottom), starts or ends: u across the art (0..1), the outermost of
// their own ends. Seen along its plane a slab shows these walls.
type standeeEndWall struct{ v0, v1, u float32 }

// Rows whose ends lie within 1/standeeEndWallGroup of the art's width of the
// first one's share one wall.
const standeeEndWallGroup = 64

// standeeCoreMinAlpha is the alpha from which a texel is solid art: the core
// fills it (prepareStandeePixels) and its tone averages it (standeeCoreTone).
const standeeCoreMinAlpha = 24

// standeeEndWallMinAlpha is the alpha from which a texel bounds an end wall:
// the art's half-coverage edge. A wall is opaque, so one at a fainter fringe
// would hide what the fringe lets through.
const standeeEndWallMinAlpha = 128

// cover widens lo..hi to the rows of bands first..last.
func (r *standeeLevelRows) cover(first, last int, lo, hi float32) (float32, float32) {
	for band := first; band <= last; {
		if band%standeeRowGroup == 0 && band+standeeRowGroup-1 <= last {
			group := band / standeeRowGroup
			lo, hi = min(lo, r.groupTop[group]), max(hi, r.groupBottom[group])
			band += standeeRowGroup
			continue
		}
		lo, hi = min(lo, r.top[band]), max(hi, r.bottom[band])
		band++
	}
	return lo, hi
}

// standeeArtRows measures every level of a token's mip chain. Only level 0 is
// read: a reduced texel colours only where its footprint in the level above
// does (downsampleMip averages), so each level's columns follow from the
// level above and never miss a texel. A token's core colours only where its
// sticker does (prepareStandeePixels), so the sticker's rows bound both chains.
func standeeArtRows(levels []*image.RGBA) []standeeLevelRows {
	if len(levels) == 0 || levels[0] == nil {
		return nil
	}
	base := levels[0]
	b := base.Bounds()
	w0, h0 := b.Dx(), b.Dy()
	if w0 < 2 || h0 < 2 {
		return nil
	}
	// The first and last coloured row of each column, -1 for none.
	first, last := make([]int, w0), make([]int, w0)
	for x := range first {
		first[x], last[x] = -1, -1
	}
	// Each row's first and last column at the wall's alpha, -1 for none.
	solidFirst, solidLast := make([]int, h0), make([]int, h0)
	for y := 0; y < h0; y++ {
		row := base.Pix[base.PixOffset(b.Min.X, b.Min.Y+y):]
		solidFirst[y], solidLast[y] = -1, -1
		for x := 0; x < w0; x++ {
			if binary.LittleEndian.Uint32(row[x*4:]) == 0 {
				continue
			}
			if row[x*4+3] >= standeeEndWallMinAlpha {
				if solidFirst[y] < 0 {
					solidFirst[y] = x
				}
				solidLast[y] = x
			}
			if first[x] < 0 {
				first[x] = y
			}
			last[x] = y
		}
	}
	rows := make([]standeeLevelRows, len(levels))
	group := max(1, w0/standeeEndWallGroup)
	rows[0].endWalls = [2][]standeeEndWall{
		standeeEndWalls(solidFirst, w0, group, false),
		standeeEndWalls(solidLast, w0, group, true),
	}
	w, h := w0, h0
	for k := range rows {
		if k > 0 {
			if levels[k] == nil {
				return nil
			}
			size := levels[k].Bounds().Size()
			first, last = reduceStandeeColumnRows(first, last, w, h, size.X, size.Y)
			w, h = size.X, size.Y
		}
		rows[k].measure(first, last, h, w0, h0)
	}
	return rows
}

// standeeEndWalls groups rows into walls: consecutive rows with solid art
// whose ends lie within group texels of the first row's share one wall, at the
// outermost of their ends. ends holds each row's first (last: its last) solid
// column, -1 for none. The faces show texel x around u = x/w0 and row y around
// v = y/(h0-1), the last ones held to the edge (standeeSourcePosition), so a
// wall stands half a texel out from its end texel.
func standeeEndWalls(ends []int, w0, group int, last bool) []standeeEndWall {
	var walls []standeeEndWall
	h0 := len(ends)
	at := func(texelEdge float64, n int) float32 {
		return float32(min(max(texelEdge/float64(n), 0), 1))
	}
	start, ref, outer := -1, 0, 0
	flush := func(y int) {
		u := at(float64(outer)-0.5, w0)
		if last {
			u = at(float64(outer)+0.5, w0)
			if outer == w0-1 {
				u = 1
			}
		}
		walls = append(walls, standeeEndWall{at(float64(start)-0.5, h0-1), at(float64(y-1)+0.5, h0-1), u})
		start = -1
	}
	for y, end := range ends {
		if start >= 0 && (end < 0 || end > ref+group || end < ref-group) {
			flush(y)
		}
		switch {
		case end < 0:
		case start < 0:
			start, ref, outer = y, end, end
		case last:
			outer = max(outer, end)
		default:
			outer = min(outer, end)
		}
	}
	if start >= 0 {
		flush(len(ends))
	}
	return walls
}

// reduceStandeeColumnRows carries each column's first and last coloured row
// one mip step down, to a level w x h.
func reduceStandeeColumnRows(first, last []int, srcW, srcH, w, h int) ([]int, []int) {
	reducedFirst, reducedLast := make([]int, w), make([]int, w)
	for x := range reducedFirst {
		top, bottom := -1, -1
		lo, hi := mipFootprint(x, srcW, w)
		for sx := lo; sx < hi; sx++ {
			if first[sx] < 0 {
				continue
			}
			if top < 0 || first[sx] < top {
				top = first[sx]
			}
			bottom = max(bottom, last[sx])
		}
		reducedFirst[x], reducedLast[x] = -1, -1
		if top >= 0 {
			reducedFirst[x], reducedLast[x] = mipFootprintOwner(top, srcH, h), mipFootprintOwner(bottom, srcH, h)
		}
	}
	return reducedFirst, reducedLast
}

// measure sets the rows of a level h texels tall in a chain whose level 0 is
// w0 x h0, from each of its columns' first and last coloured row.
func (r *standeeLevelRows) measure(first, last []int, h, w0, h0 int) {
	for band := range r.top {
		r.top[band], r.bottom[band] = 1, 0
	}
	w := len(first)
	fw, fh, fw0, fh0 := float64(w), float64(h), float64(w0), float64(h0)
	for x := 0; x < w; x++ {
		if first[x] < 0 {
			continue
		}
		// A bilinear fetch reads column x while its position in this level
		// lies within (x-0.5, x+1.5), and rows likewise; u and v reach the
		// level as standeeSourcePosition and sampleLinear1
		// (standee_sampling.kageinc) map them.
		u0 := (float64(x)-0.5)/fw - 0.5/fw0
		u1 := (float64(x)+1.5)/fw - 0.5/fw0
		v0 := ((float64(first[x])-0.5)*fh0/fh - 0.5) / (fh0 - 1)
		v1 := ((float64(last[x])+1.5)*fh0/fh - 0.5) / (fh0 - 1)
		for band := standeeRowBand(u0); band <= standeeRowBand(u1); band++ {
			r.top[band] = min(r.top[band], float32(max(v0, 0)))
			r.bottom[band] = max(r.bottom[band], float32(min(v1, 1)))
		}
	}
	for group := range r.groupTop {
		r.groupTop[group], r.groupBottom[group] = 1, 0
		for band := group * standeeRowGroup; band < (group+1)*standeeRowGroup; band++ {
			r.groupTop[group] = min(r.groupTop[group], r.top[band])
			r.groupBottom[group] = max(r.groupBottom[group], r.bottom[band])
		}
	}
}

func standeeRowBand(u float64) int {
	return min(max(int(u*standeeRowBands), 0), standeeRowBands-1)
}

// standeeTrim is the art one slab draw can read: the mip levels its shaders
// fetch. The zero value trims nothing.
type standeeTrim struct {
	levels [3]*standeeLevelRows
	count  int
	// reachU and reachV widen every lookup by the anisotropic taps of the
	// material path, as shares of the art's width and height.
	reachU, reachV float64
}

// newStandeeTrim gathers the levels a slab samples: the sticker level, the
// next one a minified sticker blends toward, and the core level. A chain
// without measured rows leaves the slab untrimmed.
func newStandeeTrim(stickerMips, coreMips *mipChain, mipLevel, nextMipLevel, coreMipLevel int, filtered bool) standeeTrim {
	var trim standeeTrim
	if stickerMips == nil || coreMips == nil || len(stickerMips.rows) == 0 ||
		len(stickerMips.rows) != len(stickerMips.levels) || len(coreMips.rows) != len(coreMips.levels) ||
		mipLevel < 0 || mipLevel >= len(stickerMips.rows) || nextMipLevel < 0 || nextMipLevel >= len(stickerMips.rows) ||
		coreMipLevel < 0 || coreMipLevel >= len(coreMips.rows) {
		return trim
	}
	add := func(level *standeeLevelRows) {
		for _, have := range trim.levels[:trim.count] {
			if have == level {
				return
			}
		}
		trim.levels[trim.count] = level
		trim.count++
	}
	add(&stickerMips.rows[mipLevel])
	if filtered {
		add(&stickerMips.rows[nextMipLevel])
	}
	add(&coreMips.rows[coreMipLevel])
	return trim
}

// reaching is the trim for the material path, whose minified sticker spreads
// four taps along its major axis. footprintX and footprintY are the path's
// own: texels of the full-size art per screen pixel. The shader picks the
// major axis per pixel, so both axes keep their reach. The slab carries one
// footprint per axis, so on a token seen edge-on the taps can land many screen
// pixels from their sample. The volume path measures its footprint per pixel
// and needs none of this.
func (t standeeTrim) reaching(footprintX, footprintY float64, full, level image.Point) standeeTrim {
	if t.count == 0 || level.X <= 0 || level.Y <= 0 || full.X < 2 || full.Y < 2 {
		return t
	}
	texelX := float64(full.X) / float64(level.X)
	t.reachU = standeeTapReach * max(footprintX-1.5*texelX, 0) / float64(full.X)
	texelY := float64(full.Y) / float64(level.Y)
	t.reachV = standeeTapReach * max(footprintY-1.5*texelY, 0) / float64(full.Y-1)
	return t
}

// pieces is how many pieces a strip of the given width is cut into; zero
// leaves it untrimmed.
func (t standeeTrim) pieces(columns int) int {
	if t.count == 0 || columns < standeeTrimMinColumns {
		return 0
	}
	return min(max(columns/standeeTrimPieceColumns, 1), standeeTrimMaxPieces)
}

// span is the stretch of v the art can colour between u0 and u1, in either
// order, widened by standeeTrimMargin on a face no shorter than height
// pixels; ok is false when the art can colour nothing there.
func (t standeeTrim) span(u0, u1, height float64) (top, bottom float64, ok bool) {
	if t.count == 0 {
		return 0, 1, true
	}
	if u1 < u0 {
		u0, u1 = u1, u0
	}
	u0, u1 = u0-t.reachU, u1+t.reachU
	if u1 < 0 || u0 > 1 {
		return 0, 0, false
	}
	lo, hi := float32(1), float32(0)
	first, last := standeeRowBand(u0), standeeRowBand(u1)
	for _, level := range t.levels[:t.count] {
		lo, hi = level.cover(first, last, lo, hi)
	}
	if lo > hi {
		return 0, 0, false
	}
	if !(height > standeeTrimMargin) {
		return 0, 1, true
	}
	slack := standeeTrimMargin/height + t.reachV
	return max(float64(lo)-slack, 0), min(float64(hi)+slack, 1), true
}
