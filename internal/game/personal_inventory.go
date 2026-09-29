package game

import "ugataima/internal/character"

func (g *MMGame) inventoryDragBag() character.InventoryBag { return g.party.Bag(g.dragInvOwner) }
