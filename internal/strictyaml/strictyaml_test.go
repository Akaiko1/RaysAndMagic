package strictyaml

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type entry struct {
	Name string `yaml:"name"`
	Hp   int    `yaml:"hp"`
}

// keyedCatalog mirrors the NPC catalog: a custom unmarshaler decoding each
// entry from its own node.
type keyedCatalog struct{ Entries map[string]entry }

func (c *keyedCatalog) UnmarshalYAML(node *yaml.Node) error {
	var src struct {
		Entries map[string]yaml.Node `yaml:"entries"`
	}
	if err := DecodeNode(node, &src); err != nil {
		return err
	}
	c.Entries = map[string]entry{}
	for k, n := range src.Entries {
		var e entry
		if err := DecodeNode(&n, &e); err != nil {
			return err
		}
		c.Entries[k] = e
	}
	return nil
}

func TestDecodeRules(t *testing.T) {
	cases := []struct {
		name, doc, wantErr string
		into               func() any
	}{
		{"known fields", "name: a\nhp: 3\n", "", func() any { return &entry{} }},
		{"empty document", "", "", func() any { return &entry{} }},
		{"unknown field", "name: a\nhpp: 3\n", "hpp", func() any { return &entry{} }},
		{"second document", "name: a\n---\nname: b\n", "one YAML document", func() any { return &entry{} }},
		{"custom unmarshaler stays strict", "entries:\n  a: {name: a, hpp: 1}\n", "hpp", func() any { return &keyedCatalog{} }},
		{"custom unmarshaler rejects unknown top-level key", "entries: {}\nextra: 1\n", "extra", func() any { return &keyedCatalog{} }},
		{"alias to another entry's anchor", "entries:\n  a: &base {name: a, hp: 2}\n  b: *base\n", "", func() any { return &keyedCatalog{} }},
		{"merge from another entry", "entries:\n  a: &base {name: a, hp: 2}\n  b:\n    <<: *base\n    name: b\n", "", func() any { return &keyedCatalog{} }},
		{"typo beside a merge", "entries:\n  a: &base {name: a, hp: 2}\n  b:\n    <<: *base\n    nmae: b\n", "nmae", func() any { return &keyedCatalog{} }},
		{"typo names its line in the file", "entries:\n  a: {name: a, hp: 1}\n  b:\n    name: b\n    hpp: 2\n", "line 5: field hpp", func() any { return &keyedCatalog{} }},
		{"anchor holding its own alias", "entries:\n  a: &x {name: a, hp: 1, more: *x}\n", "contains itself", func() any { return &keyedCatalog{} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Decode([]byte(tc.doc), tc.into())
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Fatalf("error %v, want one naming %q", err, tc.wantErr)
			}
		})
	}
}

func TestDecodeNodeResolvesAliasValues(t *testing.T) {
	var c keyedCatalog
	doc := "entries:\n  a: &base {name: a, hp: 2}\n  b:\n    <<: *base\n    name: b\n"
	if err := Decode([]byte(doc), &c); err != nil {
		t.Fatal(err)
	}
	if got := c.Entries["b"]; got != (entry{Name: "b", Hp: 2}) {
		t.Fatalf("merged entry = %+v, want name b with the anchor's hp 2", got)
	}
	if got := c.Entries["a"]; got != (entry{Name: "a", Hp: 2}) {
		t.Fatalf("anchor entry = %+v", got)
	}
}
