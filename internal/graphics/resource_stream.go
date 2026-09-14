package graphics

import (
	"context"
	"image"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
)

// ResourcePixelBytesWithin reads only the source header. It lets an owner opt
// small UI images into the synchronous API without decoding an oversized PNG.
// Zero means absent, invalid, or outside the caller's decoded-pixel budget.
func (sm *SpriteManager) ResourcePixelBytesWithin(request SpriteResourceRequest, limit int) int {
	if sm == nil || limit < 4 {
		return 0
	}
	sm.ensureIndex()
	name := request.Name
	if request.AnimationType != "" {
		name += "_" + request.AnimationType
	}
	f, err := os.Open(sm.spritePaths[name])
	if err != nil {
		return 0
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > limit/4 || cfg.Height > limit/(4*cfg.Width) {
		return 0
	}
	return cfg.Width * cfg.Height * 4
}

// SetDeferredResourceHandler confines runtime cache misses to a request queue.
// A nil handler preserves synchronous boot/editor APIs. The handler and all
// publication run on the game owner; workers only read immutable decode inputs.
func (sm *SpriteManager) SetDeferredResourceHandler(handler func(SpriteResourceRequest) bool) {
	sm.deferResource = handler
}

func (sm *SpriteManager) deferMissingResource(request SpriteResourceRequest) bool {
	if sm.deferResource == nil || sm.failedResources[request] {
		return false
	}
	sm.ensureIndex()
	name := request.Name
	if request.AnimationType != "" {
		name += "_" + request.AnimationType
	}
	if sm.spritePaths[name] == "" {
		return false
	}
	return sm.deferResource(request)
}

// ResourceStream serves unexpected demand with one bounded decode worker and
// one incremental GPU commit. It does not claim ownership of published images;
// callers register them with their existing residency owner.
type ResourceStream struct {
	manager *SpriteManager
	ctx     context.Context
	cancel  context.CancelFunc
	queue   []SpriteResourceRequest
	pending map[SpriteResourceRequest]bool
	results <-chan PreparedSpriteResource
	commit  *PreparedSpriteCommit
	request SpriteResourceRequest
}

func NewResourceStream(sm *SpriteManager) *ResourceStream {
	ctx, cancel := context.WithCancel(context.Background())
	return &ResourceStream{manager: sm, ctx: ctx, cancel: cancel, pending: make(map[SpriteResourceRequest]bool)}
}

func (s *ResourceStream) Request(request SpriteResourceRequest) {
	if s == nil || s.ctx.Err() != nil || s.pending[request] {
		return
	}
	s.pending[request] = true
	s.queue = append(s.queue, request)
}

func (s *ResourceStream) Pending() bool { return s != nil && len(s.pending) != 0 }

func (s *ResourceStream) Close() {
	if s == nil {
		return
	}
	s.cancel()
	if s.commit != nil {
		s.commit.Cancel()
	}
	s.commit, s.results, s.queue = nil, nil, nil
	clear(s.pending)
}

// Advance never waits for a worker. Non-nil images are newly usable resources;
// even decode failure completes its request and is negatively cached.
func (s *ResourceStream) Advance(maxBytes int) (SpriteResourceRequest, map[*ebiten.Image]*image.RGBA) {
	if s == nil || s.ctx.Err() != nil {
		return SpriteResourceRequest{}, nil
	}
	if s.commit != nil {
		images, done := s.commit.Advance(maxBytes)
		if done {
			request := s.request
			delete(s.pending, request)
			s.commit = nil
			return request, images
		}
		return SpriteResourceRequest{}, nil
	}
	if s.results != nil {
		select {
		case prepared, ok := <-s.results:
			if ok {
				s.request = prepared.Request
				s.commit = s.manager.BeginPreparedResourceCommit(prepared)
			} else {
				s.results = nil
			}
		default:
		}
	} else if len(s.queue) > 0 {
		requests := s.queue
		s.queue = nil
		s.results = s.manager.prepareResources(s.ctx, requests, true, NewPreparationBudget(32<<20))
	}
	return SpriteResourceRequest{}, nil
}
