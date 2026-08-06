package config

import "testing"

func TestTileDataInheritsNeighbourFloor(t *testing.T) {
	tests := []struct {
		name string
		tile TileData
		want bool
	}{
		{
			name: "standee without floor inherits",
			tile: TileData{RenderType: TileRenderStandee},
			want: true,
		},
		{
			name: "tree without floor inherits",
			tile: TileData{RenderType: TileRenderCrossedStandee},
			want: true,
		},
		{
			name: "landmark without floor inherits",
			tile: TileData{RenderType: TileRenderLandmarkStandee},
			want: true,
		},
		{
			name: "wall without floor inherits",
			tile: TileData{RenderType: TileRenderWall},
			want: true,
		},
		{
			name: "object with texture group keeps authored floor",
			tile: TileData{RenderType: TileRenderStandee, FloorTextureGroup: "planks"},
			want: false,
		},
		{
			name: "object with floor color keeps authored floor",
			tile: TileData{RenderType: TileRenderStandee, FloorColor: [3]int{1, 2, 3}},
			want: false,
		},
		{
			name: "floor without opt in keeps its authored base",
			tile: TileData{RenderType: TileRenderFloor},
			want: false,
		},
		{
			name: "marker opt in overrides its tint",
			tile: TileData{RenderType: TileRenderFloor, FloorColor: [3]int{1, 2, 3}, InheritFloor: true},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.tile.InheritsNeighbourFloor(); got != tt.want {
				t.Fatalf("InheritsNeighbourFloor() = %t, want %t", got, tt.want)
			}
		})
	}
}
