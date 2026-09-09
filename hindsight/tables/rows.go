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
	ID             string            `json:"id"`
	StartedAt      time.Time         `json:"startedAt"`
	CodeCommit     string            `json:"codeCommit"`
	BuildID        string            `json:"buildId"`
	ConfigDigest   string            `json:"configDigest"`
	Integrity      string            `json:"integrity"`
	Positions      int32             `json:"positions"`
	SchemaVersions map[string]string `json:"schemaVersions,omitempty"`
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
	OrderID       string           `json:"orderId"`
	ClientOrderID string           `json:"clientOrderId"`
	ExecID        string           `json:"execId"`
	ExecType      string           `json:"execType"`
	TradeID       int64            `json:"tradeId"`
	Side          string           `json:"side"`
	OrderType     string           `json:"orderType"`
	OrderStatus   string           `json:"orderStatus"`
	LiquidityInd  string           `json:"liquidityInd"`
	At            time.Time        `json:"fillAt"`
	LastQty       *decimal.Decimal `json:"lastQty,omitempty"`
	LastPrice     *decimal.Decimal `json:"lastPrice,omitempty"`
	Cost          *decimal.Decimal `json:"cost,omitempty"`
	CumQty        *decimal.Decimal `json:"cumQty,omitempty"`
	CumCost       *decimal.Decimal `json:"cumCost,omitempty"`
	AvgPrice      *decimal.Decimal `json:"avgPrice,omitempty"`
	FeeUsdEquiv   *decimal.Decimal `json:"feeUsdEquiv,omitempty"`
	Fees          string           `json:"fees,omitempty"`
}

// LifecycleRow is one position or order transition. Exec is nil for position
// open and close events, which carry no execution fact.
type LifecycleRow struct {
	Run                 string        `json:"run"`
	DecisionID          string        `json:"decisionId"`
	ActionCorrelationID string        `json:"actionCorrelationId"`
	Symbol              string        `json:"symbol"`
	Kind                string        `json:"kind"`
	Action              string        `json:"action"`
	At                  time.Time     `json:"at"`
	CaptureSeq          int64         `json:"captureSeq"`
	Exec                *ExecutionRow `json:"execution,omitempty"`
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
	Run      string `json:"runId"`
	Sequence int64  `json:"sequence"`
	Encoding string `json:"encoding"`
}
