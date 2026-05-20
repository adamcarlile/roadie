// Package config loads and validates the roadie collections.toml file.
package config

import (
	"fmt"

	"github.com/BurntSushi/toml"
)

// Kind values for a collection.
const (
	KindTV    = "tv"
	KindMovie = "movie"
)

// Collection is one configured source→dest mapping. Source and Dest are
// server-internal filesystem paths and are excluded from the JSON API.
type Collection struct {
	ID     string `toml:"id" json:"id"`
	Label  string `toml:"label" json:"label"`
	Source string `toml:"source" json:"-"`
	Dest   string `toml:"dest" json:"-"`
	Kind   string `toml:"kind" json:"kind"`
}

// Config is the full set of configured collections.
type Config struct {
	Collections []Collection
}

type tomlConfig struct {
	Collection []Collection `toml:"collection"`
}

// Load reads and validates a collections.toml file.
func Load(path string) (*Config, error) {
	var raw tomlConfig
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		return nil, fmt.Errorf("config %s: %w", path, err)
	}
	if len(raw.Collection) == 0 {
		return nil, fmt.Errorf("config %s: no collections defined", path)
	}
	seen := map[string]bool{}
	for i, c := range raw.Collection {
		switch {
		case c.ID == "":
			return nil, fmt.Errorf("config %s: collection #%d has no id", path, i+1)
		case c.Label == "":
			return nil, fmt.Errorf("config %s: collection %q missing required field %q", path, c.ID, "label")
		case c.Source == "":
			return nil, fmt.Errorf("config %s: collection %q missing required field %q", path, c.ID, "source")
		case c.Dest == "":
			return nil, fmt.Errorf("config %s: collection %q missing required field %q", path, c.ID, "dest")
		}
		if c.Kind != KindTV && c.Kind != KindMovie {
			return nil, fmt.Errorf("config %s: collection %q kind %q invalid (want tv or movie)", path, c.ID, c.Kind)
		}
		if seen[c.ID] {
			return nil, fmt.Errorf("config %s: duplicate collection id %q", path, c.ID)
		}
		seen[c.ID] = true
	}
	return &Config{Collections: raw.Collection}, nil
}

// Collection returns the collection with the given id.
func (c *Config) Collection(id string) (Collection, bool) {
	for _, col := range c.Collections {
		if col.ID == id {
			return col, true
		}
	}
	return Collection{}, false
}
