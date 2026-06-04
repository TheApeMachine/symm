package perspectives

import "go.yaml.in/yaml/v3"

var subjectNames = map[Subject]string{
	SubjectNone:     "none",
	SubjectSignal:   "signal",
	SubjectRegime:   "regime",
	SubjectPosition: "position",
	SubjectPrice:    "price",
	SubjectVolume:   "volume",
	SubjectSpread:   "spread",
	SubjectElapsed:  "elapsed",
}

var comparisonNames = map[Comparison]string{
	ComparisonNone:        "none",
	ComparisonAtLeast:     "at_least",
	ComparisonAtMost:      "at_most",
	ComparisonAbove:       "above",
	ComparisonBelow:       "below",
	ComparisonEquals:      "equals",
	ComparisonRoseBy:      "rose_by",
	ComparisonFellBy:      "fell_by",
	ComparisonCrossedUp:   "crossed_up",
	ComparisonCrossedDown: "crossed_down",
}

func (subject Subject) MarshalYAML() (any, error) {
	return marshalEnum(subject, subjectNames)
}

func (subject *Subject) UnmarshalYAML(node *yaml.Node) error {
	return unmarshalEnum(node, subject, subjectNames)
}

func (comparison Comparison) MarshalYAML() (any, error) {
	return marshalEnum(comparison, comparisonNames)
}

func (comparison *Comparison) UnmarshalYAML(node *yaml.Node) error {
	return unmarshalEnum(node, comparison, comparisonNames)
}

/*
UnmarshalYAML lets `do:` be a bare action ("do: iceberg") or the parameterized
object ("do: { type: stop_loss, offset: 0.015 }").
*/
func (act *Act) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		return node.Decode(&act.Type)
	}

	var raw struct {
		Type   ActionType `yaml:"type"`
		Offset float64    `yaml:"offset"`
	}

	if err := node.Decode(&raw); err != nil {
		return err
	}

	act.Type = raw.Type
	act.Offset = raw.Offset

	return nil
}

/*
MarshalYAML is the inverse of the reader above: a bare action ("do: iceberg") when
there is no per-node offset, and the object form ("do: { type: stop_loss, offset:
0.015 }") when there is, so a written playbook reads back identically.
*/
func (act Act) MarshalYAML() (any, error) {
	if act.Offset == 0 {
		return act.Type, nil
	}

	return struct {
		Type   ActionType `yaml:"type"`
		Offset float64    `yaml:"offset"`
	}{Type: act.Type, Offset: act.Offset}, nil
}

type reasoningDocument struct {
	Version  int       `yaml:"version"`
	Branches []Thought `yaml:"branches"`
}

/*
ParseThoughts decodes a version-2 playbook document into the reasoning forest.
*/
func ParseThoughts(raw []byte) ([]Thought, error) {
	var document reasoningDocument

	if err := yaml.Unmarshal(raw, &document); err != nil {
		return nil, err
	}

	return document.Branches, nil
}

/*
MarshalThoughts encodes a reasoning forest back into a playbook document — the
inverse of ParseThoughts. The optimizer writes the trees it discovers this way, and
a hand-written playbook round-trips through it unchanged.
*/
func MarshalThoughts(thoughts []Thought, version int) ([]byte, error) {
	return yaml.Marshal(reasoningDocument{Version: version, Branches: thoughts})
}
