// Package strictyaml is the one decoder for authored YAML: a field the target
// does not declare, or a second document, fails the load. A typo in content
// must stop the boot, never switch a mechanic off silently.
package strictyaml

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Decode reads one YAML document into out. An empty input decodes to the zero
// value.
func Decode(data []byte, out any) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return fmt.Errorf("expected one YAML document")
	}
	return nil
}

// DecodeNode is Decode for a node a custom UnmarshalYAML holds: yaml.v3 drops
// KnownFields inside node.Decode, so the node is re-read strictly. Aliases are
// expanded first, as their anchors may sit outside the node; error lines are
// moved back from the re-read text to the node's own document.
func DecodeNode(node *yaml.Node, out any) error {
	expanded, err := expandAliases(node, map[*yaml.Node]bool{})
	if err != nil {
		return err
	}
	data, err := yaml.Marshal(expanded)
	if err != nil {
		return err
	}
	return documentLines(Decode(data, out), node.Line)
}

// expandAliases copies n with every alias replaced by its anchored value; an
// anchor holding its own alias is an error, not endless recursion. open is
// the set of anchors being expanded on the current path.
func expandAliases(n *yaml.Node, open map[*yaml.Node]bool) (*yaml.Node, error) {
	if n == nil {
		return nil, nil
	}
	if n.Kind == yaml.AliasNode && n.Alias != nil {
		if open[n.Alias] {
			return nil, fmt.Errorf("line %d: anchor %q contains itself", n.Line, n.Value)
		}
		open[n.Alias] = true
		defer delete(open, n.Alias)
		return expandAliases(n.Alias, open)
	}
	c := *n
	c.Anchor = ""
	c.Content = make([]*yaml.Node, len(n.Content))
	for i, child := range n.Content {
		expanded, err := expandAliases(child, open)
		if err != nil {
			return nil, err
		}
		c.Content[i] = expanded
	}
	return &c, nil
}

var lineRef = regexp.MustCompile(`^line (\d+):`)

// documentLines moves the "line N:" of a re-read node's type errors by the
// node's first line, so they point into the authored file.
func documentLines(err error, first int) error {
	var te *yaml.TypeError
	if first < 1 || !errors.As(err, &te) {
		return err
	}
	moved := &yaml.TypeError{Errors: make([]string, len(te.Errors))}
	for i, msg := range te.Errors {
		moved.Errors[i] = lineRef.ReplaceAllStringFunc(msg, func(ref string) string {
			n, _ := strconv.Atoi(ref[len("line ") : len(ref)-1])
			return fmt.Sprintf("line %d:", n+first-1)
		})
	}
	return moved
}
