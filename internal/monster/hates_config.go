package monster

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// HatesConfig is the schema of assets/hates.yaml: per monster key, the list of
// party "traits" that make that (otherwise passive) monster turn hostile on
// sight. Keeps aggro relationships data-driven and separate from monster stats.
type HatesConfig struct {
	Hates map[string][]string `yaml:"hates"`
}

// HatesTable maps monster key -> hated party traits, loaded from hates.yaml.
var HatesTable = map[string][]string{}

// LoadHatesConfig reads hates.yaml into HatesTable. A missing file is not an
// error (no hate relationships defined).
func LoadHatesConfig(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			HatesTable = map[string][]string{}
			return nil
		}
		return fmt.Errorf("failed to read hates config file: %w", err)
	}
	var cfg HatesConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("failed to parse hates config YAML: %w", err)
	}
	if cfg.Hates == nil {
		cfg.Hates = map[string][]string{}
	}
	HatesTable = cfg.Hates
	return nil
}

// MustLoadHatesConfig loads hates.yaml and panics on a parse error.
func MustLoadHatesConfig(filename string) {
	if err := LoadHatesConfig(filename); err != nil {
		panic("Failed to load hates config: " + err.Error())
	}
}
