package config

import "strings"

// WaterBodyKind classifies one connected body of water tiles on a map.
type WaterBodyKind string

const (
	WaterBodyRiver  WaterBodyKind = "river"
	WaterBodyLake   WaterBodyKind = "lake"
	WaterBodySea    WaterBodyKind = "sea"
	WaterBodyPool   WaterBodyKind = "pool"
	WaterBodyStream WaterBodyKind = "stream"
)

// WaterBodyKinds is the closed set, in the order error messages list it.
var WaterBodyKinds = []WaterBodyKind{WaterBodyRiver, WaterBodyLake, WaterBodySea, WaterBodyPool, WaterBodyStream}

// Surf reports whether waves break on the bank: lakes and seas only.
func (k WaterBodyKind) Surf() bool {
	return k == WaterBodyLake || k == WaterBodySea
}

// WaterBodyKindList is the closed set as "river|lake|..." for messages.
func WaterBodyKindList() string {
	names := make([]string, len(WaterBodyKinds))
	for i, k := range WaterBodyKinds {
		names[i] = string(k)
	}
	return strings.Join(names, "|")
}

// MapWaterBody names one body by any of its cells (map-local tile).
type MapWaterBody struct {
	At   [2]int        `yaml:"at"`
	Kind WaterBodyKind `yaml:"kind"`
}
