package deskgate

import (
	"context"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/system"
)

const (
	stateKey         = "model/parameters/deskgate.bin"
	warmupMinSamples = int64(48)
	warmupCutCap     = 0.35
)

/*
Axis is one desk oracle dimension (volume, day, …) with a learned cutoff on a
unit-normalized feature. Higher means more approve-like unless Higher is false.
*/
type Axis struct {
	Phase  string  `json:"phase"`
	Higher bool    `json:"higher"`
	Cut    float64 `json:"cut"`
	Sample int64   `json:"sample"`
}

/*
Gate learns enter oracle cuts from develop vs stall outcomes — same resistance
geometry as path leaves (threshold step) without YAML knob churn.
*/
type gateState struct {
	Version int              `json:"version"`
	Axes    map[string]*Axis `json:"axes"`
}

type Gate struct {
	ctx    context.Context
	mu     sync.Mutex
	Axes   map[string]*Axis
	loaded bool
	store  *store.Client
}

/*
NewGate seeds cuts at 0.5 and loads persisted state when a store client exists.
*/
func NewGate(ctx context.Context, client *store.Client) *Gate {
	gate := &Gate{
		ctx:   ctx,
		Axes:  defaultAxes(),
		store: client,
	}

	if client != nil {
		gate.restore()
	}

	return gate
}

func defaultAxes() map[string]*Axis {
	return map[string]*Axis{
		"volume": {Phase: "volume", Higher: true, Cut: warmupCutCap},
		"day":    {Phase: "day", Higher: true, Cut: warmupCutCap},
		"spread": {Phase: "spread", Higher: true, Cut: warmupCutCap},
		"book":   {Phase: "book", Higher: true, Cut: warmupCutCap},
		"peak":   {Phase: "peak", Higher: true, Cut: warmupCutCap},
	}
}

/*
Features maps desk source labels the grid SNM layer can fan into oracle regions.
*/
func (gate *Gate) Features(
	price *broker.Price,
	symbol string,
	pump *equation.IgnitionOutput,
) map[string]float64 {
	if price == nil || symbol == "" {
		return nil
	}

	out := map[string]float64{
		"tape_accel": scoreTapeAccel(price, symbol),
		"tape_recent": scoreTapeRecent(price, symbol),
		"session_move": scoreSessionMove(price, symbol),
		"impulse": scoreImpulse(pump),
		"spread_room": scoreSpreadRoom(price, symbol),
		"touch_backing": scoreTouchBacking(price, symbol),
		"peak_room": scorePeakRoom(price, symbol),
	}

	gate.mu.Lock()
	for name, axis := range gate.Axes {
		if axis == nil {
			continue
		}
		var v float64
		switch name {
		case "volume":
			v = 0.6*out["tape_accel"] + 0.4*out["tape_recent"]
		case "day":
			if out["impulse"] > 0.05 {
				v = 0.5*out["session_move"] + 0.5*out["impulse"]
			} else {
				v = out["session_move"]
			}
		case "spread":
			v = out["spread_room"]
		case "book":
			v = out["touch_backing"]
		case "peak":
			v = out["peak_room"]
		}
		out["axis_"+name] = v
	}
	gate.mu.Unlock()

	return out
}

/*
Oracle passes each desk lamp using learned cuts on the composite axis_* score.
*/
func (gate *Gate) Oracle(
	price *broker.Price,
	symbol string,
	pump *equation.IgnitionOutput,
) (volume, day, spread, book, peak bool) {
	feat := gate.Features(price, symbol, pump)
	if feat == nil {
		return false, false, false, false, false
	}

	gate.mu.Lock()
	defer gate.mu.Unlock()

	warm := gate.warmup()

	pass := func(name string) bool {
		axis := gate.Axes[name]
		if axis == nil {
			return false
		}
		score := feat["axis_"+name]
		cut := axis.Cut
		if warm {
			cut = math.Min(cut, warmupCutCap)
		}

		learned := score >= cut
		if !axis.Higher {
			learned = score <= cut
		}

		if learned {
			return true
		}

		return warm && mechanicalOK(price, symbol, name)
	}

	return pass("volume"), pass("day"), pass("spread"), pass("book"), pass("peak")
}

