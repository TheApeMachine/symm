package strategy

import (
	"context"
	"math"
	"strings"
	"sync"

	"github.com/theapemachine/nomagique/data"
	nmstore "github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/system"
)

// virtualPromoteWins is how many consecutive virtual wins earn paper from the tree.
const virtualPromoteWins = 3

// GateDecision conveys whether an action can proceed, and if it must be Virtual.
type GateDecision struct {
	Allowed bool   // false = blocked by gate
	Virtual bool   // true = sandbox only, false = permitted for Paper
	Reason  string // diagnostic reason
}

/*
Gate learns environmental hurdles from virtual and paper round trips and bounds
each to configured [floor, ceil] rails. Cold-start seeds live in gate.bin after
the first session; there are no runtime feature toggles.

Core hurdles (paper vs net vectors):

	ignition  — minimum netHold to earn paper
	risk      — maximum risk excess over netSpark tolerated for paper
	rejection — minimum exit force to harvest

Regime hurdles (learned from virtual outcomes; entry snapshot on the lot):

	opposition      — R02 brightness that triggers starved-tape block when netHold is weak
	regimeIgnition  — minimum R06/R07 brightness for paper
	regimeFriction  — R12 brightness above which paper entry is blocked
	maturityMin     — minimum mean grid maturity for paper
*/
type Gate struct {
	ctx   context.Context
	desk  *broker.Desk
	price *broker.Price
	store *store.Client
	mu    sync.RWMutex

	ignition  float64
	risk      float64
	rejection float64

	opposition      float64
	regimeIgnition  float64
	regimeFriction  float64
	maturityMin     float64

	fuzzyMatch          float64
	minTokenBright      float64
	salienceRatio       float64
	emergencyExitBright float64
	timeoutLossPenalty  float64
	pathPruneBook       float64
	stopDamageRatio     float64
	evaporationMult     float64
	coderExitScale      float64

	ignitionFloor, ignitionCeil   float64
	riskFloor, riskCeil           float64
	rejectionFloor, rejectionCeil float64
	oppositionFloor, oppositionCeil float64
	regimeIgnitionFloor, regimeIgnitionCeil float64
	regimeFrictionFloor, regimeFrictionCeil float64
	maturityFloor, maturityCeil             float64
	fuzzyFloor, fuzzyCeil                   float64
	minTokenFloor, minTokenCeil             float64
	salienceFloor, salienceCeil             float64
	emergencyFloor, emergencyCeil           float64
	timeoutPenaltyFloor, timeoutPenaltyCeil float64
	pathPruneFloor, pathPruneCeil           float64
	stopDamageFloor, stopDamageCeil         float64
	evapMultFloor, evapMultCeil             float64
	coderExitFloor, coderExitCeil           float64
	rate                          float64
}

// GateResult is the settled round trip the gate learns from: the realized
// fee-net return, how the position exited, and the enter frame snapshot.
type GateResult struct {
	ReturnFraction float64 // realized fee-net fraction of cost (+win / -loss)
	StoppedOut     bool    // exited by an invalidation/evaporation stop, not a harvest
	Virtual        bool    // sandbox lot (primary learning mass for regime hurdles)
	EnterR02       float64 // R02 brightness at open
	EnterNetHold   float64 // hold ignition net at open
	GridMaturity   float64 // mean enter-story grid maturity at open
	ExitTrigger    string  // desk stop / cognitive trigger at settle
}

