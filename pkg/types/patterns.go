package types

import (
	"bytes"
	"encoding/csv"
	"path"
	"slices"
	"strings"

	"sigs.k8s.io/kustomize/kyaml/yaml"
)

type Patterns []string

func (p *Patterns) UnmarshalYAML(node *yaml.Node) error {
	if node == nil {
		*p = nil
		return nil
	}
	if node.Kind == yaml.ScalarNode {
		return p.unmarshalScalar(node)
	}
	items := []string{}
	if err := node.Decode(&items); err != nil {
		return err
	}
	return p.unmarshalList(items)
}

func (p *Patterns) unmarshalScalar(node *yaml.Node) error {
	var raw string
	if err := node.Decode(&raw); err != nil {
		return err
	}

	buf := bytes.NewBufferString(raw)
	r := csv.NewReader(buf)
	rawParts, err := r.ReadAll()
	if err != nil {
		return err
	}

	parts := slices.Concat(rawParts...)
	for i, part := range parts {
		parts[i] = strings.TrimSpace(part)
	}

	return p.unmarshalList(parts)
}

func (p *Patterns) unmarshalList(parts []string) error {
	for _, pattern := range parts {
		if _, err := path.Match(pattern, ""); err != nil {
			return err
		}
	}
	*p = parts
	return nil
}

func (p Patterns) Match(name string) bool {
	for _, pattern := range p {
		match, err := path.Match(pattern, name)
		if err != nil {
			panic(err)
		}
		if match {
			return true
		}
	}
	return false
}

type PatternSelector struct {
	Include Patterns `json:"include" yaml:"include"`
	Exclude Patterns `json:"exclude" yaml:"exclude"`
}

func (sel *PatternSelector) Select(names []string) []string {
	if 0 == max(len(sel.Include), len(sel.Exclude)) {
		return names
	}

	selected := map[string]struct{}{}
	for _, name := range names {
		if len(sel.Include) == 0 || sel.Include.Match(name) {
			selected[name] = struct{}{}
		}
	}
	for _, name := range names {
		if sel.Exclude.Match(name) {
			delete(selected, name)
		}
	}
	result := []string{}
	for _, name := range names {
		if _, match := selected[name]; match {
			result = append(result, name)
		}
	}
	return result
}

func (sel *PatternSelector) UnmarshalYAML(node *yaml.Node) error {
	if node == nil {
		*sel = PatternSelector{}
		return nil
	}
	if node.Kind != yaml.MappingNode {
		return sel.unmarshalPattern(node)
	}

	type selStruct PatternSelector
	var plain selStruct
	if err := node.Decode(&plain); err != nil {
		return err
	}
	*sel = (PatternSelector)(plain)
	return nil
}

func (sel *PatternSelector) unmarshalPattern(node *yaml.Node) error {
	var patterns Patterns
	if err := node.Decode(&patterns); err != nil {
		return err
	}
	for _, part := range patterns {
		part, not := strings.CutPrefix(part, "-")
		if not {
			sel.Exclude = append(sel.Exclude, part)
		} else {
			sel.Include = append(sel.Include, part)
		}
	}
	return nil
}
