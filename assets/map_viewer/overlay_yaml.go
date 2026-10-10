package main

import (
	"bytes"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// The overlay rewrites only what an edit changed. An entry is merged node by
// node with the newly encoded value: unchanged subtrees keep their text,
// anchors, aliases and comments; zero values never add keys; a cleared field
// loses its key. Text outside the touched entries is preserved byte for byte.
//
// "Zero" follows the Go type the entry decodes into: two texts are equal when
// the game decodes them to the same value. An optional pointer set to a zero
// value, a map key and a slice element are present even when zero; a plain
// field, a fixed array and an empty block are not. A nil type (untyped data)
// treats every zero as absent.

// overlayPlain decodes a node to plain data with absent-equivalent values
// dropped, as described above.
func overlayPlain(n *yaml.Node, t reflect.Type) any {
	if n == nil {
		return nil
	}
	var v any
	if err := n.Decode(&v); err != nil {
		return n.Value
	}
	return overlayNormalize(v, t)
}

// overlayPresent marks a zero value the game still sees.
type overlayPresent struct{}

func overlayKept(v any) any {
	if v == nil {
		return overlayPresent{}
	}
	return v
}

func overlayNormalize(v any, t reflect.Type) any {
	if t != nil && t.Kind() == reflect.Interface {
		t = nil
	}
	if t != nil && v != nil {
		switch t.Kind() {
		case reflect.Pointer:
			n := overlayNormalize(v, t.Elem())
			if n == nil && t.Elem().Kind() != reflect.Struct {
				return overlayPresent{} // an optional value explicitly set to zero
			}
			return n // a block is present through its contents
		case reflect.Struct, reflect.Map:
			m, ok := v.(map[string]any)
			if !ok {
				break
			}
			out := map[string]any{}
			for k, e := range m {
				n := overlayNormalize(e, overlayChild(t, k))
				if t.Kind() == reflect.Map {
					n = overlayKept(n)
				}
				if n != nil {
					out[k] = n
				}
			}
			if len(out) == 0 {
				return nil
			}
			return out
		case reflect.Slice, reflect.Array:
			list, ok := v.([]any)
			if !ok {
				break
			}
			out := make([]any, len(list))
			empty := true
			for i, e := range list {
				out[i] = overlayNormalize(e, t.Elem())
				if t.Kind() == reflect.Slice {
					out[i] = overlayKept(out[i])
				}
				empty = empty && out[i] == nil
			}
			if empty {
				return nil
			}
			return out
		}
	}
	switch x := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, e := range x {
			if n := overlayNormalize(e, nil); n != nil {
				out[k] = n
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case []any:
		out := make([]any, len(x))
		empty := true
		for i, e := range x {
			out[i] = overlayNormalize(e, nil)
			empty = empty && out[i] == nil
		}
		if empty {
			return nil
		}
		return out
	case string:
		if x == "" {
			return nil
		}
	case bool:
		if !x {
			return nil
		}
	case int:
		if x == 0 {
			return nil
		}
		return float64(x)
	case uint64:
		if x == 0 {
			return nil
		}
		return float64(x)
	case float64:
		if x == 0 {
			return nil
		}
	}
	return v
}

func overlayDeref(t reflect.Type) reflect.Type {
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

// overlayChild is the Go type a mapping key decodes into; nil when unknown.
func overlayChild(t reflect.Type, key string) reflect.Type {
	t = overlayDeref(t)
	if t == nil {
		return nil
	}
	switch t.Kind() {
	case reflect.Map:
		return t.Elem()
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			name, _, _ := strings.Cut(f.Tag.Get("yaml"), ",")
			if !f.IsExported() || name == "-" {
				continue
			}
			if name == "" {
				name = strings.ToLower(f.Name)
			}
			if name == key {
				return f.Type
			}
		}
	}
	return nil
}

// overlayElem is the Go type of a sequence element; nil when unknown.
func overlayElem(t reflect.Type) reflect.Type {
	t = overlayDeref(t)
	if t != nil && (t.Kind() == reflect.Slice || t.Kind() == reflect.Array) {
		return t.Elem()
	}
	return nil
}

func overlayNodeEqual(a, b *yaml.Node, t reflect.Type) bool {
	return reflect.DeepEqual(overlayPlain(a, t), overlayPlain(b, t))
}

// overlayCopy deep-copies a node. Alias targets stay shared; stripAnchors
// removes anchor definitions so a copied subtree cannot redefine them.
func overlayCopy(n *yaml.Node, stripAnchors bool) *yaml.Node {
	if n == nil {
		return nil
	}
	c := *n
	if stripAnchors {
		c.Anchor = ""
	}
	c.Content = make([]*yaml.Node, len(n.Content))
	for i, child := range n.Content {
		c.Content[i] = overlayCopy(child, stripAnchors)
	}
	return &c
}

type overlayMerger struct {
	// Anchored nodes whose content changed: original pointer and unchanged copy.
	detached []overlayDetached
}
type overlayDetached struct {
	original, copy *yaml.Node
}

// merge combines old text with the newly encoded value of Go type t.
func (m *overlayMerger) merge(old, next *yaml.Node, t reflect.Type) *yaml.Node {
	if overlayNodeEqual(old, next, t) {
		return old
	}
	if old.Kind == yaml.AliasNode && old.Alias != nil {
		// Copy-on-write: this entry stops sharing, starting from the shared text.
		return m.merge(overlayCopy(old.Alias, true), next, t)
	}
	if old.Anchor != "" {
		m.detached = append(m.detached, overlayDetached{original: old, copy: overlayCopy(old, false)})
	}
	var out *yaml.Node
	switch {
	case old.Kind == yaml.MappingNode && next.Kind == yaml.MappingNode:
		out = m.mergeMapping(old, next, t)
	case old.Kind == yaml.SequenceNode && next.Kind == yaml.SequenceNode:
		out = &yaml.Node{Kind: yaml.SequenceNode, Tag: old.Tag, Style: old.Style, HeadComment: old.HeadComment, LineComment: old.LineComment, FootComment: old.FootComment}
		for i, child := range next.Content {
			if i < len(old.Content) {
				out.Content = append(out.Content, m.merge(old.Content[i], child, overlayElem(t)))
			} else {
				out.Content = append(out.Content, child)
			}
		}
		for _, dropped := range old.Content[min(len(old.Content), len(next.Content)):] {
			m.detachAll(dropped)
		}
	default:
		for _, child := range old.Content {
			m.detachAll(child) // the whole old subtree is replaced
		}
		out = next
		if old.Kind == next.Kind && old.Tag == next.Tag {
			out.Style = old.Style
		}
		out.HeadComment, out.LineComment, out.FootComment = old.HeadComment, old.LineComment, old.FootComment
	}
	out.Anchor = ""
	return out
}

// detachAll records every anchor defined in a subtree that is being dropped,
// so its other users receive the unchanged text.
func (m *overlayMerger) detachAll(n *yaml.Node) {
	if n == nil || n.Kind == yaml.AliasNode {
		return
	}
	if n.Anchor != "" {
		m.detached = append(m.detached, overlayDetached{original: n, copy: overlayCopy(n, false)})
		return // the copy carries any anchors nested inside it
	}
	for _, child := range n.Content {
		m.detachAll(child)
	}
}

// overlayMergeSupplied lists the keys a mapping inherits through `<<`.
func overlayMergeSupplied(n *yaml.Node, t reflect.Type) map[string]bool {
	out := map[string]bool{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Tag != "!!merge" {
			continue
		}
		var v any
		if n.Content[i+1].Decode(&v) != nil {
			continue
		}
		add := func(x any) {
			if m, ok := x.(map[string]any); ok {
				for k, e := range m {
					if overlayNormalize(e, overlayChild(t, k)) != nil {
						out[k] = true
					}
				}
			}
		}
		if list, ok := v.([]any); ok {
			for _, x := range list {
				add(x)
			}
		} else {
			add(v)
		}
	}
	return out
}

func (m *overlayMerger) mergeMapping(old, next *yaml.Node, t reflect.Type) *yaml.Node {
	out := &yaml.Node{Kind: yaml.MappingNode, Tag: old.Tag, Style: old.Style, HeadComment: old.HeadComment, LineComment: old.LineComment, FootComment: old.FootComment}
	if len(old.Content) == 0 {
		out.Style = 0 // an empty `{}` grows into an ordinary block mapping
	}
	nextValues := map[string]*yaml.Node{}
	var order []string
	for i := 0; i+1 < len(next.Content); i += 2 {
		nextValues[next.Content[i].Value] = next.Content[i+1]
		order = append(order, next.Content[i].Value)
	}
	var effective map[string]any
	_ = old.Decode(&effective)
	supplied := overlayMergeSupplied(old, t)
	// A Go map holds its keys: one missing from the new value is removed even
	// when its text is zero, and a new one is written even when zero.
	keyed := overlayDeref(t) != nil && overlayDeref(t).Kind() == reflect.Map
	plain := func(n *yaml.Node, key string) any {
		p := overlayPlain(n, overlayChild(t, key))
		if keyed {
			p = overlayKept(p)
		}
		return p
	}
	explicit := map[string]bool{}
	keyStyle := yaml.Style(0)
	null := func() *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"} }
	for i := 0; i+1 < len(old.Content); i += 2 {
		k, v := old.Content[i], old.Content[i+1]
		if k.Tag == "!!merge" {
			out.Content = append(out.Content, k, v)
			continue
		}
		if keyStyle == 0 {
			keyStyle = k.Style
		}
		explicit[k.Value] = true
		nv, ok := nextValues[k.Value]
		switch {
		case ok:
			out.Content = append(out.Content, k, m.merge(v, nv, overlayChild(t, k.Value)))
		case plain(v, k.Value) == nil:
			out.Content = append(out.Content, k, v) // an explicit zero is harmless
		case supplied[k.Value]:
			m.detachAll(v)
			out.Content = append(out.Content, k, null()) // cleared, but `<<` would refill it
		default:
			m.detachAll(v) // cleared: the key goes away
		}
	}
	for _, key := range order {
		if explicit[key] {
			continue
		}
		nv := nextValues[key]
		prior, has := effective[key]
		was := overlayNormalize(prior, overlayChild(t, key))
		if keyed && has {
			was = overlayKept(was)
		}
		if reflect.DeepEqual(was, plain(nv, key)) {
			continue // absent and zero, or already inherited unchanged through `<<`
		}
		out.Content = append(out.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key, Style: keyStyle}, nv)
	}
	inherited := make([]string, 0, len(supplied))
	for key := range supplied {
		if !explicit[key] && nextValues[key] == nil {
			inherited = append(inherited, key)
		}
	}
	sort.Strings(inherited)
	for _, key := range inherited {
		out.Content = append(out.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key, Style: keyStyle}, null())
	}
	return out
}