/*
Learn nudges cuts from settle outcomes. developed means the trade proved the
enter reading (armed stop or fee-net win) — tighten toward those features;
otherwise push the cut away from borderline passes.
*/
func (gate *Gate) Learn(features map[string]float64, developed bool) {
	if len(features) == 0 {
		return
	}

	step := system.Cfg.Learning.ThresholdStep
	if step <= 0 {
		step = 0.05
	}

	gate.mu.Lock()
	for name, axis := range gate.Axes {
		if axis == nil {
			continue
		}
		score := features["axis_"+name]
		if score == 0 && features["tape_accel"] != 0 {
			continue
		}

		axis.Sample++

		if developed {
			axis.Cut = axis.Cut + step*(score-axis.Cut)
		} else if score >= axis.Cut && axis.Higher {
			axis.Cut = math.Min(0.95, axis.Cut+step)
		} else if score <= axis.Cut && !axis.Higher {
			axis.Cut = math.Max(0.05, axis.Cut-step)
		}
		axis.Cut = clampCut(axis.Cut)
	}
	gate.mu.Unlock()

	gate.persist()
}

/*
Developed is true when the position showed the path deserved real attention.
*/
func Developed(position *broker.Position) bool {
	if position == nil {
		return false
	}

	arm := system.Cfg.Risk.StoplossArmPnLPct
	if position.Stoploss != nil && position.Stoploss.PeakPnLPct >= arm {
		return true
	}

	if position.PnLPct != nil {
		return position.PnLPct.InexactFloat64() >= system.Cfg.Learning.PolicyExitMinProfitPct
	}

	return false
}

func (gate *Gate) persist() {
	if gate.store == nil {
		return
	}

	gate.mu.Lock()
	raw, err := sonic.Marshal(gateState{Version: 1, Axes: gate.Axes})
	gate.mu.Unlock()

	if err != nil {
		errnie.Error(errnie.Err(errnie.IO, "[deskgate] marshal", err))
		return
	}

	if _, err := gate.store.WriteSync(stateKey, raw); err != nil {
		errnie.Error(err)
	}
}

func (gate *Gate) restore() {
	for page := range gate.store.List("model/parameters/") {
		for _, object := range page.Contents {
			if object.Key == nil || !strings.HasSuffix(*object.Key, "deskgate.bin") {
				continue
			}
			raw, err := gate.store.Download(gate.ctx, *object.Key)
			if err != nil || len(raw) == 0 {
				continue
			}
			var loaded gateState
			if err := sonic.Unmarshal(raw, &loaded); err != nil {
				errnie.Error(errnie.Err(errnie.IO, "[deskgate] unmarshal", err))
				return
			}
			if loaded.Version < 1 {
				return
			}

			gate.mu.Lock()
			for name, axis := range loaded.Axes {
				if axis == nil {
					continue
				}
				gate.Axes[name] = axis
			}
			gate.loaded = true
			gate.mu.Unlock()
			return
		}
	}
}

func (gate *Gate) warmup() bool {
	gate.mu.Lock()
	defer gate.mu.Unlock()

	var total int64
	for _, axis := range gate.Axes {
		if axis != nil {
			total += axis.Sample
		}
	}

	return total < warmupMinSamples
}

