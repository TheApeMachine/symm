package pumpdump

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

var outputKeys = []string{
	"trade_price",
	"trade_quantity",
	"trade_notional",
	"trade_interval_seconds",
	"volume_bar_target_quantity",
	"volume_bar_quantity",
	"volume_bar_notional",
	"volume_bar_trade_count",
	"volume_bar_duration",
	"volume_rate",
	"notional_rate",
	"trade_rate",
	"completed_bars",
	"notional_rate_baseline",
	"notional_rate_ratio",
	"notional_rate_divergence",
	"notional_rate_zscore",
	"notional_rate_velocity",
	"best_bid",
	"best_ask",
	"midpoint",
	"spread",
	"relative_spread",
	"relative_spread_baseline",
	"spread_ratio",
	"spread_divergence",
	"spread_zscore",
	"spread_divergence_velocity",
	"midpoint:from",
	"midpoint:at",
	"midpoint_log_return",
	"midpoint_return_rate",
	"positive_midpoint_return",
	"negative_midpoint_return",
	"midpoint_return_baseline",
	"midpoint_return_divergence",
	"midpoint_return_zscore",
	"midpoint_return_velocity",
	"historical_path_distance",
	"historical_path_percentile",
}

type causalEstimator struct {
	count float64
	mean  float64
	m2    float64
}

func (ce *causalEstimator) Step(value float64) (bool, float64, float64, float64, float64) {
	priorCount := ce.count
	priorMean := ce.mean
	priorM2 := ce.m2

	ce.count++
	delta := value - ce.mean
	ce.mean += delta / ce.count
	ce.m2 += delta * (value - ce.mean)

	baseline := value
	hasPrior := false
	if priorCount > 0 {
		hasPrior = true
		baseline = priorMean
	}

	var priorVar float64
	if priorCount > 1 {
		priorVar = priorM2 / (priorCount - 1)
	}

	residual := value - baseline
	scale := math.Abs(residual)
	var dispersion float64

	if priorVar > 0 {
		d := math.Sqrt(priorVar)
		if d > 2.220446049250313e-16 {
			scale = d
			dispersion = d
		}
	}

	var zScore float64
	if scale > 0 {
		zScore = residual / scale
	}

	return hasPrior, baseline, dispersion, residual, zScore
}

type volumeBarState struct {
	hasPrev       bool
	prevAt        float64
	hasBarStart   bool
	barStart      float64
	barQuantity   float64
	barNotional   float64
	barTradeCount float64
	barTarget     float64
	barFromMid    float64
	completedBars float64
	hasSeenAt     bool
	lastSeenAt    float64
	cached        [18]float64
}