// overlayLocate walks a mapping path. parent is the mapping holding the last
// key, index the key's position in parent.Content (-1 when absent).
func overlayLocate(root *yaml.Node, path []string) (parent *yaml.Node, index int, err error) {
	if len(root.Content) == 0 {
		return nil, -1, fmt.Errorf("Empty YAML document")
	}
	node := root.Content[0]
	for depth, key := range path {
		if node.Kind != yaml.MappingNode {
			return nil, -1, fmt.Errorf("%s is not a mapping", strings.Join(path[:depth], "."))
		}
		found := -1
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value == key {
				found = i
				break
			}
		}
		if depth == len(path)-1 {
			return node, found, nil
		}
		if found < 0 {
			return nil, -1, fmt.Errorf("Missing %s", key)
		}
		node = node.Content[found+1]
	}
	return nil, -1, fmt.Errorf("Empty YAML path")
}

// overlayEmptyParent reports a path ending in `{}` or a bare null value.
func overlayEmptyParent(root *yaml.Node, path []string) bool {
	parent, index, err := overlayLocate(root, path)
	if err != nil || index < 0 {
		return false
	}
	v := parent.Content[index+1]
	return v.Kind == yaml.MappingNode && len(v.Content) == 0 || v.Kind == yaml.ScalarNode && v.Tag == "!!null"
}

