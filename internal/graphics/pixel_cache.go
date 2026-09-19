package graphics

import (
	"bufio"
	"compress/zlib"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"image"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// PixelCache stores only reproducible CPU pixels. Call it from preparation
// workers, never Draw/Update. Entries are exact RGBA, not PNG round trips that
// can change premultiplied colors. No GPU images or decoded entries are kept.
type PixelCache struct{ Dir string }

const pixelCacheDiskLimit = 256 << 20
const pixelCacheEntryLimit = 32 << 20

var pixelCacheWriteMu sync.Mutex

// PixelCacheKey includes the algorithm/settings discriminator and every source
// pixel, ignoring origin and stride padding. Source edits invalidate the entry
// even when a file's name, size and modification time are unchanged.
func PixelCacheKey(settings string, sources ...*image.RGBA) [32]byte {
	h := sha256.New()
	io.WriteString(h, settings)
	for _, src := range sources {
		b := src.Bounds()
		binary.Write(h, binary.LittleEndian, uint64(b.Dx()))
		binary.Write(h, binary.LittleEndian, uint64(b.Dy()))
		for y := b.Min.Y; y < b.Max.Y; y++ {
			start := src.PixOffset(b.Min.X, y)
			h.Write(src.Pix[start : start+b.Dx()*4])
		}
	}
	var key [32]byte
	copy(key[:], h.Sum(nil))
	return key
}

func (c PixelCache) path(key [32]byte) string {
	return filepath.Join(c.Dir, hex.EncodeToString(key[:])+".rgba")
}

func cachePixelBytes(sizes []image.Point) int64 {
	var n int64
	for _, s := range sizes {
		if s.X <= 0 || s.Y <= 0 || s.X > pixelCacheEntryLimit/4 || s.Y > pixelCacheEntryLimit/(s.X*4) {
			return 0
		}
		n += int64(s.X) * int64(s.Y) * 4
		if n > pixelCacheEntryLimit {
			return 0
		}
	}
	return n
}

// Load requires the exact expected layout before allocating. Corrupt, partial,
// incompatible and oversized entries are ordinary cache misses.
func (c PixelCache) Load(ctx context.Context, key [32]byte, sizes []image.Point) ([]*image.RGBA, bool) {
	if c.Dir == "" || ctx.Err() != nil || cachePixelBytes(sizes) == 0 {
		return nil, false
	}
	f, err := os.Open(c.path(key))
	if err != nil {
		return nil, false
	}
	defer f.Close()
	z, err := zlib.NewReader(bufio.NewReaderSize(f, 64<<10))
	if err != nil {
		return nil, false
	}
	defer z.Close()
	var stored [32]byte
	if _, err := io.ReadFull(z, stored[:]); err != nil || stored != key {
		return nil, false
	}
	var count uint32
	if binary.Read(z, binary.LittleEndian, &count) != nil || int(count) != len(sizes) {
		return nil, false
	}
	images := make([]*image.RGBA, 0, len(sizes))
	for _, size := range sizes {
		if ctx.Err() != nil {
			return nil, false
		}
		var dims [2]uint32
		if binary.Read(z, binary.LittleEndian, &dims) != nil || dims != [2]uint32{uint32(size.X), uint32(size.Y)} {
			return nil, false
		}
		img := image.NewRGBA(image.Rectangle{Max: size})
		if _, err := io.ReadFull(z, img.Pix); err != nil {
			return nil, false
		}
		images = append(images, img)
	}
	// Read through the checksum, and reject unexpected trailing payload.
	var extra [1]byte
	if n, err := z.Read(extra[:]); n != 0 || err != io.EOF {
		return nil, false
	}
	return images, true
}

// Store is optional: full/unwritable disks or cancellation never fail loading.
// Atomic replacement prevents a second process from reading a partial entry.
func (c PixelCache) Store(ctx context.Context, key [32]byte, images []*image.RGBA) {
	if c.Dir == "" || ctx.Err() != nil {
		return
	}
	sizes := make([]image.Point, 0, len(images))
	for _, img := range images {
		if img == nil {
			return
		}
		sizes = append(sizes, img.Bounds().Size())
	}
	if cachePixelBytes(sizes) == 0 {
		return
	}
	pixelCacheWriteMu.Lock()
	defer pixelCacheWriteMu.Unlock()
	if ctx.Err() != nil || os.MkdirAll(c.Dir, 0755) != nil {
		return
	}
	f, err := os.CreateTemp(c.Dir, ".pixels-*")
	if err != nil {
		return
	}
	defer os.Remove(f.Name())
	defer f.Close()
	// Deflate emits many tiny writes. Buffer them before the filesystem;
	// otherwise populating this optional cache can dominate cold loading.
	buffered := bufio.NewWriterSize(f, 64<<10)
	z, err := zlib.NewWriterLevel(buffered, zlib.BestSpeed)
	if err != nil {
		return
	}
	write := func() error {
		if _, err := z.Write(key[:]); err != nil {
			return err
		}
		if err := binary.Write(z, binary.LittleEndian, uint32(len(images))); err != nil {
			return err
		}
		for _, img := range images {
			if err := ctx.Err(); err != nil {
				return err
			}
			b := img.Bounds()
			if err := binary.Write(z, binary.LittleEndian, [2]uint32{uint32(b.Dx()), uint32(b.Dy())}); err != nil {
				return err
			}
			for y := b.Min.Y; y < b.Max.Y; y++ {
				start := img.PixOffset(b.Min.X, y)
				if _, err := z.Write(img.Pix[start : start+b.Dx()*4]); err != nil {
					return err
				}
			}
		}
		return nil
	}
	err = write()
	closeErr := z.Close()
	flushErr := buffered.Flush()
	fileErr := f.Close()
	if err != nil || closeErr != nil || flushErr != nil || fileErr != nil || ctx.Err() != nil {
		return
	}
	if os.Rename(f.Name(), c.path(key)) != nil {
		return
	}
	c.prune(pixelCacheDiskLimit)
}

func (c PixelCache) prune(limit int64) {
	entries, err := os.ReadDir(c.Dir)
	if err != nil {
		return
	}
	var files []os.FileInfo
	var total int64
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".rgba") {
			continue
		}
		if info, err := e.Info(); err == nil {
			files = append(files, info)
			total += info.Size()
		}
	}
	// Oldest generated entries go first. This does not retain decoded pixels or
	// touch user saves, and concurrent readers retain their open file handles.
	sort.Slice(files, func(i, j int) bool { return files[i].ModTime().Before(files[j].ModTime()) })
	for _, f := range files {
		if total <= limit {
			break
		}
		if os.Remove(filepath.Join(c.Dir, f.Name())) == nil {
			total -= f.Size()
		}
	}
}