func NewGate(
	ctx context.Context,
	desk *broker.Desk,
	price *broker.Price,
	client *store.Client,
) *Gate {
	rate := 0.10
	ignitionFloor, ignitionCeil := 0.15, 1.0
	riskFloor, riskCeil := 0.5, 5.0
	rejectionFloor, rejectionCeil := 0.3, 4.0
	oppositionFloor, oppositionCeil := 0.05, 0.95
	regimeIgnitionFloor, regimeIgnitionCeil := 0.05, 0.95
	regimeFrictionFloor, regimeFrictionCeil := 0.05, 0.95
	maturityFloor, maturityCeil := 0.05, 0.95

	opposition := 0.25
	regimeIgnition := 0.25
	regimeFriction := 0.25
	maturityMin := 0.35
	fuzzyMatch := 0.75
	minTokenBright := 0.25
	salienceRatio := 0.40
	emergencyExitBright := 0.75
	timeoutLossPenalty := 0.02
	pathPruneBook := 0.80
	stopDamageRatio := 0.75
	evaporationMult := 2.0
	coderExitScale := 1.0

	fuzzyFloor, fuzzyCeil := 0.40, 0.95
	minTokenFloor, minTokenCeil := 0.05, 0.60
	salienceFloor, salienceCeil := 0.15, 0.85
	emergencyFloor, emergencyCeil := 0.40, 1.0
	timeoutPenaltyFloor, timeoutPenaltyCeil := 0.005, 0.08
	pathPruneFloor, pathPruneCeil := 0.55, 0.95
	stopDamageFloor, stopDamageCeil := 0.50, 0.95
	evapMultFloor, evapMultCeil := 1.25, 4.0
	coderExitFloor, coderExitCeil := 0.50, 2.50

	if cfg := system.Cfg.Learning; cfg != nil {
		if cfg.GateHurdleRate > 0 {
			rate = cfg.GateHurdleRate
		}
		ignitionFloor, ignitionCeil = cfg.GateIgnitionFloor, cfg.GateIgnitionCeil
		riskFloor, riskCeil = cfg.GateRiskFloor, cfg.GateRiskCeil
		rejectionFloor, rejectionCeil = cfg.GateRejectionFloor, cfg.GateRejectionCeil
		if cfg.GateOppositionFloor > 0 {
			oppositionFloor = cfg.GateOppositionFloor
		}
		if cfg.GateOppositionCeil > 0 {
			oppositionCeil = cfg.GateOppositionCeil
		}
		if cfg.GateRegimeIgnitionFloor > 0 {
			regimeIgnitionFloor = cfg.GateRegimeIgnitionFloor
		}
		if cfg.GateRegimeIgnitionCeil > 0 {
			regimeIgnitionCeil = cfg.GateRegimeIgnitionCeil
		}
		if cfg.GateRegimeFrictionFloor > 0 {
			regimeFrictionFloor = cfg.GateRegimeFrictionFloor
		}
		if cfg.GateRegimeFrictionCeil > 0 {
			regimeFrictionCeil = cfg.GateRegimeFrictionCeil
		}
		if cfg.GateMaturityFloor > 0 {
			maturityFloor = cfg.GateMaturityFloor
		}
		if cfg.GateMaturityCeil > 0 {
			maturityCeil = cfg.GateMaturityCeil
		}
	}

	gate := &Gate{
		ctx:              ctx,
		desk:             desk,
		price:            price,
		store:            client,
		ignition:         1.0,
		risk:             1.0,
		rejection:        1.0,
		opposition:       opposition,
		regimeIgnition:   regimeIgnition,
		regimeFriction:   regimeFriction,
		maturityMin:           maturityMin,
		fuzzyMatch:            fuzzyMatch,
		minTokenBright:        minTokenBright,
		salienceRatio:         salienceRatio,
		emergencyExitBright:   emergencyExitBright,
		timeoutLossPenalty:    timeoutLossPenalty,
		pathPruneBook:         pathPruneBook,
		stopDamageRatio:       stopDamageRatio,
		evaporationMult:       evaporationMult,
		coderExitScale:        coderExitScale,
		fuzzyFloor:            fuzzyFloor,
		fuzzyCeil:             fuzzyCeil,
		minTokenFloor:         minTokenFloor,
		minTokenCeil:          minTokenCeil,
		salienceFloor:         salienceFloor,
		salienceCeil:          salienceCeil,
		emergencyFloor:        emergencyFloor,
		emergencyCeil:         emergencyCeil,
		timeoutPenaltyFloor:   timeoutPenaltyFloor,
		timeoutPenaltyCeil:    timeoutPenaltyCeil,
		pathPruneFloor:        pathPruneFloor,
		pathPruneCeil:         pathPruneCeil,
		stopDamageFloor:       stopDamageFloor,
		stopDamageCeil:        stopDamageCeil,
		evapMultFloor:         evapMultFloor,
		evapMultCeil:          evapMultCeil,
		coderExitFloor:        coderExitFloor,
		coderExitCeil:         coderExitCeil,
		ignitionFloor:    ignitionFloor,
		ignitionCeil:     ignitionCeil,
		riskFloor:        riskFloor,
		riskCeil:         riskCeil,
		rejectionFloor:   rejectionFloor,
		rejectionCeil:    rejectionCeil,
		oppositionFloor:  oppositionFloor,
		oppositionCeil:   oppositionCeil,
		regimeIgnitionFloor: regimeIgnitionFloor,
		regimeIgnitionCeil:  regimeIgnitionCeil,
		regimeFrictionFloor: regimeFrictionFloor,
		regimeFrictionCeil:  regimeFrictionCeil,
		maturityFloor:       maturityFloor,
		maturityCeil:        maturityCeil,
		rate:                rate,
	}

	gate.restore()
	gate.publishStops()
	return gate
}