func overlayNodeLines(n *yaml.Node) (first, last int) {
	first, last = n.Line, n.Line
	var walk func(*yaml.Node)
	walk = func(x *yaml.Node) {
		if x.Line > last {
			last = x.Line
		}
		for _, c := range x.Content {
			walk(c)
		}
	}
	walk(n)
	return
}

// overlayFirstAlias finds the earliest alias of target outside [skipFirst, skipLast].
func overlayFirstAlias(root, target *yaml.Node, skipFirst, skipLast int) *yaml.Node {
	var best *yaml.Node
	var walk func(*yaml.Node)
	walk = func(x *yaml.Node) {
		if x.Kind == yaml.AliasNode && x.Alias == target && (x.Line < skipFirst || x.Line > skipLast) {
			if best == nil || x.Line < best.Line || x.Line == best.Line && x.Column < best.Column {
				best = x
			}
		}
		for _, c := range x.Content {
			walk(c)
		}
	}
	walk(root)
	return best
}

// overlayEncodeEntry renders `key: value` at the entry's indentation.
func overlayEncodeEntry(key string, value *yaml.Node, indent int, eol string) ([]byte, error) {
	value = overlayCopy(value, false)
	// Comments after the entry belong to the following text, which is kept.
	value.FootComment = ""
	node := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, value}}
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(node); err != nil {
		return nil, err
	}
	_ = enc.Close()
	var out bytes.Buffer
	for _, line := range strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n") {
		out.WriteString(strings.Repeat(" ", indent))
		out.WriteString(line)
		out.WriteString(eol)
	}
	return out.Bytes(), nil
}