/*
mechanicalOK is the book feasibility path used only until desk axes have enough
settle samples — not a second YAML gate, just liquidity/microstructure survival.
*/
func mechanicalOK(price *broker.Price, symbol, name string) bool {
	if price == nil {
		return false
	}

	switch name {
	case "volume":
		return price.TapeAccelOK(symbol)
	case "day":
		risk := system.Cfg.Risk
		if price.ChangePct(symbol) < risk.MinMoverChangePct {
			return false
		}
		recent, _ := price.TapeUptake(symbol, time.Now())
		return recent >= risk.MinTapeNotionalUSD
	case "spread":
		max := system.Cfg.Risk.MaxEntrySpreadPct
		if max <= 0 {
			return true
		}
		spread, ok := price.SpreadPct(symbol)
		return ok && spread != nil && spread.InexactFloat64() <= max
	case "book":
		risk := system.Cfg.Risk
		cash, ok := price.TouchBidCash(symbol)
		if ok && cash != nil && cash.InexactFloat64() >= risk.MinTouchBidCash {
			ratio, ok := price.BackingRatio(symbol)
			if ok && ratio >= risk.MinBackingRatio {
				return true
			}
		}
		return price.TapeAccelOK(symbol)
	case "peak":
		return price.EntryPeakOK(symbol, false)
	default:
		return false
	}
}

func clampCut(v float64) float64 {
	floor := system.Cfg.Learning.ThresholdFloor
	ceil := system.Cfg.Learning.ThresholdCeil
	if v < floor {
		return floor
	}
	if v > ceil {
		return ceil
	}
	return v
}

func scoreTapeAccel(price *broker.Price, symbol string) float64 {
	recent, prior := price.TapeUptake(symbol, time.Now())
	if recent <= 0 {
		return 0
	}
	if prior <= 0 {
		return math.Min(recent/(system.Cfg.Risk.MinTapeNotionalUSD*2), 1)
	}
	return math.Min(recent/prior/2, 1)
}

func scoreTapeRecent(price *broker.Price, symbol string) float64 {
	recent, _ := price.TapeUptake(symbol, time.Now())
	floor := system.Cfg.Risk.MinTapeNotionalUSD
	if floor <= 0 {
		floor = 1
	}
	return math.Min(recent/floor, 1)
}

func scoreSessionMove(price *broker.Price, symbol string) float64 {
	floor := system.Cfg.Risk.MinMoverChangePct
	if floor <= 0 {
		floor = 0.5
	}
	return math.Min(math.Max(price.ChangePct(symbol), 0)/floor/2, 1)
}

func scoreImpulse(pump *equation.IgnitionOutput) float64 {
	if pump == nil {
		return 0
	}

	sel := system.Cfg.Selection

	magnitude := 0.0
	if sel.IgnitionPrecursorMin > 0 {
		magnitude = math.Min(pump.Precursor/sel.IgnitionPrecursorMin, 1)
	}

	velocity := math.Min(pump.RVOL, 1)

	accelScore := 0.5
	if sel.IgnitionIgnitionMin > 0 {
		pace := math.Max(pump.Ignition, pump.Trend)

		if pace > 0 {
			accelScore = math.Min(pace/sel.IgnitionIgnitionMin, 1)
		}
	}

	return 0.5*magnitude + 0.25*velocity + 0.25*accelScore
}

func scoreSpreadRoom(price *broker.Price, symbol string) float64 {
	max := system.Cfg.Risk.MaxEntrySpreadPct
	if max <= 0 {
		return 1
	}
	spread, ok := price.SpreadPct(symbol)
	if !ok || spread == nil {
		return 0
	}
	return 1 - math.Min(spread.InexactFloat64()/max, 1)
}

func scoreTouchBacking(price *broker.Price, symbol string) float64 {
	risk := system.Cfg.Risk
	cash, ok := price.TouchBidCash(symbol)
	touch := 0.0
	if ok && cash != nil {
		touch = math.Min(cash.InexactFloat64()/risk.MinTouchBidCash, 1)
	}
	ratio, ok := price.BackingRatio(symbol)
	back := 0.0
	if ok {
		back = math.Min(ratio/risk.MinBackingRatio, 1)
	}
	tape := scoreTapeAccel(price, symbol)
	return (touch + back + tape) / 3
}

func scorePeakRoom(price *broker.Price, symbol string) float64 {
	frac, ok := price.PeakFrac(symbol)
	if !ok {
		return 0
	}
	cap := system.Cfg.Risk.MaxEntryPeakFrac
	if cap <= 0 || cap >= 1 {
		cap = 0.85
	}
	if frac >= cap {
		return 0
	}
	return 1 - frac/cap
}