func (gate *Gate) publishStops() {
	if gate == nil {
		return
	}
	gate.mu.RLock()
	damage := gate.stopDamageRatio
	evap := gate.evaporationMult
	gate.mu.RUnlock()
	broker.LearnedStops.Set(damage, evap)
}

func (gate *Gate) restore() {
	if gate == nil || gate.store == nil {
		return
	}

	value := gate.store.Download(gate.ctx, "model/parameters/gate.bin")
	if value == nil || value.Err != nil {
		return
	}

	gate.mu.Lock()
	if h := value.GetFloat64("ignition_hurdle"); h > 0 {
		gate.ignition = h
	}
	if r := value.GetFloat64("risk_tolerance"); r > 0 {
		gate.risk = r
	}
	if rej := value.GetFloat64("rejection_hurdle"); rej > 0 {
		gate.rejection = rej
	}
	if v := value.GetFloat64("opposition_hurdle"); v > 0 {
		gate.opposition = v
	}
	if v := value.GetFloat64("regime_ignition_hurdle"); v > 0 {
		gate.regimeIgnition = v
	}
	if v := value.GetFloat64("regime_friction_hurdle"); v > 0 {
		gate.regimeFriction = v
	}
	if v := value.GetFloat64("maturity_hurdle"); v > 0 {
		gate.maturityMin = v
	}
	if v := value.GetFloat64("fuzzy_match"); v > 0 {
		gate.fuzzyMatch = v
	}
	if v := value.GetFloat64("min_token_brightness"); v > 0 {
		gate.minTokenBright = v
	}
	if v := value.GetFloat64("salience_ratio"); v > 0 {
		gate.salienceRatio = v
	}
	if v := value.GetFloat64("emergency_exit_brightness"); v > 0 {
		gate.emergencyExitBright = v
	}
	if v := value.GetFloat64("timeout_loss_penalty"); v > 0 {
		gate.timeoutLossPenalty = v
	}
	if v := value.GetFloat64("path_prune_book"); v > 0 {
		gate.pathPruneBook = v
	}
	if v := value.GetFloat64("stop_damage_ratio"); v > 0 {
		gate.stopDamageRatio = v
	}
	if v := value.GetFloat64("evaporation_mult"); v > 0 {
		gate.evaporationMult = v
	}
	if v := value.GetFloat64("coder_exit_scale"); v > 0 {
		gate.coderExitScale = v
	}
	gate.mu.Unlock()
}

func (gate *Gate) persist() {
	if gate.store == nil {
		return
	}

	gate.mu.RLock()
	value := data.NewValue("model", "parameters", "gate")
	value.Set(gate.ignition, "ignition_hurdle")
	value.Set(gate.risk, "risk_tolerance")
	value.Set(gate.rejection, "rejection_hurdle")
	value.Set(gate.opposition, "opposition_hurdle")
	value.Set(gate.regimeIgnition, "regime_ignition_hurdle")
	value.Set(gate.regimeFriction, "regime_friction_hurdle")
	value.Set(gate.maturityMin, "maturity_hurdle")
	value.Set(gate.fuzzyMatch, "fuzzy_match")
	value.Set(gate.minTokenBright, "min_token_brightness")
	value.Set(gate.salienceRatio, "salience_ratio")
	value.Set(gate.emergencyExitBright, "emergency_exit_brightness")
	value.Set(gate.timeoutLossPenalty, "timeout_loss_penalty")
	value.Set(gate.pathPruneBook, "path_prune_book")
	value.Set(gate.stopDamageRatio, "stop_damage_ratio")
	value.Set(gate.evaporationMult, "evaporation_mult")
	value.Set(gate.coderExitScale, "coder_exit_scale")
	gate.mu.RUnlock()
	gate.store.WriteAtPrefix("model/parameters/gate.bin", value.Bytes())
}