type overlaySplice struct {
	start, end int
	text       []byte
}

// overlayMergePatch implements overlayPatch. A shared anchor whose content
// changes moves, unchanged, to its first alias: other entries keep their
// values and only this entry stops sharing.
func overlayMergePatch(data []byte, path []string, value any) ([]byte, error) {
	next := &yaml.Node{}
	if err := next.Encode(value); err != nil {
		return nil, err
	}
	if next.Kind == yaml.DocumentNode && len(next.Content) > 0 {
		next = next.Content[0]
	}
	return overlayMergeNode(data, path, next, reflect.TypeOf(value))
}

// overlayMergeNode patches the entry at path with next, an encoded value of Go type t.
func overlayMergeNode(data []byte, path []string, next *yaml.Node, t reflect.Type) ([]byte, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	if len(path) > 1 && overlayEmptyParent(&root, path[:len(path)-1]) {
		// `maps: {}` or a bare `maps:` holds no entries to splice next to:
		// write the parent itself as a block mapping holding the new entry.
		if overlayPlain(next, t) == nil {
			return data, nil
		}
		var parentType reflect.Type
		if t != nil {
			parentType = reflect.MapOf(reflect.TypeOf(""), t)
		}
		entry := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{{Kind: yaml.ScalarNode, Tag: "!!str", Value: path[len(path)-1]}, next}}
		return overlayMergeNode(data, path[:len(path)-1], entry, parentType)
	}
	parent, index, err := overlayLocate(&root, path)
	if err != nil {
		return nil, err
	}
	eol := "\n"
	if bytes.Contains(data, []byte("\r\n")) {
		eol = "\r\n"
	}
	key := path[len(path)-1]
	start, end, indent, found, err := overlayEntry(data, path)
	if err != nil {
		return nil, err
	}
	if index < 0 || !found {
		if overlayPlain(next, t) == nil {
			return data, nil
		}
		text, err := overlayEncodeEntry(key, next, indent, eol)
		if err != nil {
			return nil, err
		}
		return overlayApplySplices(data, []overlaySplice{{start, end, overlayLeadingBreak(data, start, eol, text)}})
	}
	old := parent.Content[index+1]
	if overlayNodeEqual(old, next, t) {
		return data, nil
	}
	m := &overlayMerger{}
	merged := m.merge(old, next, t)
	first, last := overlayNodeLines(parent.Content[index])
	if _, l := overlayNodeLines(old); l > last {
		last = l
	}
	splices := []overlaySplice{}
	if overlayPlain(merged, t) == nil {
		splices = append(splices, overlaySplice{start, end, nil})
	} else {
		parent.Content[index+1] = merged
	}
	// Resolve every alias site against the original line numbers first; a
	// relocated copy carries its anchor's old lines.
	type relocation struct {
		site, copy *yaml.Node
		at         int
	}
	var relocations []relocation
	for _, det := range m.detached {
		site := overlayFirstAlias(&root, det.original, first, last)
		if site == nil {
			continue // nothing else shares it; the anchor simply goes away
		}
		at := -1
		for i := 0; i+1 < len(parent.Content); i += 2 {
			if i == index {
				continue
			}
			f, l := overlayNodeLines(parent.Content[i])
			if _, vl := overlayNodeLines(parent.Content[i+1]); vl > l {
				l = vl
			}
			if site.Line >= f && site.Line <= l {
				at = i
				break
			}
		}
		if at < 0 {
			return nil, fmt.Errorf("Shared YAML anchor &%s is used outside %s; edit it in the YAML file", det.copy.Anchor, strings.Join(path[:len(path)-1], "."))
		}
		relocations = append(relocations, relocation{site, det.copy, at})
	}
	touched := map[int]bool{}
	for _, r := range relocations {
		*r.site = *r.copy
		touched[r.at] = true
	}
	for at := range touched {
		otherPath := append(append([]string(nil), path[:len(path)-1]...), parent.Content[at].Value)
		s, e, ind, ok, err := overlayEntry(data, otherPath)
		if err != nil || !ok {
			return nil, fmt.Errorf("Cannot relocate a shared YAML anchor into %s", strings.Join(otherPath, "."))
		}
		text, err := overlayEncodeEntry(parent.Content[at].Value, parent.Content[at+1], ind, eol)
		if err != nil {
			return nil, err
		}
		splices = append(splices, overlaySplice{s, e, text})
	}
	if overlayPlain(merged, t) != nil {
		text, err := overlayEncodeEntry(key, merged, indent, eol)
		if err != nil {
			return nil, err
		}
		splices = append(splices, overlaySplice{start, end, text})
	}
	return overlayApplySplices(data, splices)
}

