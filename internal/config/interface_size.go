package config

import "fmt"

// InterfaceSize is one Settings > Display preset. The interface lays out at
// the logical resolution divided by Scale and draws at the screen's native
// resolution; the 3D view keeps Normal's resolution.
type InterfaceSize struct {
	Key   string  `yaml:"key"`
	Label string  `yaml:"label"`
	Scale float64 `yaml:"scale"`
}

const maxInterfaceScale = 3

// InterfaceSize resolves a preset by its saved key.
func (d DisplayConfig) InterfaceSize(key string) (InterfaceSize, bool) {
	for _, size := range d.InterfaceSizes {
		if size.Key == key {
			return size, true
		}
	}
	return InterfaceSize{}, false
}

func (d DisplayConfig) validateInterfaceSizes() error {
	if len(d.InterfaceSizes) == 0 {
		return fmt.Errorf("display.interface_sizes is required")
	}
	seen := make(map[string]bool, len(d.InterfaceSizes))
	previous := 0.0
	for i, size := range d.InterfaceSizes {
		where := fmt.Sprintf("display.interface_sizes[%d]", i)
		switch {
		case size.Key == "":
			return fmt.Errorf("%s: key is required", where)
		case seen[size.Key]:
			return fmt.Errorf("%s: duplicate key %q", where, size.Key)
		case size.Label == "":
			return fmt.Errorf("%s: label is required", where)
		case size.Scale < 1 || size.Scale > maxInterfaceScale:
			return fmt.Errorf("%s: scale %.2f must be within 1..%d", where, size.Scale, maxInterfaceScale)
		case size.Scale <= previous:
			return fmt.Errorf("%s: scale %.2f must be larger than the previous preset", where, size.Scale)
		}
		if !isASCII(size.Label) {
			return fmt.Errorf("%s: label must be ASCII", where)
		}
		seen[size.Key] = true
		previous = size.Scale
	}
	if _, ok := d.InterfaceSize(d.DefaultInterfaceSize); !ok {
		return fmt.Errorf("display.default_interface_size %q is not one of display.interface_sizes", d.DefaultInterfaceSize)
	}
	return nil
}
