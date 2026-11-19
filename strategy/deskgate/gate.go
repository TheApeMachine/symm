package deskgate

import (
	"context"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/adaptive"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/system"
)

const (
	stateKey          = "model/parameters/deskgate.bin"
	cutNeutral        = 0.5
	trackReadyAt      = 0.5
	brightnessScale   = 2.0
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
	track  *adaptive.Standardizer `json:"-"`
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
NewGate seeds cuts at neutral brightness and loads persisted state when a store exists.
*/
func NewGate(ctx context.Context, client *store.Client) *Gate {
	gate := &Gate{
		ctx:   ctx,
		store: client,
	}
	gate.Axes = gate.seedAxes()

	if client != nil {
		gate.restore()
	}

	return gate
}

func (gate *Gate) seedAxes() map[string]*Axis {
	return map[string]*Axis{
		"volume": gate.newAxis("volume", true),
		"day":    gate.newAxis("day", true),
		"spread": gate.newAxis("spread", true),
		"book":   gate.newAxis("book", true),
		"peak":   gate.newAxis("peak", true),
	}
}

func (gate *Gate) newAxis(phase string, higher bool) *Axis {
	return &Axis{
		Phase:  phase,
		Higher: higher,
		Cut:    cutNeutral,
		track:  adaptive.NewStandardizer(),
	}
}

func (gate *Gate) bindTracks() {
	for _, axis := range gate.Axes {
		if axis != nil && axis.track == nil {
			axis.track = adaptive.NewStandardizer()
		}
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

	now := time.Now()
	out := map[string]float64{
		"tape_recent":  gate.tapeRecent(price, symbol, now),
		"tape_ratio":   gate.tapeRatio(price, symbol, now),
		"session_move": math.Max(price.ChangePct(symbol), 0),
		"impulse":      gate.impulseLevel(pump),
		"spread_pct":   gate.spreadPct(price, symbol),
		"touch_cash":   gate.touchCash(price, symbol),
		"backing":      gate.backingRatio(price, symbol),
		"peak_frac":    gate.peakFrac(price, symbol),
		"volume_usd":   gate.volumeUSD(price, symbol),
		"open_drag":    gate.openDrag(price, symbol),
	}

	gate.mu.Lock()
	gate.bindTracks()
	for name, axis := range gate.Axes {
		if axis == nil {
			continue
		}
		raw := gate.rawAxis(name, out, pump)
		out["axis_"+name] = axis.score(raw)
		out["axis_"+name+"_raw"] = raw
	}
	gate.mu.Unlock()

	return out
}

func (gate *Gate) rawAxis(name string, out map[string]float64, pump *equation.IgnitionOutput) float64 {
	switch name {
	case "volume":
		recent := out["tape_recent"]
		ratio := out["tape_ratio"]
		if ratio > 0 {
			return recent + math.Log1p(ratio)
		}
		return recent
	case "day":
		move := out["session_move"]
		if move <= 0 {
			return out["impulse"]
		}
		if pump != nil && out["impulse"] > 0 {
			return move + out["impulse"]
		}
		return move
	case "spread":
		return -out["spread_pct"]
	case "book":
		drag := out["open_drag"]
		if drag < 0 {
			drag = 0
		}
		return out["touch_cash"] + out["backing"] + out["tape_ratio"] - drag
	case "peak":
		frac := out["peak_frac"]
		if frac < 0 {
			return 0
		}
		return 1 - frac
	default:
		return 0
	}
}

func (axis *Axis) score(raw float64) float64 {
	if axis == nil || axis.track == nil {
		return 0
	}

	out, err := axis.track.ZScore(raw)
	if err != nil || !out.Ready {
		return 0
	}

	z := out.Value
	if !axis.Higher {
		z = -z
	}

	return 0.5 + 0.5*math.Tanh(z/brightnessScale)
}

/*
Oracle passes each desk lamp using learned cuts on the composite axis_* score.
*/
func (gate *Gate) Oracle(
	price *broker.Price,
	symbol string,
	pump *equation.IgnitionOutput,
) (volume, day, spread, book, peak bool) {
	return gate.OracleFrom(gate.Features(price, symbol, pump))
}

/*
OracleFrom applies learned cuts to a desk feature map — Training computes Features
once per symbol and passes the same map here and to grid Observe.
*/
func (gate *Gate) OracleFrom(feat map[string]float64) (volume, day, spread, book, peak bool) {
	if feat == nil {
		return false, false, false, false, false
	}

	gate.mu.Lock()
	defer gate.mu.Unlock()

	return gate.oraclePass("volume", feat),
		gate.oraclePass("day", feat),
		gate.oraclePass("spread", feat),
		gate.oraclePass("book", feat),
		gate.oraclePass("peak", feat)
}

func (gate *Gate) oraclePass(name string, feat map[string]float64) bool {
	axis := gate.Axes[name]
	if axis == nil {
		return false
	}

	// Risk floors never drop out — a 3-sample Standardizer must not replace
	// MinMoverVolumeUSD / MaxEntrySpreadPct / … with cuts fit only on entered
	// trades. Cold track: static only. Warm track: static AND learned cut.
	if !gate.staticPass(name, feat) {
		return false
	}

	if !axis.ready() {
		return true
	}

	score := feat["axis_"+name]
	if score <= 0 {
		return false
	}

	if axis.Higher {
		return score >= axis.Cut
	}

	return score <= axis.Cut
}

/*
staticPass is the cold-oracle path: config risk floors, not learned cuts.
*/
func (gate *Gate) staticPass(name string, feat map[string]float64) bool {
	risk := system.Cfg.Risk

	switch name {
	case "volume":
		return feat["volume_usd"] >= risk.MinMoverVolumeUSD
	case "day":
		return feat["session_move"] >= risk.MinMoverChangePct
	case "spread":
		if risk.MaxEntrySpreadPct <= 0 {
			return true
		}

		spread := feat["spread_pct"]
		return spread > 0 && spread <= risk.MaxEntrySpreadPct
	case "book":
		// touch_cash in Features is Log1p(USD) for the learned axis.
		if feat["touch_cash"] < math.Log1p(risk.MinTouchBidCash) ||
			feat["backing"] < risk.MinBackingRatio {
			return false
		}
		if risk.MaxEntryOpenDragPct > 0 && feat["open_drag"] > risk.MaxEntryOpenDragPct {
			return false
		}
		return true
	case "peak":
		if risk.MaxEntryPeakFrac <= 0 || risk.MaxEntryPeakFrac >= 1 {
			return true
		}

		frac := feat["peak_frac"]
		if frac < 0 {
			return true
		}

		return frac < risk.MaxEntryPeakFrac
	default:
		return false
	}
}

func (axis *Axis) ready() bool {
	return axis != nil &&
		axis.track != nil &&
		axis.track.Precision() >= trackReadyAt
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

	learn := system.Cfg.Learning
	step := learn.ThresholdStep
	if step <= 0 {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"[deskgate] learning.threshold_step required",
			nil,
		))

		return
	}

	floor, ceil := learn.ThresholdFloor, learn.ThresholdCeil

	gate.mu.Lock()
	gate.bindTracks()
	for name, axis := range gate.Axes {
		if axis == nil {
			continue
		}
		rawKey := "axis_" + name + "_raw"
		raw := features[rawKey]
		if raw == 0 && features["tape_recent"] == 0 {
			continue
		}

		if raw != 0 {
			if _, err := axis.track.Measure(raw); err != nil {
				errnie.Error(err)
			}
		}

		score := features["axis_"+name]
		if score == 0 {
			score = axis.score(raw)
		}

		axis.Sample++

		if developed {
			axis.Cut = axis.Cut + step*(score-axis.Cut)
		} else if score >= axis.Cut && axis.Higher {
			axis.Cut = math.Min(ceil, axis.Cut+step)
		} else if score <= axis.Cut && !axis.Higher {
			axis.Cut = math.Max(floor, axis.Cut-step)
		}
		axis.Cut = math.Min(ceil, math.Max(floor, axis.Cut))
	}
	gate.mu.Unlock()

	gate.persist()
}

