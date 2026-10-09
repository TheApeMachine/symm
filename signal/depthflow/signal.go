package depthflow

import (
	"context"
	"fmt"
	"math"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

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

type symbolState struct {
	prevBids            map[float64]float64
	prevAsks            map[float64]float64
	prevTotal           float64
	prevAtNano          float64
	hasPrev             bool
	hasPrevBookImb      bool
	prevBookImb         float64
	hasPrevGap          bool
	prevGap             float64
	bookImbEstimator    causalEstimator
	gapEstimator        causalEstimator
	turnoverEstimator   causalEstimator
	netChangeEstimator  causalEstimator
	signedFlowEstimator causalEstimator
	historyPoints       [][2]float64
	historyDistances    []float64
}

type Signal struct {
	*runtime.System
	books  broker.BookSource
	states map[string]*symbolState
}

func NewSignal(ctx context.Context, books broker.BookSource) *Signal {
	signal := &Signal{
		books:  books,
		states: make(map[string]*symbolState),
	}

	signal.System = runtime.NewSystem(ctx, "depthflow", signal)

	if err := errnie.Require(map[string]any{"books": books}); err != nil {
		signal.Error(errnie.Err(errnie.Internal, "[depthflow] book manager is required", err))
	}

	return signal
}

func (signal *Signal) Step(prior *data.Measurement) *data.Measurement {
	if signal.Status() != runtime.READY || prior == nil || prior.Label == "" {
		return nil
	}

	if signal.books == nil {
		signal.Error(errnie.Err(errnie.Internal, "[depthflow] book manager is required", nil))
		return nil
	}

	var bids, asks [][2]float64
	var crossedBid, crossedAsk float64
	ok := false
	crossed := false

	signal.books.Book(prior.Label, func(book *spotbook.Book) {
		if book == nil {
			return
		}

		bid := book.BestBid()
		ask := book.BestAsk()

		if bid == nil || ask == nil || bid.Price == nil || ask.Price == nil {
			return
		}

		bidPrice := kraken.Float64(bid.Price)
		askPrice := kraken.Float64(ask.Price)

		if askPrice <= bidPrice {
			crossed = true
			crossedBid = bidPrice
			crossedAsk = askPrice
			return
		}

		for cursor, count := bid, 0; cursor != nil && count < 100; cursor, count = cursor.Lower, count+1 {
			if cursor.Price == nil || cursor.Quantity == nil {
				continue
			}

			price := kraken.Float64(cursor.Price)
			qty := kraken.Float64(cursor.Quantity)

			if price <= 0 || qty <= 0 {
				continue
			}

			bids = append(bids, [2]float64{price, qty})
		}

		for cursor, count := ask, 0; cursor != nil && count < 100; cursor, count = cursor.Higher, count+1 {
			if cursor.Price == nil || cursor.Quantity == nil {
				continue
			}

			price := kraken.Float64(cursor.Price)
			qty := kraken.Float64(cursor.Quantity)

			if price <= 0 || qty <= 0 {
				continue
			}

			asks = append(asks, [2]float64{price, qty})
		}

		ok = len(bids) > 0 && len(asks) > 0
	})

	if crossed {
		errnie.Warn(fmt.Sprintf("[depthflow] dropping frame for %s with crossed book: bid=%v ask=%v", prior.Label, crossedBid, crossedAsk))
		return nil
	}

	if !ok {
		return nil
	}

	touchBid := bids[0][0] * bids[0][1]
	touchAsk := asks[0][0] * asks[0][1]

	obsBid := 0.0
	currentBids := make(map[float64]float64, len(bids))

	for _, b := range bids {
		notional := b[0] * b[1]
		obsBid += notional
		currentBids[b[0]] = notional
	}

	obsAsk := 0.0
	currentAsks := make(map[float64]float64, len(asks))

	for _, a := range asks {
		notional := a[0] * a[1]
		obsAsk += notional
		currentAsks[a[0]] = notional
	}

	total := obsBid + obsAsk
	bookImb := (obsBid - obsAsk) / total
	touchImb := (touchBid - touchAsk) / (touchBid + touchAsk)
	gap := touchImb - bookImb
	gapDist := math.Abs(gap)

	atNano := float64(prior.At.UnixNano())
	state, found := signal.states[prior.Label]

	if !found {
		state = &symbolState{
			prevBids: make(map[float64]float64),
			prevAsks: make(map[float64]float64),
		}
		signal.states[prior.Label] = state
	}

	addedBid, removedBid := 0.0, 0.0
	addedAsk, removedAsk := 0.0, 0.0
	timeDelta := 0.1
	bookTurnoverRate := 0.0
	netBookChangeRate := 0.0
	signedNetFlowRate := 0.0

	addedRateBid := 0.0
	addedRateAsk := 0.0
	removedRateBid := 0.0
	removedRateAsk := 0.0
	netFlowRateBid := 0.0
	netFlowRateAsk := 0.0

	netBid := 0.0
	netAsk := 0.0
	flowActivityImbalance := 0.0

	if state.hasPrev {
		if atNano > state.prevAtNano {
			timeDelta = (atNano - state.prevAtNano) * 1e-9
		}

		for price, notional := range currentBids {
			prev := state.prevBids[price]
			if notional > prev {
				addedBid += notional - prev
				continue
			}

			if notional < prev {
				removedBid += prev - notional
			}
		}

		for price, prev := range state.prevBids {
			if _, exists := currentBids[price]; !exists {
				removedBid += prev
			}
		}

		for price, notional := range currentAsks {
			prev := state.prevAsks[price]
			if notional > prev {
				addedAsk += notional - prev
				continue
			}

			if notional < prev {
				removedAsk += prev - notional
			}
		}

		for price, prev := range state.prevAsks {
			if _, exists := currentAsks[price]; !exists {
				removedAsk += prev
			}
		}

		netBid = addedBid - removedBid
		netAsk = addedAsk - removedAsk
		grossBid := addedBid + removedBid
		grossAsk := addedAsk + removedAsk
		bookActivity := grossBid + grossAsk
		netBookChange := total - state.prevTotal
		signedNetFlow := netBid - netAsk

		reference := (state.prevTotal + total) / 2.0
		if reference > 0 && timeDelta > 0 {
			bookTurnoverRate = bookActivity / (reference * timeDelta)
			netBookChangeRate = netBookChange / (reference * timeDelta)
			signedNetFlowRate = signedNetFlow / (reference * timeDelta)
		}

		if timeDelta > 0 {
			addedRateBid = addedBid / timeDelta
			addedRateAsk = addedAsk / timeDelta
			removedRateBid = removedBid / timeDelta
			removedRateAsk = removedAsk / timeDelta
			netFlowRateBid = netBid / timeDelta
			netFlowRateAsk = netAsk / timeDelta
		}

		denom := math.Abs(netBid) + math.Abs(netAsk)
		if denom > 0 {
			flowActivityImbalance = signedNetFlow / denom
		}
	}

	bookImbBaseline := 0.0
	bookImbDivergence := 0.0
	bookImbZScore := 0.0
	hasBImpPrior, bBase, _, bRes, bZ := state.bookImbEstimator.Step(bookImb)
	if hasBImpPrior {
		bookImbBaseline = bBase
		bookImbDivergence = bRes
		bookImbZScore = bZ
	}

	bookImbVelocity := 0.0
	if state.hasPrevBookImb {
		bookImbVelocity = bookImb - state.prevBookImb
	}
	state.prevBookImb = bookImb
	state.hasPrevBookImb = true

	gapBaseline := 0.0
	gapDivergence := 0.0
	gapZScore := 0.0
	hasGapPrior, gBase, _, gRes, gZ := state.gapEstimator.Step(gap)
	if hasGapPrior {
		gapBaseline = gBase
		gapDivergence = gRes
		gapZScore = gZ
	}

	gapVelocity := 0.0
	if state.hasPrevGap {
		gapVelocity = gap - state.prevGap
	}
	state.prevGap = gap
	state.hasPrevGap = true

	turnoverBaseline := 0.0
	turnoverDivergence := 0.0
	turnoverZScore := 0.0
	turnoverRatio := 1.0

	if state.hasPrev {
		hasTPrior, tBase, _, tRes, tZ := state.turnoverEstimator.Step(bookTurnoverRate)
		if hasTPrior {
			turnoverBaseline = tBase
			turnoverDivergence = tRes
			turnoverZScore = tZ
			if turnoverBaseline > 0 {
				turnoverRatio = bookTurnoverRate / turnoverBaseline
			}
		}
	}

	netChangeBaseline := 0.0
	netChangeDivergence := 0.0
	netChangeZScore := 0.0

	if state.hasPrev {
		hasNCPrior, ncBase, _, ncRes, ncZ := state.netChangeEstimator.Step(netBookChangeRate)
		if hasNCPrior {
			netChangeBaseline = ncBase
			netChangeDivergence = ncRes
			netChangeZScore = ncZ
		}
	}

	signedFlowBaseline := 0.0
	signedFlowDivergence := 0.0
	signedFlowZScore := 0.0

	if state.hasPrev {
		hasSFPrior, sfBase, _, sfRes, sfZ := state.signedFlowEstimator.Step(signedNetFlowRate)
		if hasSFPrior {
			signedFlowBaseline = sfBase
			signedFlowDivergence = sfRes
			signedFlowZScore = sfZ
		}
	}

	histDist := 0.0
	histPerc := 0.0

	if state.hasPrev {
		target := [2]float64{turnoverZScore, netChangeZScore}
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
	}

	state.prevBids = currentBids
	state.prevAsks = currentAsks
	state.prevTotal = total
	state.prevAtNano = atNano
	state.hasPrev = true

	return prior.Next(signal.Name(), map[string]float64{
		"book_notional:bid":                         obsBid,
		"book_notional:ask":                         obsAsk,
		"book_notional":                             total,
		"observed_notional:bid":                     obsBid,
		"observed_notional:ask":                     obsAsk,
		"observed_notional":                         total,
		"book_imbalance":                            bookImb,
		"observed_notional_imbalance":               bookImb,
		"touch_imbalance":                           touchImb,
		"imbalance_resolution_gap":                  gap,
		"imbalance_resolution_distance":             gapDist,
		"added_notional:bid":                        addedBid,
		"removed_notional:bid":                      removedBid,
		"net_displayed_flow:bid":                    netBid,
		"added_notional:ask":                        addedAsk,
		"removed_notional:ask":                      removedAsk,
		"net_displayed_flow:ask":                    netAsk,
		"flow_activity_imbalance":                   flowActivityImbalance,
		"book_imbalance_baseline":                   bookImbBaseline,
		"book_imbalance_divergence":                 bookImbDivergence,
		"book_imbalance_zscore":                     bookImbZScore,
		"resolution_gap_baseline":                   gapBaseline,
		"resolution_gap_divergence":                 gapDivergence,
		"resolution_gap_zscore":                     gapZScore,
		"book_imbalance_velocity":                   bookImbVelocity,
		"resolution_gap_velocity":                   gapVelocity,
		"added_notional_rate:bid":                   addedRateBid,
		"added_notional_rate:ask":                   addedRateAsk,
		"removed_notional_rate:bid":                 removedRateBid,
		"removed_notional_rate:ask":                 removedRateAsk,
		"net_displayed_flow_rate:bid":               netFlowRateBid,
		"net_displayed_flow_rate:ask":               netFlowRateAsk,
		"book_turnover_rate":                        bookTurnoverRate,
		"net_book_change_rate":                      netBookChangeRate,
		"signed_net_displayed_flow_rate":            signedNetFlowRate,
		"turnover_baseline":                         turnoverBaseline,
		"turnover_divergence":                       turnoverDivergence,
		"turnover_zscore":                           turnoverZScore,
		"turnover_ratio":                            turnoverRatio,
		"net_book_change_rate_baseline":             netChangeBaseline,
		"net_book_change_rate_divergence":           netChangeDivergence,
		"net_book_change_rate_zscore":               netChangeZScore,
		"signed_net_displayed_flow_rate_baseline":   signedFlowBaseline,
		"signed_net_displayed_flow_rate_divergence": signedFlowDivergence,
		"signed_net_displayed_flow_rate_zscore":     signedFlowZScore,
		"historical_path_distance":                  histDist,
		"historical_path_percentile":                histPerc,
	})
}