func (vb *volumeBarState) Step(price, qty, atNanos, midpoint float64) [18]float64 {
	if vb.barTarget <= 0 {
		vb.barTarget = 1.0
	}

	if vb.hasSeenAt && atNanos == vb.lastSeenAt {
		return vb.cached
	}

	var timeDelta float64
	if vb.hasPrev {
		timeDelta = (atNanos - vb.prevAt) / 1e9
	}
	vb.hasPrev = true
	vb.prevAt = atNanos

	if !vb.hasBarStart {
		vb.hasBarStart = true
		vb.barStart = atNanos
		vb.barFromMid = midpoint
	}

	tradeNotional := price * qty
	vb.barQuantity += qty
	vb.barNotional += tradeNotional
	vb.barTradeCount += 1

	barDuration := (atNanos - vb.barStart) / 1e9
	barClosed := vb.barQuantity >= vb.barTarget && barDuration > 0

	outBarStart := atNanos
	var volumeBarDuration, fromMid, atMid float64
	var volumeRate, notionalRate, tradeRate float64
	var midpointLogReturn, midpointReturnRate, positiveReturn, negativeReturn float64

	if barClosed {
		vb.completedBars += 1
		outBarStart = vb.barStart
		volumeBarDuration = barDuration
		fromMid = vb.barFromMid
		atMid = midpoint

		if volumeBarDuration > 0 {
			volumeRate = vb.barQuantity / volumeBarDuration
			notionalRate = vb.barNotional / volumeBarDuration
			tradeRate = vb.barTradeCount / volumeBarDuration

			if fromMid > 0 && atMid > 0 {
				midpointLogReturn = math.Log(atMid / fromMid)
				midpointReturnRate = midpointLogReturn / volumeBarDuration

				if midpointLogReturn > 0 {
					positiveReturn = midpointLogReturn
				}
				if midpointLogReturn <= 0 {
					negativeReturn = -midpointLogReturn
				}
			}
		}
	}

	curBarTarget := vb.barTarget
	curBarQty := vb.barQuantity
	curBarNotional := vb.barNotional
	curBarTradeCount := vb.barTradeCount
	completedBars := vb.completedBars

	if barClosed {
		vb.barStart = atNanos
		vb.barQuantity = 0
		vb.barNotional = 0
		vb.barTradeCount = 0
		vb.barFromMid = midpoint
	}

	vb.hasSeenAt = true
	vb.lastSeenAt = atNanos
	vb.cached = [18]float64{
		tradeNotional,
		timeDelta,
		curBarTarget,
		curBarQty,
		curBarNotional,
		curBarTradeCount,
		volumeBarDuration,
		volumeRate,
		notionalRate,
		tradeRate,
		completedBars,
		fromMid,
		atMid,
		midpointLogReturn,
		midpointReturnRate,
		positiveReturn,
		negativeReturn,
		outBarStart,
	}

	return vb.cached
}

type symbolState struct {
	vb                 volumeBarState
	notionalEstimator  causalEstimator
	spreadEstimator    causalEstimator
	midReturnEstimator causalEstimator
	hasPrevNotional    bool
	prevNotionalRate   float64
	hasPrevSpreadDiv   bool
	prevSpreadDiv      float64
	hasPrevMidReturn   bool
	prevMidReturn      float64
	historyPoints      [][2]float64
	historyDistances   []float64
}

type Signal struct {
	*runtime.System
	books  broker.BookSource
	mu     sync.Mutex
	states map[string]*symbolState
}

func NewSignal(ctx context.Context, books broker.BookSource) *Signal {
	signal := &Signal{
		books:  books,
		states: make(map[string]*symbolState),
	}

	signal.System = runtime.NewSystem(ctx, "pumpdump", signal)

	if err := errnie.Require(map[string]any{"books": books}); err != nil {
		signal.Error(errnie.Err(errnie.Internal, "[pumpdump] book manager is required", err))
	}

	return signal
}

