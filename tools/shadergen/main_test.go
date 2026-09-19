package main

import "testing"

func TestMergePlatformArtifactsRequiresMatchingSourceID(t *testing.T) {
	previous := []shader{{SourceID: "same", Metal: []byte{1}, DXBCVertex: []byte{2}, DXBCPixel: []byte{3}}, {SourceID: "removed", Metal: []byte{4}}}
	shaders := []shader{{SourceID: "same"}, {SourceID: "changed"}}
	mergePlatformArtifacts(shaders, previous)
	if len(shaders) != 2 || len(shaders[0].Metal) != 1 || len(shaders[0].DXBCVertex) != 1 || len(shaders[0].DXBCPixel) != 1 {
		t.Fatal("other backend artifacts lost")
	}
	if len(shaders[1].Metal) != 0 || len(shaders[1].DXBCVertex) != 0 || len(shaders[1].DXBCPixel) != 0 {
		t.Fatal("stale artifacts survived a source/engine change")
	}
}
