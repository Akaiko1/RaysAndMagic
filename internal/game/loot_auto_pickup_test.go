package game

import (
	"math"
	"testing"

	"ugataima/internal/items"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
)

// Walking picks up every loot bag on the party's tile and the eight tiles
// around it, in real time and turn-based alike. Chests, farther bags, bags
// out of reach and bags still falling stay on the ground; so does everything
// while the party stands still.
func TestLootBagsAutoPickupOnArrival(t *testing.T) {
	type bag struct {
		dx, dy int // tile offset from where the party lands
		chest  bool
		hop    bool // still falling
		want   bool // picked up
	}
	for _, mode := range []struct {
		name string
		tb   bool
	}{{"real-time", false}, {"turn-based", true}} {
		for _, tc := range []struct {
			name  string
			bags  []bag
			walls [][2]int // tile offsets from the landing tile
			still bool     // the party does not move
		}{
			{name: "same tile", bags: []bag{{want: true}}},
			{name: "orthogonal neighbour", bags: []bag{{dx: 0, dy: -1, want: true}}},
			{name: "diagonal neighbour", bags: []bag{{dx: 1, dy: 1, want: true}}},
			{name: "two tiles away", bags: []bag{{dx: 2, dy: 0}}},
			{name: "chest next door", bags: []bag{{dx: 1, dy: 0, chest: true}}},
			{name: "corner walled off", bags: []bag{{dx: 1, dy: 1}}, walls: [][2]int{{1, 0}, {0, 1}}},
			{name: "still falling", bags: []bag{{dx: 0, dy: 1, hop: true}}},
			{name: "several at once", bags: []bag{{want: true}, {dx: -1, dy: 1, want: true}, {dx: 1, dy: -1, want: true}, {dx: 3, dy: 3}}},
			{name: "standing still", bags: []bag{{dx: 0, dy: 1}}, still: true},
		} {
			t.Run(mode.name+"/"+tc.name, func(t *testing.T) {
				g, gl, ts := tbBehaviorGame(t, 30, 30)
				g.turnBasedMode = mode.tb
				if mode.tb {
					g.startPartyTurn()
				}
				g.frameCount = 1000
				ih := NewInputHandler(g)
				gl.inputHandler = ih
				g.gameLoop = gl
				placePlayerAtTile(g, 10, 10, ts)
				g.camera.Angle = 0 // facing +X
				land := [2]int{10, 10}
				if mode.tb && !tc.still {
					land = [2]int{11, 10}
				}
				for _, w := range tc.walls {
					g.world.Tiles[land[1]+w[1]][land[0]+w[0]] = world.TileWall
				}
				g.collisionSystem.UpdateTileChecker(g.world)
				for i, b := range tc.bags {
					x, y := TileCenterFromTile(land[0]+b.dx, land[1]+b.dy, ts)
					c := GroundContainer{Kind: ContainerKindLootBag, X: x, Y: y, Gold: 10 + i}
					if b.chest {
						c.Kind = ContainerKindTreasureChest
						c.Items = []items.Item{items.CreateItemFromYAML("health_potion")}
					}
					if b.hop {
						c.hop = lootHop{started: g.frameCount, duration: 1000}
					}
					g.groundContainers = append(g.groundContainers, c)
				}
				gold := g.party.Gold
				if !tc.still {
					ih.keys = keysPressed(ebiten.KeyW)
					ih.heldKeys = heldOnly(ebiten.KeyW)
					ih.HandleInput()
				}
				if lx, ly := math.Floor(g.camera.X/ts), math.Floor(g.camera.Y/ts); int(lx) != land[0] || int(ly) != land[1] {
					t.Fatalf("fixture: the party landed on %v,%v, want %v", lx, ly, land)
				}
				wantGold := gold
				for i, b := range tc.bags {
					if b.want {
						wantGold += 10 + i
					}
				}
				if g.party.Gold != wantGold {
					t.Errorf("gold %d, want %d", g.party.Gold, wantGold)
				}
				left := 0
				for _, b := range tc.bags {
					if !b.want {
						left++
					}
				}
				if len(g.groundContainers) != left {
					t.Errorf("%d containers left on the ground, want %d", len(g.groundContainers), left)
				}
			})
		}
	}
}
