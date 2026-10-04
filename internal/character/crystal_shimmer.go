package character

// CrystalShimmer locates luminous gems in normalized facade texture space.
type CrystalShimmer struct {
	IntervalSeconds float64         `yaml:"interval_seconds"`
	DurationSeconds float64         `yaml:"duration_seconds"`
	Crystals        []FacadeCrystal `yaml:"crystals"`
}

type FacadeCrystal struct {
	Center [2]float64 `yaml:"center"`
	Radius [2]float64 `yaml:"radius"`
	Color  [3]int     `yaml:"color"`
}