func (gate *Gate) oppositionBlockMin() float64 {
	if gate == nil {
		return 0.25
	}
	gate.mu.RLock()
	defer gate.mu.RUnlock()
	return gate.opposition
}

func (gate *Gate) regimeIgnitionMin() float64 {
	if gate == nil {
		return 0.25
	}
	gate.mu.RLock()
	defer gate.mu.RUnlock()
	return gate.regimeIgnition
}

func (gate *Gate) regimeFrictionMin() float64 {
	if gate == nil {
		return 0.25
	}
	gate.mu.RLock()
	defer gate.mu.RUnlock()
	return gate.regimeFriction
}

func (gate *Gate) gridMaturityMin() float64 {
	if gate == nil {
		return 0.35
	}
	gate.mu.RLock()
	defer gate.mu.RUnlock()
	return gate.maturityMin
}

func (gate *Gate) fuzzyMatchMin() float64 {
	if gate == nil {
		return 0.75
	}
	gate.mu.RLock()
	defer gate.mu.RUnlock()
	return gate.fuzzyMatch
}

func (gate *Gate) minTokenBrightness() float64 {
	if gate == nil {
		return 0.25
	}
	gate.mu.RLock()
	defer gate.mu.RUnlock()
	return gate.minTokenBright
}

func (gate *Gate) salienceMin() float64 {
	if gate == nil {
		return 0.40
	}
	gate.mu.RLock()
	defer gate.mu.RUnlock()
	return gate.salienceRatio
}

func (gate *Gate) emergencyExitBrightness() float64 {
	if gate == nil {
		return 0.75
	}
	gate.mu.RLock()
	defer gate.mu.RUnlock()
	return gate.emergencyExitBright
}

func (gate *Gate) pathPruneThreshold() float64 {
	if gate == nil {
		return 0.80
	}
	gate.mu.RLock()
	defer gate.mu.RUnlock()
	return gate.pathPruneBook
}

func (gate *Gate) coderExitVeto(exitForce, confidence, risk float64, bullish bool) bool {
	if gate == nil || !bullish || confidence <= 0 || risk >= 1.0 {
		return false
	}
	gate.mu.RLock()
	scale := gate.coderExitScale
	gate.mu.RUnlock()
	return exitForce < scale*(1.0+confidence)
}

func clamp(v, floor, ceil float64) float64 {
	if v < floor {
		return floor
	}
	if v > ceil {
		return ceil
	}
	return v
}

/*
EvaluateEnter inspects entry invariants and environmental posture.
Physical impossibilities block entirely; environmental doubts relegate to Virtual.
*/
func (gate *Gate) EvaluateEnter(
	symbol string,
	observed []*nmstore.Region,
	coder CoderReading,
) GateDecision {
	forceVirtual := false
	if !coder.Bullish {
		// Bearish or flat forward curve: virtual path learning only, never paper.
		forceVirtual = true
	} else if !coder.Ready {
		forceVirtual = true
	}

	if gate.desk != nil && !gate.desk.CanSize(symbol) {
		return GateDecision{Allowed: false, Reason: "insufficient book liquidity (CanSize failed)"}
	}

	// PHYSICAL INVARIANT: book integrity at the lot size we would actually trade.
	if gate.price != nil {
		if spreadPct, ok := gate.price.SpreadPct(symbol); ok && spreadPct != nil {
			if spreadPct.Float64() > 4.0 {
				return GateDecision{Allowed: false, Reason: "blown-out spread: bid-ask spread exceeds 4.0%"}
			}
		}

		if closeNow, ok := gate.price.EntryCloseNowPnLPct(symbol); ok && closeNow != nil {
			if budget, ok := gate.price.OpenDragBudgetPct(symbol); ok && budget != nil {
				if closeNow.Float64() < -budget.Float64() {
					return GateDecision{
						Allowed: false,
						Reason:  "entry close-now drag exceeds round-trip budget (would open underwater)",
					}
				}
			}
		}
	}

	brightness := regionBrightness(observed)
	netSpark, netHold := entryNets(brightness)

	risk := max(brightness[12], brightness[13], brightness[14]) // R12-R14: Risk Oracle

	gate.mu.RLock()
	ignitionHurdle := gate.ignition
	riskHurdle := gate.risk
	gate.mu.RUnlock()

	lacksIgnition := netHold < ignitionHurdle
	riskDominant := (risk - netSpark) > riskHurdle

	if netSpark <= 0 {
		return GateDecision{
			Allowed: false,
			Reason:  "enter band net negative (detector+support does not beat opposition)",
		}
	}

	if lacksIgnition && brightness[2] >= gate.oppositionBlockMin() {
		return GateDecision{
			Allowed: false,
			Reason:  "enter opposition with insufficient hold ignition (starved or hostile tape)",
		}
	}

	if forceVirtual {
		return GateDecision{Allowed: true, Virtual: true, Reason: "coder uncalibrated; virtual exploration only"}
	}

	frictionMin := gate.regimeFrictionMin()
	if brightness[12] >= frictionMin {
		return GateDecision{
			Allowed: false,
			Reason:  "board friction hurdle (R12): spread cost dominates entry",
		}
	}

	ignitionMin := gate.regimeIgnitionMin()
	if brightness[6] < ignitionMin || brightness[7] < ignitionMin {
		return GateDecision{
			Allowed: false,
			Reason:  "hold ignition not lit (R06/R07 below minimum — tape/book not supportive yet)",
		}
	}

	if lacksIgnition || riskDominant {
		reason := "environmental caution: "
		if lacksIgnition {
			reason += "low ignition; "
		}
		if riskDominant {
			reason += "risk dominates net spark; "
		}
		return GateDecision{Allowed: true, Virtual: true, Reason: reason}
	}

	return GateDecision{Allowed: true, Virtual: false}
}

