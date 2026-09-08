package tables

import (
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
)

/*
The row types below are this package's own vocabulary, deliberately free of
hindsight and strategy types. Both of those packages need to read these tables,
so importing either here would close a dependency cycle; callers convert their
records into these rows instead.

A zero time.Time encodes as null, and a nil pointer encodes as null. Optional
columns in the schemas correspond exactly to those two cases.
*/

// EnvelopeRefRow names one Workspace Envelope: the raw input it came from and
// its deterministic ordinal within that input.
type EnvelopeRefRow struct {
	Run      string
	Sequence int64
	Ordinal  int64
}

// RunRow is one process capture session.
type RunRow struct {
	ID             string
	StartedAt      time.Time
	CodeCommit     string
	BuildID        string
	ConfigDigest   string
	Integrity      string
	Positions      int32
	SchemaVersions map[string]string
}

// CaptureRow is one raw external input exactly as it arrived.
type CaptureRow struct {
	Run            string
	Sequence       int64
	Stream         string
	StreamEpoch    int64
	StreamSequence int64
	ReceivedAt     time.Time
	Endpoint       string
	Kind           string
	PayloadHash    string
	Payload        []byte
}

// ManifestRow records how one raw frame entered Workspace.
type ManifestRow struct {
	Run           string
	Envelope      EnvelopeRefRow
	Workload      string
	DomainKind    string
	Symbol        string
	VenueAt       time.Time
	VenueSequence string
}

// WitnessRow is evidence of what the running binary produced at one boundary.
type WitnessRow struct {
	Run                   string
	Envelope              EnvelopeRefRow
	Boundary              string
	ArtifactKind          string
	ArtifactIdentity      string
	ArtifactKindLabel     string
	ProducedAt            time.Time
	Component             string
	ComponentStateVersion int64
	ImmediateParents      []EnvelopeRefRow
	SemanticParents       []string
	Payload               []byte
}

// ExecutionRow carries the venue's authoritative economics for one execution.
type ExecutionRow struct {
	OrderID       string
	ClientOrderID string
	ExecID        string
	ExecType      string
	TradeID       int64
	Side          string
	OrderType     string
	OrderStatus   string
	LiquidityInd  string
	At            time.Time
	LastQty       *decimal.Decimal
	LastPrice     *decimal.Decimal
	Cost          *decimal.Decimal
	CumQty        *decimal.Decimal
	CumCost       *decimal.Decimal
	AvgPrice      *decimal.Decimal
	FeeUsdEquiv   *decimal.Decimal
	Fees          string
}

// LifecycleRow is one position or order transition. Exec is nil for position
// open and close events, which carry no execution fact.
type LifecycleRow struct {
	Run                 string
	DecisionID          string
	ActionCorrelationID string
	Symbol              string
	Kind                string
	Action              string
	At                  time.Time
	CaptureSeq          int64
	Exec                *ExecutionRow
}

// OutcomeRow is one graded decision.
type OutcomeRow struct {
	Run          string
	DecisionID   int64
	Trader       int32
	Label        string
	At           time.Time
	ActionKind   string
	ActionPower  int32
	ActionReduce bool
	Authority    float64
	Outcome      *float64
	Context      []int64
	Through      time.Time
	Value        float64
	Complete     bool
	Forced       bool
	Initial      *decimal.Decimal
	Reference    *decimal.Decimal
	Quantity     *decimal.Decimal
	Cost         *decimal.Decimal
	Fee          *decimal.Decimal
	Opportunity  *decimal.Decimal
}

// GapRow marks one place where a run's capture is known to be incomplete.
type GapRow struct {
	Run      string
	Sequence int64
	Encoding string
}
