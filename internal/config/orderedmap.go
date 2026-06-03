// orderedmap.go — preserves YAML key insertion order across the
// `logmind config list/get/set` round-trip so the on-disk file matches
// what Python yaml.dump(sort_keys=False) produces.
//
// Go's standard yaml.v3 marshaler sorts map[string]any keys
// alphabetically. That would reorder a config-section the user just
// edited (e.g. adding `auto_rebase: true` to the `git:` section would
// alphabetize it) and break byte-identical parity with a Python-written
// file. OrderedMap solves this by implementing yaml.Marshaler /
// yaml.Unmarshaler so the encoder walks our key slice instead of the
// underlying map.
package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// OrderedMap is a simple insertion-ordered string→any map that
// round-trips through YAML preserving key order.
type OrderedMap struct {
	keys   []string
	values map[string]any
}

// NewOrderedMap returns a freshly initialised empty OrderedMap.
func NewOrderedMap() *OrderedMap {
	return &OrderedMap{values: map[string]any{}}
}

// Keys returns the keys in insertion order. The returned slice is a
// copy — callers may mutate it freely.
func (m *OrderedMap) Keys() []string {
	out := make([]string, len(m.keys))
	copy(out, m.keys)
	return out
}

// Get returns the value at key and a present-bool. Mirrors Go's
// idiomatic map access.
func (m *OrderedMap) Get(key string) (any, bool) {
	v, ok := m.values[key]
	return v, ok
}

// Set inserts or updates the value at key. New keys are appended;
// existing keys keep their position.
func (m *OrderedMap) Set(key string, value any) {
	if _, ok := m.values[key]; !ok {
		m.keys = append(m.keys, key)
	}
	m.values[key] = value
}

// MarshalYAML serialises the map preserving insertion order. Required
// for byte-identical parity with Python yaml.dump(sort_keys=False).
func (m *OrderedMap) MarshalYAML() (any, error) {
	node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, key := range m.keys {
		keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
		valNode, err := encodeValueAsNode(m.values[key])
		if err != nil {
			return nil, fmt.Errorf("ordered map key %q: %w", key, err)
		}
		node.Content = append(node.Content, keyNode, valNode)
	}
	return node, nil
}

// UnmarshalYAML accepts a mapping node and stores its entries in
// insertion order. Nested mappings are recursively wrapped in
// OrderedMap so deep paths preserve order through round-trips.
func (m *OrderedMap) UnmarshalYAML(node *yaml.Node) error {
	if m.values == nil {
		m.values = map[string]any{}
	}
	if node.Kind == yaml.DocumentNode && len(node.Content) == 1 {
		node = node.Content[0]
	}
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("expected mapping, got kind %d", node.Kind)
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valNode := node.Content[i+1]
		val, err := decodeNode(valNode)
		if err != nil {
			return err
		}
		m.Set(keyNode.Value, val)
	}
	return nil
}

// encodeValueAsNode converts a Go value into a yaml.Node. Lists are
// emitted as block-style sequences (matches Python's
// default_flow_style=False); maps are recursively encoded so nested
// OrderedMap order is preserved.
func encodeValueAsNode(v any) (*yaml.Node, error) {
	switch typed := v.(type) {
	case *OrderedMap:
		return typed.encodeAsNode()
	case []any:
		seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Style: yaml.LiteralStyle}
		// LiteralStyle on a sequence isn't a thing in yaml.v3 — set
		// 0 (default) to keep block style.
		seq.Style = 0
		for _, item := range typed {
			child, err := encodeValueAsNode(item)
			if err != nil {
				return nil, err
			}
			seq.Content = append(seq.Content, child)
		}
		return seq, nil
	default:
		node := &yaml.Node{}
		if err := node.Encode(v); err != nil {
			return nil, err
		}
		return node, nil
	}
}

func (m *OrderedMap) encodeAsNode() (*yaml.Node, error) {
	out, err := m.MarshalYAML()
	if err != nil {
		return nil, err
	}
	return out.(*yaml.Node), nil
}

// decodeNode converts a yaml.Node into a Go value. Mappings become
// *OrderedMap; sequences become []any; scalars use yaml.v3's default
// parse (bool, int, float, string).
func decodeNode(node *yaml.Node) (any, error) {
	switch node.Kind {
	case yaml.MappingNode:
		nested := NewOrderedMap()
		if err := nested.UnmarshalYAML(node); err != nil {
			return nil, err
		}
		return nested, nil
	case yaml.SequenceNode:
		var out []any
		for _, child := range node.Content {
			val, err := decodeNode(child)
			if err != nil {
				return nil, err
			}
			out = append(out, val)
		}
		return out, nil
	case yaml.ScalarNode:
		var v any
		if err := node.Decode(&v); err != nil {
			return nil, err
		}
		return v, nil
	case yaml.AliasNode:
		if node.Alias != nil {
			return decodeNode(node.Alias)
		}
		return nil, nil
	default:
		var v any
		if err := node.Decode(&v); err != nil {
			return nil, err
		}
		return v, nil
	}
}
