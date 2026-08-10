package types

import (
	"sync"

	"github.com/theapemachine/datura"
)

/*
Readiness records which stages have evaluated one symbol.
*/
type Readiness struct {
	mu          sync.RWMutex
	ui          chan []byte
	Symbol      string `json:"symbol"`
	Correlation bool   `json:"correlation"`
	CVD         bool   `json:"cvd"`
	DepthFlow   bool   `json:"depth_flow"`
	Exhaustion  bool   `json:"exhaustion"`
	Hawkes      bool   `json:"hawkes"`
	LeadLag     bool   `json:"lead_lag"`
	Liquidity   bool   `json:"liquidity"`
	PumpDump    bool   `json:"pump_dump"`
	Sentiment   bool   `json:"sentiment"`
	Toxicity    bool   `json:"toxicity"`
	Categories  bool   `json:"categories"`
	Cognition   bool   `json:"cognition"`
	Manifold    bool   `json:"manifold"`
	Resonance   bool   `json:"resonance"`
	Causal      bool   `json:"causal"`
	Graph       bool   `json:"graph"`
	Planner     bool   `json:"planner"`
}

func NewReadiness(symbol string, ui chan []byte) Readiness {
	return Readiness{
		ui:     ui,
		Symbol: symbol,
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
	case SourceCategory:
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
			"readiness", []*Readiness{readiness},
		).MarshalAndFree():
		default:
		}
	}
}

/*
Stamped answers whether one stage has already run this epoch.

Every module is woken by the same broadcast, so each needs to recognise work it
has already done: a stage that ran, stamped, and notified would otherwise be
woken by its own notification and run again forever. Prerequisites-stamped and
self-not-stamped together are the whole of a module's admission test.
*/
func (readiness *Readiness) Stamped(source SourceType) bool {
	readiness.mu.RLock()
	defer readiness.mu.RUnlock()

	switch source {
	case SourceCorrelation:
		return readiness.Correlation
	case SourceCVD:
		return readiness.CVD
	case SourceDepthFlow:
		return readiness.DepthFlow
	case SourceExhaustion:
		return readiness.Exhaustion
	case SourceHawkes:
		return readiness.Hawkes
	case SourceLeadLag:
		return readiness.LeadLag
	case SourceLiquidity:
		return readiness.Liquidity
	case SourcePumpDump:
		return readiness.PumpDump
	case SourceSentiment:
		return readiness.Sentiment
	case SourceToxicity:
		return readiness.Toxicity
	case SourceCategory:
		return readiness.Categories
	case SourceCognition:
		return readiness.Cognition
	case SourceManifold:
		return readiness.Manifold
	case SourceResonance:
		return readiness.Resonance
	case SourceCausal:
		return readiness.Causal
	case SourceGraph:
		return readiness.Graph
	case SourcePlanner:
		return readiness.Planner
	default:
		return false
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

	return readiness.Correlation &&
		readiness.CVD &&
		readiness.DepthFlow &&
		readiness.Exhaustion &&
		readiness.Hawkes &&
		readiness.LeadLag &&
		readiness.Liquidity &&
		readiness.PumpDump &&
		readiness.Sentiment &&
		readiness.Toxicity &&
		readiness.Categories &&
		readiness.Cognition &&
		readiness.Manifold &&
		readiness.Resonance &&
		readiness.Causal &&
		readiness.Graph &&
		readiness.Planner
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

	if readiness.ui != nil {
		select {
		case readiness.ui <- datura.NewMap(
			"readiness", []*Readiness{readiness},
		).MarshalAndFree():
		default:
		}
	}
}