func (gate *Gate) EvaluateExit(
	symbol string,
	observed []*nmstore.Region,
	coder CoderReading,
	position *broker.Position,
) GateDecision {
	if position == nil {
		return GateDecision{Allowed: false, Reason: "no open position"}
	}

	inProfit := position.PnL != nil && position.PnL.Sign() > 0
	if position.PnLPct != nil && position.PnLPct.Float64() <= 0 {
		inProfit = false
	}

	if !inProfit {
		return GateDecision{Allowed: false, Reason: "trade underwater (downside guarded by stoploss)"}
	}

	brightness := regionBrightness(observed)

	// NET REJECTION: story exit band + board exit confidence (R11).
	netRejection := (brightness[3] + brightness[4] + brightness[11]) - brightness[5]

	// EXHAUSTION: hold opposition (R08) alone — board deadness is not wired here.
	exhaustion := brightness[8]

	// TOTAL EXIT FORCE
	exitForce := math.Max(netRejection, exhaustion)

	// RISK ORACLE: R12 (Friction), R13 (Contagion), R14 (Capacity/Liquidity)
	risk := max(brightness[12], brightness[13], brightness[14])
	capacityRisk := brightness[14]

	// SAFETY VETO: If R14 is screaming, executing a market sell right now will
	// cause excessive slippage and crash the book. Block the cognitive exit and
	// let the StopLoss VWAP-evaporation logic manage the exit safely.
	if capacityRisk > 1.0 && exitForce < capacityRisk {
		return GateDecision{
			Allowed: false,
			Reason:  "exit capacity risk (R14): executing market sell would crash the book",
		}
	}

	// Test A: Has the exit force overcome the learned (bounded) hurdle?
	// (If Macro Contagion R13 is high, we bypass the hurdle and get out)
	gate.mu.RLock()
	rejectionHurdle := gate.rejection
	gate.mu.RUnlock()

	if exitForce < rejectionHurdle && risk < 1.0 {
		return GateDecision{
			Allowed: false,
			Reason:  "exit force below learned hurdle (bids absorbing sells / tape alive)",
		}
	}

	if gate.coderExitVeto(exitForce, coder.Confidence, risk, coder.Bullish) {
		return GateDecision{
			Allowed: false,
			Reason:  "coder forward curve remains bullish (letting runner develop)",
		}
	}

	return GateDecision{Allowed: true, Reason: "exit hurdle cleared"}
}