func (signal *Signal) Step(prior *data.Measurement) *data.Measurement {
	if signal.Status() != runtime.READY || prior == nil || prior.Label == "" {
		return nil
	}

	if signal.books == nil {
		signal.Error(errnie.Err(errnie.Internal, "[pumpdump] book manager is required", nil))
		return nil
	}

	priceEntry := data.Pull(prior.Read("price"))
	if priceEntry == nil || priceEntry.Metric == nil {
		signal.Error(errnie.Err(errnie.Validation, "[pumpdump] price is required", nil))
		return nil
	}
	price := priceEntry.Metric.Raw

	qtyEntry := data.Pull(prior.Read("qty"))
	if qtyEntry == nil || qtyEntry.Metric == nil || qtyEntry.Metric.Raw <= 0 {
		return nil
	}
	qty := qtyEntry.Metric.Raw

	var bid, ask float64
	var hasBook bool

	signal.books.Book(prior.Label, func(book *spotbook.Book) {
		if book == nil {
			return
		}

		b := book.BestBid()
		a := book.BestAsk()

		if b == nil || a == nil || b.Price == nil || a.Price == nil ||
			b.Quantity == nil || a.Quantity == nil {
			return
		}

		bid = kraken.Float64(b.Price)
		ask = kraken.Float64(a.Price)
		hasBook = true
	})

	if hasBook {
		touch := map[string]float64{
			"bid": bid,
			"ask": ask,
		}

		for _, value := range touch {
			if !broker.ValidTouchValue(value) {
				signal.Error(broker.InvalidTouch("pumpdump", prior.Label, touch))
				return nil
			}
		}

		if ask <= bid {
			errnie.Warn(fmt.Sprintf("[pumpdump] dropping frame for %s with crossed book: bid=%v ask=%v", prior.Label, bid, ask))
			return nil
		}
	}

	atNanos := float64(prior.At.UnixNano())
	hasTouch := bid > 0 && ask > 0
	midpoint := 0.0
	spread := 0.0
	relSpread := 0.0

	if hasTouch {
		midpoint = (bid + ask) * 0.5
		spread = ask - bid
		if midpoint > 0 {
			relSpread = spread / midpoint
		}
	}

	vbMidpoint := midpoint
	if !hasTouch {
		vbMidpoint = 0.0
	}

	signal.mu.Lock()
	state, found := signal.states[prior.Label]
	if !found {
		state = &symbolState{}
		signal.states[prior.Label] = state
	}

	vb := state.vb.Step(price, qty, atNanos, vbMidpoint)

	tradeNotional := vb[0]
	tradeInterval := vb[1]
	barTarget := vb[2]
	barQty := vb[3]
	barNotional := vb[4]
	barTradeCount := vb[5]
	barDuration := vb[6]
	volumeRate := vb[7]
	notionalRate := vb[8]
	tradeRate := vb[9]
	completedBars := vb[10]
	fromMid := vb[11]
	atMid := vb[12]
	midReturn := vb[13]
	midReturnRate := vb[14]
	posReturn := vb[15]
	negReturn := vb[16]
	outBarStart := vb[17]

	notionalBaseline := 0.0
	notionalRatio := 1.0
	notionalDivergence := 0.0
	notionalZScore := 0.0

	hasNotionalPrior, nBase, _, _, nZ := state.notionalEstimator.Step(notionalRate)
	if hasNotionalPrior {
		notionalBaseline = nBase
		if notionalBaseline > 0 && notionalRate > 0 {
			notionalRatio = notionalRate / notionalBaseline
			notionalDivergence = math.Log(notionalRatio)
		}
		notionalZScore = nZ
	}

	notionalVelocity := 0.0
	if state.hasPrevNotional {
		notionalVelocity = notionalRate - state.prevNotionalRate
	}
	state.prevNotionalRate = notionalRate
	state.hasPrevNotional = true

	spreadBaseline := 0.0
	spreadRatio := 0.0
	spreadDivergence := 0.0
	spreadZScore := 0.0
	relSpreadBaseline := 0.0

	if hasTouch && spread > 0 {
		spreadRatio = 1.0
		hasSpreadPrior, sBase, sDisp, _, _ := state.spreadEstimator.Step(spread)
		if hasSpreadPrior {
			spreadBaseline = sBase
			if spreadBaseline > 0 {
				spreadRatio = spread / spreadBaseline
				spreadDivergence = math.Log(spreadRatio)
			}
			if sDisp > 0 {
				spreadZScore = (spread - spreadBaseline) / sDisp
			}
			if sDisp <= 0 {
				spreadZScore = spreadDivergence
			}
			if midpoint > 0 {
				relSpreadBaseline = spreadBaseline / midpoint
			}
		}
	}

	spreadDivVelocity := 0.0
	if state.hasPrevSpreadDiv {
		spreadDivVelocity = spreadDivergence - state.prevSpreadDiv
	}
	state.prevSpreadDiv = spreadDivergence
	state.hasPrevSpreadDiv = true

	midReturnBaseline := 0.0
	midReturnDivergence := 0.0
	midReturnZScore := 0.0

	if hasTouch {
		hasMidPrior, mBase, _, mRes, mZ := state.midReturnEstimator.Step(midReturn)
		if hasMidPrior {
			midReturnBaseline = mBase
			midReturnDivergence = mRes
			midReturnZScore = mZ
		}
	}

	midReturnVelocity := 0.0
	if state.hasPrevMidReturn {
		midReturnVelocity = midReturn - state.prevMidReturn
	}
	state.prevMidReturn = midReturn
	state.hasPrevMidReturn = true

	target := [2]float64{spreadZScore, notionalZScore}
	histDist := 0.0
	histPerc := 0.0

	if len(state.historyPoints) > 0 {
		diffX := target[0] - state.historyPoints[0][0]
		diffY := target[1] - state.historyPoints[0][1]
		minDist := math.Sqrt(diffX*diffX + diffY*diffY)

		for idx := 1; idx < len(state.historyPoints); idx++ {
			dx := target[0] - state.historyPoints[idx][0]
			dy := target[1] - state.historyPoints[idx][1]
			distance := math.Sqrt(dx*dx + dy*dy)
			if distance < minDist {
				minDist = distance
			}
		}

		if len(state.historyDistances) > 0 {
			belowCount := 0
			for _, pastDist := range state.historyDistances {
				if pastDist <= minDist {
					belowCount++
				}
			}
			histPerc = float64(belowCount) / float64(len(state.historyDistances))
		}

		histDist = minDist
		state.historyDistances = append(state.historyDistances, minDist)
		if len(state.historyDistances) > 256 {
			state.historyDistances = state.historyDistances[len(state.historyDistances)-256:]
		}
	}

	state.historyPoints = append(state.historyPoints, target)
	if len(state.historyPoints) > 256 {
		state.historyPoints = state.historyPoints[len(state.historyPoints)-256:]
	}
	signal.mu.Unlock()

	if outBarStart > 0 {
		prior.From = time.Unix(0, int64(outBarStart))
	}

	output := map[string]float64{
		"trade_price":                price,
		"trade_quantity":             qty,
		"trade_notional":             tradeNotional,
		"trade_interval_seconds":     tradeInterval,
		"volume_bar_target_quantity": barTarget,
		"volume_bar_quantity":        barQty,
		"volume_bar_notional":        barNotional,
		"volume_bar_trade_count":     barTradeCount,
		"volume_bar_duration":        barDuration,
		"volume_rate":                volumeRate,
		"notional_rate":              notionalRate,
		"trade_rate":                 tradeRate,
		"completed_bars":             completedBars,
		"notional_rate_baseline":     notionalBaseline,
		"notional_rate_ratio":        notionalRatio,
		"notional_rate_divergence":   notionalDivergence,
		"notional_rate_zscore":       notionalZScore,
		"notional_rate_velocity":     notionalVelocity,
		"best_bid":                   bid,
		"best_ask":                   ask,
		"midpoint":                   midpoint,
		"spread":                     spread,
		"relative_spread":            relSpread,
		"relative_spread_baseline":   relSpreadBaseline,
		"spread_ratio":               spreadRatio,
		"spread_divergence":          spreadDivergence,
		"spread_zscore":              spreadZScore,
		"spread_divergence_velocity": spreadDivVelocity,
		"midpoint:from":              fromMid,
		"midpoint:at":                atMid,
		"midpoint_log_return":        midReturn,
		"midpoint_return_rate":       midReturnRate,
		"positive_midpoint_return":   posReturn,
		"negative_midpoint_return":   negReturn,
		"midpoint_return_baseline":   midReturnBaseline,
		"midpoint_return_divergence": midReturnDivergence,
		"midpoint_return_zscore":     midReturnZScore,
		"midpoint_return_velocity":   midReturnVelocity,
		"historical_path_distance":   histDist,
		"historical_path_percentile": histPerc,
	}

	return prior.Next(signal.Name(), output)
}