func overlayLeadingBreak(data []byte, start int, eol string, text []byte) []byte {
	if start > 0 && data[start-1] != '\n' {
		return append([]byte(eol), text...)
	}
	return text
}

// Splices never overlap: each covers one sibling entry. Apply from the end so
// earlier offsets stay valid, then validate the actual result.
func overlayApplySplices(data []byte, splices []overlaySplice) ([]byte, error) {
	sort.Slice(splices, func(i, j int) bool { return splices[i].start > splices[j].start })
	out := append([]byte(nil), data...)
	for _, s := range splices {
		out = append(append(append([]byte(nil), out[:s.start]...), s.text...), out[s.end:]...)
	}
	var checked any
	if err := yaml.Unmarshal(out, &checked); err != nil {
		return nil, err
	}
	return out, nil
}

// overlayZero reports a value that would encode to nothing.
func overlayZero(v any) bool {
	n := &yaml.Node{}
	if n.Encode(v) != nil {
		return false
	}
	return overlayPlain(n, reflect.TypeOf(v)) == nil
}

// overlayRenameBoss rewrites requires_boss/type references from one boss
// archetype to another inside one entry and patches only that entry.
func overlayRenameBoss(data []byte, path []string, match func(map[string]any) bool, field, old, next string) ([]byte, bool, error) {
	var doc any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, false, err
	}
	entry := doc
	for _, key := range path {
		m, ok := entry.(map[string]any)
		if !ok {
			return data, false, nil
		}
		entry = m[key]
	}
	changed := false
	var walk func(any)
	walk = func(x any) {
		switch t := x.(type) {
		case map[string]any:
			if t[field] == old && match(t) {
				t[field] = next
				changed = true
			}
			for _, child := range t {
				walk(child)
			}
		case []any:
			for _, child := range t {
				walk(child)
			}
		}
	}
	walk(entry)
	if !changed {
		return data, false, nil
	}
	out, err := overlayPatch(data, path, entry)
	return out, true, err
}
