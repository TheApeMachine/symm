package reasoning

import (
	"time"

	"go.yaml.in/yaml/v3"
)

/*
Lookback selects how far back a predicate reads. Exactly one mode applies:
  - zero value -> now (index 0)
  - Ago > 0    -> the Nth most recent reading (event-indexed)
  - Within > 0 -> the reading at or before (now - Within) on the wall clock
*/
type Lookback struct {
	Ago    int
	Within time.Duration
}

/*
YAMLDuration unmarshals playbook durations such as "500ms" or "5s".
*/
type YAMLDuration time.Duration

func (duration *YAMLDuration) UnmarshalYAML(node *yaml.Node) error {
	var raw string

	if err := node.Decode(&raw); err != nil {
		return err
	}

	parsed, err := time.ParseDuration(raw)

	if err != nil {
		return err
	}

	*duration = YAMLDuration(parsed)

	return nil
}

func (duration YAMLDuration) Duration() time.Duration {
	return time.Duration(duration)
}

func lookbackFromPredicate(pred Predicate) Lookback {
	return Lookback{Ago: pred.Ago, Within: pred.Within.Duration()}
}

func lookbackFromOperand(operand Operand) Lookback {
	return Lookback{Ago: operand.Ago, Within: operand.Within.Duration()}
}