/*
Update takes one bounded step on every hurdle toward the target the settled
round trip implies, then persists. The return is clamped to [-1,1] before
scaling so a single outsized move cannot run a hurdle out of its band.
*/
func (gate *Gate) Update(result GateResult) {
	r := clamp(result.ReturnFraction, -1.0, 1.0)
	if result.ExitTrigger != "" &&
		(strings.Contains(result.ExitTrigger, "expired") || strings.Contains(result.ExitTrigger, "expiry")) {
		gate.mu.RLock()
		penalty := gate.timeoutLossPenalty
		gate.mu.RUnlock()
		r = clamp(r-penalty, -1.0, 1.0)
	}
	eta := gate.rate

	gate.mu.Lock()

	// ignition: a win lowers the floor (easier), a loss raises it (harder).
	gate.ignition = clamp(gate.ignition-eta*r, gate.ignitionFloor, gate.ignitionCeil)

	// risk: same direction as ignition for entries (a win means tolerate less).
	gate.risk = clamp(gate.risk-eta*r, gate.riskFloor, gate.riskCeil)

	// rejection: a harvested win RAISES the required force (hold longer); a
	// stop-out lowers it (take profits sooner). This is the inverse of the old
	// exp(-return) product, which lowered rejection on wins and clipped them.
	dir := r
	if result.StoppedOut {
		dir = -math.Abs(r)
	}
	gate.rejection = clamp(gate.rejection+eta*dir, gate.rejectionFloor, gate.rejectionCeil)

	// Regime hurdles: virtual round trips supply most of the learning mass.
	weight := 1.0
	if result.Virtual {
		weight = 1.0
	}
	regimeStep := eta * weight * 0.5

	gate.regimeIgnition = clamp(
		gate.regimeIgnition-regimeStep*r,
		gate.regimeIgnitionFloor, gate.regimeIgnitionCeil,
	)
	gate.regimeFriction = clamp(
		gate.regimeFriction+regimeStep*r,
		gate.regimeFrictionFloor, gate.regimeFrictionCeil,
	)
	gate.maturityMin = clamp(
		gate.maturityMin-regimeStep*r*0.25,
		gate.maturityFloor, gate.maturityCeil,
	)

	if result.EnterR02 > 0 {
		weakHold := result.EnterNetHold < gate.ignition
		oppositionStep := regimeStep
		if weakHold {
			oppositionStep *= 1.5
		}
		gate.opposition = clamp(
			gate.opposition+oppositionStep*r,
			gate.oppositionFloor, gate.oppositionCeil,
		)
	}

	gate.fuzzyMatch = clamp(gate.fuzzyMatch+regimeStep*r*0.25, gate.fuzzyFloor, gate.fuzzyCeil)
	gate.minTokenBright = clamp(gate.minTokenBright-regimeStep*r*0.25, gate.minTokenFloor, gate.minTokenCeil)
	gate.salienceRatio = clamp(gate.salienceRatio-regimeStep*r*0.25, gate.salienceFloor, gate.salienceCeil)
	gate.emergencyExitBright = clamp(
		gate.emergencyExitBright-regimeStep*r*0.25,
		gate.emergencyFloor, gate.emergencyCeil,
	)
	gate.timeoutLossPenalty = clamp(
		gate.timeoutLossPenalty-regimeStep*math.Abs(r)*0.1,
		gate.timeoutPenaltyFloor, gate.timeoutPenaltyCeil,
	)
	gate.pathPruneBook = clamp(gate.pathPruneBook+regimeStep*r, gate.pathPruneFloor, gate.pathPruneCeil)
	gate.stopDamageRatio = clamp(
		gate.stopDamageRatio+regimeStep*r*0.25,
		gate.stopDamageFloor, gate.stopDamageCeil,
	)
	gate.evaporationMult = clamp(
		gate.evaporationMult+regimeStep*r*0.25,
		gate.evapMultFloor, gate.evapMultCeil,
	)
	gate.coderExitScale = clamp(
		gate.coderExitScale+regimeStep*r*0.25,
		gate.coderExitFloor, gate.coderExitCeil,
	)

	gate.mu.Unlock()
	gate.persist()
	gate.publishStops()
}

// regionBrightness returns the per-region brightness vector (15 entries) from
// the observed regions, indexed by region key.
func regionBrightness(observed []*nmstore.Region) [15]float64 {
	var out [15]float64
	for _, r := range observed {
		if r == nil || int(r.Key) < 0 || int(r.Key) >= 15 {
			continue
		}
		out[r.Key] = r.Brightness
	}
	return out
}
