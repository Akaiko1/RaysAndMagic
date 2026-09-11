package graphics

import "github.com/hajimehoshi/ebiten/v2"

// ResourceImages returns the currently published allocation roots. Animation
// frames are independent images; renderer-created SubImages are only views and
// belong to the renderer's alias index. This never triggers a load.
func (sm *SpriteManager) ResourceImages(request SpriteResourceRequest) []*ebiten.Image {
	if sm == nil {
		return nil
	}
	if request.AnimationType == "" {
		if img := sm.sprites[request.Name]; img != nil {
			return []*ebiten.Image{img}
		}
		return nil
	}
	if animation := sm.animations[animationKey(request.Name, request.AnimationType)]; animation != nil {
		return append([]*ebiten.Image(nil), animation.Frames...)
	}
	return nil
}

func (sm *SpriteManager) indexResource(request SpriteResourceRequest) {
	if sm.imageResources == nil {
		sm.imageResources = make(map[*ebiten.Image]SpriteResourceRequest)
	}
	for _, img := range sm.ResourceImages(request) {
		if img != nil {
			sm.imageResources[img] = request
		}
	}
}

// DetachResource removes cache and origin references without releasing GPU
// allocations. The renderer's resource registry uses it when aliases may share
// allocation ownership; EvictResource retains its standalone release contract.
func (sm *SpriteManager) DetachResource(name, animationType string) []*ebiten.Image {
	request := SpriteResourceRequest{Name: name, AnimationType: animationType}
	images := sm.ResourceImages(request)
	if sm == nil {
		return nil
	}
	if animationType == "" {
		delete(sm.sprites, name)
	} else {
		key := animationKey(name, animationType)
		delete(sm.animations, key)
		delete(sm.animationMissing, key)
	}
	for _, img := range images {
		if sm.imageResources[img] == request {
			delete(sm.imageResources, img)
		}
	}
	return images
}
