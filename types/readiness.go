package types

import (
	"sync"

	"github.com/theapemachine/datura"
)

/*
Readiness returns the readiness of the thesis for evaluation.

A stage is ready when it has stamped the thesis, and only then. A stamp is the
one statement a solver makes about itself having run to completion, so reading
anything else — the contents of its output map, the length of a slice it
happens to fill — infers readiness from a side effect and lets a stage that
legitimately produced nothing read as never having run.
*/
type Readiness struct {
	mu          sync.RWMutex
	ui          chan []byte
	Correlation bool `json:"correlation"`
	CVD         bool `json:"cvd"`
	DepthFlow   bool `json:"depth_flow"`
	Exhaustion  bool `json:"exhaustion"`
	Hawkes      bool `json:"hawkes"`
	LeadLag     bool `json:"lead_lag"`
	Liquidity   bool `json:"liquidity"`
	PumpDump    bool `json:"pump_dump"`
	Sentiment   bool `json:"sentiment"`
	Toxicity    bool `json:"toxicity"`
	Categories  bool `json:"categories"`
	Cognition   bool `json:"cognition"`
	Manifold    bool `json:"manifold"`
	Resonance   bool `json:"resonance"`
	Causal      bool `json:"causal"`
	Graph       bool `json:"graph"`
	Planner     bool `json:"planner"`
}

func NewReadiness(ui chan []byte) Readiness {
	return Readiness{
		ui: ui,
	}
}

/*
Stamp records completion of one concurrently running signal stage.
*/
func (readiness *Readiness) Stamp(source SourceType) {
	readiness.mu.Lock()
	defer readiness.mu.Unlock()

	didUpdate := false

	switch source {
	case SourceCorrelation:
		if !readiness.Correlation {
			readiness.Correlation = true
			didUpdate = true
		}
	case SourceCVD:
		if !readiness.CVD {
			readiness.CVD = true
			didUpdate = true
		}
	case SourceDepthFlow:
		if !readiness.DepthFlow {
			readiness.DepthFlow = true
			didUpdate = true
		}
	case SourceExhaustion:
		if !readiness.Exhaustion {
			readiness.Exhaustion = true
			didUpdate = true
		}
	case SourceHawkes:
		if !readiness.Hawkes {
			readiness.Hawkes = true
			didUpdate = true
		}
	case SourceLeadLag:
		if !readiness.LeadLag {
			readiness.LeadLag = true
			didUpdate = true
		}
	case SourceLiquidity:
		if !readiness.Liquidity {
			readiness.Liquidity = true
			didUpdate = true
		}
	case SourcePumpDump:
		if !readiness.PumpDump {
			readiness.PumpDump = true
			didUpdate = true
		}
	case SourceSentiment:
		if !readiness.Sentiment {
			readiness.Sentiment = true
			didUpdate = true
		}
	case SourceToxicity:
		if !readiness.Toxicity {
			readiness.Toxicity = true
			didUpdate = true
		}
	case SourceCategories:
		if !readiness.Categories {
			readiness.Categories = true
			didUpdate = true
		}
	case SourceCognition:
		if !readiness.Cognition {
			readiness.Cognition = true
			didUpdate = true
		}
	case SourceManifold:
		if !readiness.Manifold {
			readiness.Manifold = true
			didUpdate = true
		}
	case SourceResonance:
		if !readiness.Resonance {
			readiness.Resonance = true
			didUpdate = true
		}
	case SourceCausal:
		if !readiness.Causal {
			readiness.Causal = true
			didUpdate = true
		}
	case SourceGraph:
		if !readiness.Graph {
			readiness.Graph = true
			didUpdate = true
		}
	case SourcePlanner:
		if !readiness.Planner {
			readiness.Planner = true
			didUpdate = true
		}
	default:
		return
	}

	if readiness.ui != nil && didUpdate {
		select {
		case readiness.ui <- datura.NewMap(
			"readiness", readiness,
		).MarshalAndFree():
		default:
		}
	}
}

func (readiness *Readiness) SignalsMeasured() bool {
	readiness.mu.RLock()
	defer readiness.mu.RUnlock()

	return readiness.Correlation &&
		readiness.CVD &&
		readiness.DepthFlow &&
		readiness.Exhaustion &&
		readiness.Hawkes &&
		readiness.LeadLag &&
		readiness.Liquidity &&
		readiness.PumpDump &&
		readiness.Sentiment &&
		readiness.Toxicity
}

func (readiness *Readiness) LogicAnalyzed() bool {
	readiness.mu.RLock()
	defer readiness.mu.RUnlock()

	return readiness.Categories &&
		readiness.Cognition &&
		readiness.Manifold &&
		readiness.Resonance &&
		readiness.Causal &&
		readiness.Graph
}

func (readiness *Readiness) StrategyDecided() bool {
	readiness.mu.RLock()
	defer readiness.mu.RUnlock()

	return readiness.Planner
}

/*
Complete answers whether every stage that produces evidence has stamped this
tick, which is what a decision can be drawn from.

Allocation and Decisions are stamped by the decision pass rather than read by
it: orders are sized after a candidate has been judged, and a tick is only known
to have decided once it has. Requiring them here made the gate wait on the pass
it was gating, and since a tick that fails the gate is never reset, neither flag
could be raised on any later tick either.
*/
func (readiness *Readiness) Complete() bool {
	readiness.mu.RLock()
	defer readiness.mu.RUnlock()

	return readiness.SignalsMeasured() && readiness.LogicAnalyzed() && readiness.StrategyDecided()
}

/*
Reset clears every stage stamp for the next evaluation epoch.
*/
func (readiness *Readiness) Reset() {
	readiness.mu.Lock()
	defer readiness.mu.Unlock()

	readiness.Correlation = false
	readiness.CVD = false
	readiness.DepthFlow = false
	readiness.Exhaustion = false
	readiness.Hawkes = false
	readiness.LeadLag = false
	readiness.Liquidity = false
	readiness.PumpDump = false
	readiness.Sentiment = false
	readiness.Toxicity = false

	readiness.Categories = false
	readiness.Cognition = false
	readiness.Manifold = false
	readiness.Resonance = false
	readiness.Causal = false
	readiness.Graph = false

	readiness.Planner = false
}