/*
Developed is true when the position showed the path deserved real attention.
*/
func (gate *Gate) Developed(position *broker.Position) bool {
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
			gate.bindTracks()
			gate.loaded = true
			gate.mu.Unlock()
			return
		}
	}
}

func (gate *Gate) tapeRecent(price *broker.Price, symbol string, at time.Time) float64 {
	recent, _ := price.TapeUptake(symbol, at)
	if recent <= 0 {
		return 0
	}
	return math.Log1p(recent)
}

func (gate *Gate) tapeRatio(price *broker.Price, symbol string, at time.Time) float64 {
	recent, prior := price.TapeUptake(symbol, at)
	if recent <= 0 || prior <= 0 {
		return 0
	}
	return recent / prior
}

func (gate *Gate) impulseLevel(pump *equation.IgnitionOutput) float64 {
	if pump == nil {
		return 0
	}
	return math.Log1p(pump.Precursor) +
		math.Log1p(pump.Ignition) +
		math.Log1p(pump.Compression) +
		math.Log1p(pump.RVOL)
}

func (gate *Gate) spreadPct(price *broker.Price, symbol string) float64 {
	spread, ok := price.SpreadPct(symbol)
	if !ok || spread == nil {
		return 0
	}
	return spread.InexactFloat64()
}

func (gate *Gate) touchCash(price *broker.Price, symbol string) float64 {
	cash, ok := price.TouchBidCash(symbol)
	if !ok || cash == nil || cash.Sign() <= 0 {
		return 0
	}
	return math.Log1p(cash.InexactFloat64())
}

func (gate *Gate) backingRatio(price *broker.Price, symbol string) float64 {
	ratio, ok := price.BackingRatio(symbol)
	if !ok {
		return 0
	}
	return ratio
}

func (gate *Gate) peakFrac(price *broker.Price, symbol string) float64 {
	frac, ok := price.PeakFrac(symbol)
	if !ok {
		return -1
	}
	return frac
}

func (gate *Gate) volumeUSD(price *broker.Price, symbol string) float64 {
	if price == nil {
		return 0
	}

	return price.VolumeUSD(symbol)
}

func (gate *Gate) openDrag(price *broker.Price, symbol string) float64 {
	drag, ok := price.OpenDragPct(symbol)
	if !ok {
		return -1
	}
	return drag
}