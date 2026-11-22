package config

// StatusEffectsConfig holds the control-save rules shared by every control
// spell. Each status's own numbers live in assets/statuses.yaml.
type StatusEffectsConfig struct {
	ControlSaves ControlSaveConfig `yaml:"control_saves"`
}

func (s StatusEffectsConfig) validate() error {
	return s.ControlSaves.validate()
}
