package main

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"ugataima/internal/config"
)

func overlayMechanicsWrite(d *overlayDocument, maps *config.MapConfigs) (overlayWrite, error) {
	now, err := os.ReadFile(config.BossMechanicsPath)
	if err != nil && !os.IsNotExist(err) {
		return overlayWrite{}, err
	}
	read := func(raw []byte) (*config.BossMechanicsConfig, error) {
		c := &config.BossMechanicsConfig{}
		if len(raw) > 0 {
			if err := yaml.Unmarshal(raw, c); err != nil {
				return nil, err
			}
		}
		return c, nil
	}
	baseRules, err := read(d.mechanicsBase)
	if err != nil {
		return overlayWrite{}, err
	}
	currentRules, err := read(now)
	if err != nil {
		return overlayWrite{}, err
	}
	// Compare the real entry before creating a container for the first save.
	// Missing and empty lists both mean this map has no mechanics.
	before, existsBefore := baseRules.Maps[d.key]
	after, existsAfter := currentRules.Maps[d.key]
	if (len(before) > 0 || len(after) > 0) && (!overlaySame(before, after) || existsBefore != existsAfter) {
		return overlayWrite{}, fmt.Errorf("Boss mechanics changed on disk; reload before saving")
	}
	current := now
	if len(current) == 0 {
		current = []byte("maps: {}\n")
	}
	path := []string{"maps", d.key}
	next, err := overlayPatch(current, path, d.state.Mechanics)
	if err != nil {
		return overlayWrite{}, err
	}
	var cfg config.BossMechanicsConfig
	if err = yaml.Unmarshal(next, &cfg); err != nil {
		return overlayWrite{}, err
	}
	refs := map[string]*config.MapConfig{}
	for k, m := range maps.Maps {
		copy := m
		refs[k] = &copy
	}
	if err = cfg.Validate(refs); err != nil {
		return overlayWrite{}, err
	}
	return overlayWrite{config.BossMechanicsPath, now, next}, nil
}
