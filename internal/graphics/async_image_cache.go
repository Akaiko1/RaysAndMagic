package graphics

import "github.com/hajimehoshi/ebiten/v2"

// AsyncImageCache owns optional static UI/editor art. Decode and incremental
// upload reuse ResourceStream; the cache adds bounded residency, not a second
// loader. Images returned by Get are borrowed until the next Advance/Close.
type AsyncImageCache struct {
	manager      *SpriteManager
	stream       *ResourceStream
	entries      map[string]*asyncImageEntry
	limit, bytes int
	clock        uint64
	closed       bool
}

type asyncImageEntry struct {
	image *ebiten.Image
	bytes int
	used  uint64
	ready bool
}

const asyncImageMaxEntries = 128

func NewAsyncImageCache(limit int) *AsyncImageCache {
	sm := NewSpriteManager()
	return &AsyncImageCache{manager: sm, stream: NewResourceStream(sm), entries: make(map[string]*asyncImageEntry), limit: max(4, limit)}
}

// Get returns ready=false while loading; a ready nil image is missing/invalid.
// This distinction lets portrait fallbacks wait for their preferred image.
func (c *AsyncImageCache) Get(name string) (*ebiten.Image, bool) {
	if c == nil || c.closed || name == "" {
		return nil, true
	}
	c.clock++
	if entry := c.entries[name]; entry != nil {
		entry.used = c.clock
		return entry.image, entry.ready
	}
	if len(c.entries) >= asyncImageMaxEntries {
		// Never release a borrowed image during Draw. Advance evicts entries.
		return nil, false
	}
	entry := &asyncImageEntry{used: c.clock}
	c.entries[name] = entry
	request := SpriteResourceRequest{Name: name}
	if c.manager.ResourcePixelBytesWithin(request, c.limit) == 0 {
		entry.ready = true
		return nil, true
	}
	c.stream.Request(request)
	return nil, false
}

func (c *AsyncImageCache) evictOldest(except string) bool {
	var name string
	var oldest *asyncImageEntry
	for key, entry := range c.entries {
		if key != except && entry.ready && (oldest == nil || entry.used < oldest.used) {
			name, oldest = key, entry
		}
	}
	if oldest == nil {
		return false
	}
	c.manager.EvictResource(name, "")
	delete(c.manager.visibleFrameBounds, name)
	c.bytes -= oldest.bytes
	delete(c.entries, name)
	return true
}

func (c *AsyncImageCache) Advance(maxBytes int) {
	if c == nil || c.closed {
		return
	}
	request, images := c.stream.Advance(maxBytes)
	if request.Name != "" {
		entry := c.entries[request.Name]
		entry.ready = true
		for img := range images {
			entry.image = img
			entry.bytes = img.Bounds().Dx() * img.Bounds().Dy() * 4
		}
		c.bytes += entry.bytes
		for c.bytes > c.limit && c.evictOldest(request.Name) {
		}
	}
	// Leave one admission slot. Pending requests are never evicted.
	for len(c.entries) >= asyncImageMaxEntries && c.evictOldest("") {
	}
}

func (c *AsyncImageCache) Close() {
	if c == nil || c.closed {
		return
	}
	c.closed = true
	c.stream.Close()
	for name := range c.entries {
		c.manager.EvictResource(name, "")
	}
	clear(c.entries)
	c.bytes = 0
}
