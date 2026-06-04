package perspectives

/*
This file is the PROPOSED tree language — a ground-up redesign of Branch so the
playbook can express a thought process, not a one-step reflex. It is intentionally
parallel to (and unused by) the current Branch/evaluator: we agree the shape here
first, then migrate the interpreter, optimizer, and encoding onto it.

It folds the three condition types that were being sketched into one:
  - ConditionBooleanType (And/Or)              -> Predicate.All / Predicate.Any / Predicate.Not
  - Observed{ObservedType, Values, ...}        -> Predicate leaf (Subject + Ago + Op + Value)
  - Observation{ObservationType, ..., Branch}  -> Thought.When (subject: position) + Thought.Then

A Thought is one node in the reasoning:

    when: <predicate>   the condition that makes this thought relevant
    then: [<thought>]   the reasoning that follows ONCE `when` holds — these are
                        monitored on the ticks that FOLLOW, so `then` reads as
                        "and then, over time, watch for ...". This is what makes
                        depth a temporal sequence instead of a snapshot conjunction.
    do:   <action>      the decision taken here, if any. A node may both `do` and
                        `then` — "act, and keep thinking" (enter, then manage).
*/
type Thought struct {
	When Predicate `yaml:"when"`
	Then []Thought `yaml:"then,omitempty"`
	Do   Act       `yaml:"do,omitempty"`
}

/*
Act is the decision at a node. Offset lets a single node override the global
protective threshold for the trade it manages — a short scalp can carry a tight
stop and a long accumulation a wide one, which a global ReplayCosts.StopLossPct
cannot express (this is the per-action parameter the analysis correctly flagged).
The YAML accepts a bare action for the no-parameter case ("do: iceberg") or the
object form ("do: { type: stop_loss, offset: 0.015 }").
*/
type Act struct {
	Type   ActionType `yaml:"type"`
	Offset float64    `yaml:"offset,omitempty"` // overrides the global stop/take/trail fraction for this node (0 = use global)
}

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
	Subject   Subject         `yaml:"subject,omitempty"`
	Category  CategoryType    `yaml:"category,omitempty"`  // Subject == SubjectSignal
	Regime    Regime          `yaml:"regime,omitempty"`    // Subject == SubjectRegime (target state)
	Lifecycle ObservationType `yaml:"lifecycle,omitempty"` // Subject == SubjectPosition (target state)
	Unit      UnitType        `yaml:"unit,omitempty"`      // how Value reads: snr / confidence / percentage / time_*
	Ago       int             `yaml:"ago,omitempty"`       // 0 = now; N = compared to N measurements ago
	Op        Comparison      `yaml:"op,omitempty"`

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
	Subject  Subject      `yaml:"subject,omitempty"`
	Category CategoryType `yaml:"category,omitempty"`
	Unit     UnitType     `yaml:"unit,omitempty"`
	Ago      int          `yaml:"ago,omitempty"`
}

/*
Subject is what a leaf predicate observes. It widens the vocabulary past category
signals so a thought can reason about price action, participation, the clock, and
the trade's own life — the things a trader actually watches.
*/
type Subject uint8

const (
	SubjectNone     Subject = iota
	SubjectSignal           // a microstructure category's strength (pairs with Category + Unit snr|confidence)
	SubjectRegime           // the price-action regime (target in Regime)
	SubjectPosition         // the trade's lifecycle (target in Lifecycle: not_holding|holding|has_started|has_continued|has_ended)
	SubjectPrice            // last traded price (Unit percentage for relative moves)
	SubjectVolume           // traded volume
	SubjectSpread           // quoted spread (bps)
	SubjectElapsed          // time held in the current position (Unit time_minutes|time_seconds)
)

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
