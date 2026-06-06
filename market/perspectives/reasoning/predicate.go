package reasoning

import (
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives/types"
)

/*
Predicate is one composable condition. It is either a boolean of sub-predicates
(exactly one of All/Any/Not set) or a leaf comparison (the fields below it).

A leaf reads a Subject — optionally COMPARED TO `Ago` measurements ago, which is
what turns "X is high" into "X became high" / "A vs N moments ago rose by V". An
empty Ago (0) means "now".
*/
type Predicate struct {
	// Boolean composition — set exactly one for a compound predicate.
	All []Predicate `yaml:"all,omitempty"` // every operand holds (AND)
	Any []Predicate `yaml:"any,omitempty"` // at least one operand holds (OR)
	Not *Predicate  `yaml:"not,omitempty"` // the operand does not hold (NOT)

	// Leaf comparison — used when All/Any/Not are empty.
	Subject   Subject               `yaml:"subject,omitempty"`
	Category  types.CategoryType    `yaml:"category,omitempty"`  // Subject == SubjectSignal
	Regime    types.Regime          `yaml:"regime,omitempty"`    // Subject == SubjectRegime (target state)
	Lifecycle types.ObservationType `yaml:"lifecycle,omitempty"` // Subject == SubjectPosition (target state)
	Side      trading.Side          `yaml:"side,omitempty"`      // Subject == SubjectPosition (buy = long, sell = short)
	Unit      UnitType              `yaml:"unit,omitempty"`      // how Value reads: snr / confidence / percentage / time_*
	Ago       int                   `yaml:"ago,omitempty"`       // 0 = now; N = compared to N measurements ago
	Op        Comparison            `yaml:"op,omitempty"`

	// Right-hand side of the comparison: a static Value, OR another live subject
	// (Versus) for metric-to-metric gating — "this signal stronger than that one",
	// "volume rose while price stalled". Exactly one is used.
	Value  float64  `yaml:"value,omitempty"`
	Versus *Operand `yaml:"versus,omitempty"`
}

/*
Operand is a live right-hand side: another subject (optionally at its own Ago) to
compare the predicate's Subject against, instead of a static Value.
*/
type Operand struct {
	Subject  Subject            `yaml:"subject,omitempty"`
	Category types.CategoryType `yaml:"category,omitempty"`
	Unit     UnitType           `yaml:"unit,omitempty"`
	Ago      int                `yaml:"ago,omitempty"`
}

/*
Comparison relates the observed value to the target. The plain comparators are
level checks ("is"); rose_by/fell_by and crossed_up/crossed_down are temporal —
they use Ago and capture the "becomes" in "when A vs N moments ago becomes B".
*/
type Comparison uint8

const (
	ComparisonNone        Comparison = iota
	ComparisonAtLeast                // value >= target
	ComparisonAtMost                 // value <= target
	ComparisonAbove                  // value >  target
	ComparisonBelow                  // value <  target
	ComparisonEquals                 // value == target (regime / lifecycle / category match)
	ComparisonRoseBy                 // value(now) - value(now-Ago) >= target (Unit: abs or %)
	ComparisonFellBy                 // value(now-Ago) - value(now) >= target
	ComparisonCrossedUp              // value crossed above target within Ago (an EDGE)
	ComparisonCrossedDown            // value crossed below target within Ago (an EDGE)
)
