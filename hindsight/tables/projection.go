package tables

import (
	"context"
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/apache/iceberg-go"
	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
)

var (
	symbolInternMu sync.RWMutex
	symbolIntern   = make(map[string]string)
)

func internSymbol(symbol string) string {
	symbolInternMu.RLock()
	interned, ok := symbolIntern[symbol]
	symbolInternMu.RUnlock()

	if ok {
		return interned
	}

	symbolInternMu.Lock()
	interned, ok = symbolIntern[symbol]

	if !ok {
		interned = symbol
		symbolIntern[symbol] = symbol
	}

	symbolInternMu.Unlock()

	return interned
}

type HindsightRun struct {
	ID             string            `json:"id"`
	StartedAt      string            `json:"startedAt"`
	CodeCommit     string            `json:"codeCommit"`
	BuildID        string            `json:"buildId"`
	ConfigDigest   string            `json:"configDigest"`
	SchemaVersions map[string]string `json:"schemaVersions,omitempty"`
	Integrity      string            `json:"integrity"`
	Positions      int               `json:"positions"`
}

type HindsightExecutionFact struct {
	OrderID       string `json:"orderId,omitempty"`
	ClientOrderID string `json:"clientOrderId,omitempty"`
	ExecID        string `json:"execId,omitempty"`
	Side          string `json:"side,omitempty"`
	OrderStatus   string `json:"orderStatus,omitempty"`
	LastQty       string `json:"lastQty,omitempty"`
	LastPrice     string `json:"lastPrice,omitempty"`
	CumQty        string `json:"cumQty,omitempty"`
	CumCost       string `json:"cumCost,omitempty"`
	AvgPrice      string `json:"avgPrice,omitempty"`
	FeeUsdEquiv   string `json:"feeUsdEquiv,omitempty"`
	FillAt        string `json:"fillAt,omitempty"`
}

type HindsightLifecycleEvent struct {
	DecisionID string                  `json:"decisionId"`
	Symbol     string                  `json:"symbol"`
	Kind       string                  `json:"kind"`
	Action     string                  `json:"action"`
	At         string                  `json:"at"`
	Execution  *HindsightExecutionFact `json:"execution,omitempty"`
	CaptureSeq int64                   `json:"captureSeq,omitempty"`
}

type TimelineQuery struct {
	Run        string
	Symbol     string
	Coordinate string
	Axis       string
	Buckets    int
	From       int64
	To         int64
	Symbols    bool
}

type HindsightTimelineBucket struct {
	Index                int     `json:"index"`
	FromSequence         int64   `json:"fromSequence"`
	ToSequence           int64   `json:"toSequence"`
	FromAt               string  `json:"fromAt"`
	ToAt                 string  `json:"toAt"`
	ObservedFromSequence int64   `json:"observedFromSequence"`
	ObservedToSequence   int64   `json:"observedToSequence"`
	ObservedFromAt       string  `json:"observedFromAt"`
	ObservedToAt         string  `json:"observedToAt"`
	Observations         int     `json:"observations"`
	Tickers              int     `json:"tickers"`
	Trades               int     `json:"trades"`
	TradeQty             float64 `json:"tradeQty"`
	Defined              bool    `json:"defined"`
	Open                 float64 `json:"open"`
	High                 float64 `json:"high"`
	Low                  float64 `json:"low"`
	Close                float64 `json:"close"`
	SpreadFraction       float64 `json:"spreadFraction"`
	HasSpreadFraction    bool    `json:"hasSpreadFraction"`
	TouchDepth           float64 `json:"touchDepth"`
	HasTouchDepth        bool    `json:"hasTouchDepth"`
	CaptureRate          float64 `json:"captureRate"`
	HasCaptureRate       bool    `json:"hasCaptureRate"`
}

type HindsightTimelineSpan struct {
	FromSequence int64  `json:"fromSequence"`
	ToSequence   int64  `json:"toSequence"`
	FromAt       string `json:"fromAt"`
	ToAt         string `json:"toAt"`
}

type HindsightSymbolSummary struct {
	Symbol           string  `json:"symbol"`
	Observations     int     `json:"observations"`
	Defined          int     `json:"defined"`
	Tickers          int     `json:"tickers"`
	Trades           int     `json:"trades"`
	FirstSequence    int64   `json:"firstSequence"`
	LastSequence     int64   `json:"lastSequence"`
	FirstAt          string  `json:"firstAt"`
	LastAt           string  `json:"lastAt"`
	Episodes         int     `json:"episodes"`
	InsufficientData bool    `json:"insufficientData"`
	TopExcursion     float64 `json:"topExcursion"`
	PriceEpisodes    int     `json:"priceEpisodes"`
	RegimeEpisodes   int     `json:"regimeEpisodes"`
}

type HindsightStreamSpan struct {
	Stream       string `json:"stream"`
	Epoch        int64  `json:"epoch"`
	FromSequence int64  `json:"fromSequence"`
	ToSequence   int64  `json:"toSequence"`
	FromAt       string `json:"fromAt"`
	ToAt         string `json:"toAt"`
	Frames       int    `json:"frames"`
	Reconnect    bool   `json:"reconnect"`
}

type HindsightDiscoveryPolicy struct {
	Coordinate        string  `json:"coordinate"`
	FloorExcursion    float64 `json:"floorExcursion"`
	ExcursionSigmas   float64 `json:"excursionSigmas"`
	ExcursionHorizon  int     `json:"excursionHorizon"`
	RetraceFraction   float64 `json:"retraceFraction"`
	RegimeWindow      int     `json:"regimeWindow"`
	RegimeBaseline    int     `json:"regimeBaseline"`
	VolatilityRatio   float64 `json:"volatilityRatio"`
	SpreadRatio       float64 `json:"spreadRatio"`
	DepthRatio        float64 `json:"depthRatio"`
	ArrivalRatio      float64 `json:"arrivalRatio"`
	MinRegimeSpan     int     `json:"minRegimeSpan"`
	MinObservations   int     `json:"minObservations"`
	MaxEpisodesPerSet int     `json:"maxEpisodesPerSet"`
}

type HindsightDiscovery struct {
	Symbol           string                   `json:"symbol"`
	Coordinate       string                   `json:"coordinate"`
	Policy           HindsightDiscoveryPolicy `json:"policy"`
	Observations     int                      `json:"observations"`
	Defined          int                      `json:"defined"`
	Undefined        int                      `json:"undefined"`
	Sigma            float64                  `json:"sigma"`
	HasSigma         bool                     `json:"hasSigma"`
	QualifyingMove   float64                  `json:"qualifyingMove"`
	Episodes         []any                    `json:"episodes"`
	InsufficientData bool                     `json:"insufficientData"`
}

type HindsightTimeline struct {
	Run               string                   `json:"run"`
	Symbol            string                   `json:"symbol"`
	Coordinate        string                   `json:"coordinate"`
	Policy            HindsightDiscoveryPolicy `json:"policy"`
	Axis              string                   `json:"axis"`
	Span              HindsightTimelineSpan    `json:"span"`
	RunSpan           HindsightTimelineSpan    `json:"runSpan"`
	Buckets           []HindsightTimelineBucket `json:"buckets"`
	Discovery         HindsightDiscovery       `json:"discovery"`
	Streams           []HindsightStreamSpan    `json:"streams"`
	Symbols           []HindsightSymbolSummary `json:"symbols"`
	TotalObservations int                      `json:"totalObservations"`
	TotalSymbols      int                      `json:"totalSymbols"`
	IndexedAt         string                   `json:"indexedAt"`
}

type HindsightCaptureIdentity struct {
	Run            string `json:"run"`
	Sequence       int64  `json:"sequence"`
	Stream         string `json:"stream"`
	StreamEpoch    int64  `json:"streamEpoch"`
	StreamSequence int64  `json:"streamSequence"`
}

type HindsightCapture struct {
	Identity   HindsightCaptureIdentity `json:"identity"`
	Kind       string                   `json:"kind"`
	Endpoint   string                   `json:"endpoint"`
	ReceivedAt string                   `json:"receivedAt"`
}

type HindsightEnvelopeManifest struct {
	Envelope struct {
		Origin  HindsightCaptureIdentity `json:"origin"`
		Ordinal int                      `json:"ordinal"`
	} `json:"envelope"`
	Workload   string `json:"workload"`
	DomainKind string `json:"domainKind"`
	Symbol     string `json:"symbol"`
}

type HindsightEnvelope struct {
	Run       string                      `json:"run"`
	Sequence  int64                       `json:"sequence"`
	Capture   HindsightCapture            `json:"capture"`
	Payload   string                      `json:"payload"`
	Manifests []HindsightEnvelopeManifest `json:"manifests"`
	Witnesses []any                       `json:"witnesses"`
}

type HindsightState struct {
	Envelope struct {
		Origin  HindsightCaptureIdentity `json:"origin"`
		Ordinal int                      `json:"ordinal"`
	} `json:"envelope"`
	Payload string `json:"payload"`
}

type HindsightGap struct {
	RunID    string `json:"runId"`
	Encoding string `json:"encoding"`
	Sequence int64  `json:"sequence"`
	Detail   string `json:"detail,omitempty"`
}

type ResidentMetric struct {
	Key             string  `json:"key"`
	Label           string  `json:"label"`
	Raw             float64 `json:"raw"`
	Normalized      float64 `json:"normalized"`
	HasNormalized   bool    `json:"hasNormalized"`
	Standardized    float64 `json:"standardized"`
	HasStandardized bool    `json:"hasStandardized"`
	Unit            string  `json:"unit"`
	Timescale       string  `json:"timescale"`
}

type ResidentMeasurement struct {
	Source   string `json:"source"`
	Identity string `json:"identity"`
	Origin   struct {
		Origin  HindsightCaptureIdentity `json:"origin"`
		Ordinal int                      `json:"ordinal"`
	} `json:"origin"`
	AtNs       int64            `json:"atNs"`
	AgeNs      int64            `json:"ageNs"`
	HasAge     bool             `json:"hasAge"`
	Carried    bool             `json:"carried"`
	Maturity   float64          `json:"maturity"`
	SNR        float64          `json:"snr"`
	SNRDefined bool             `json:"snrDefined"`
	Metrics    []ResidentMetric `json:"metrics"`
}

type HindsightResident struct {
	Run          string                `json:"run"`
	Symbol       string                `json:"symbol"`
	Sequence     int64                 `json:"sequence"`
	Ordinal      int                   `json:"ordinal"`
	At           string                `json:"at"`
	Signals      []ResidentMeasurement `json:"signals"`
	Categories   []any                 `json:"categories"`
	Perspectives []any                 `json:"perspectives"`
	Examined     int                   `json:"examined"`
	ReachedBack  int                   `json:"reachedBack"`
	Exhausted    bool                  `json:"exhausted"`
}

// Epochs returns distinct epoch partitions across canonical tables.
func (c *Catalog) Epochs(ctx context.Context) ([]int64, error) {
	c.cacheMu.RLock()

	if time.Since(c.epochsLoaded) < 5*time.Second && len(c.cachedEpochs) > 0 {
		cached := make([]int64, len(c.cachedEpochs))
		copy(cached, c.cachedEpochs)
		c.cacheMu.RUnlock()

		return cached, nil
	}

	c.cacheMu.RUnlock()

	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()

	if time.Since(c.epochsLoaded) < 30*time.Second && len(c.cachedEpochs) > 0 {
		cached := make([]int64, len(c.cachedEpochs))
		copy(cached, c.cachedEpochs)

		return cached, nil
	}

	epochSet := make(map[int64]struct{})

	for epochKey := range c.timelineIndex {
		epochSet[epochKey] = struct{}{}
	}

	checkTable := func(tableName string) {
		loaded, err := c.Load(ctx, tableName)

		if err != nil {
			return
		}

		tasks, err := loaded.Scan().PlanFiles(ctx)

		if err != nil {
			return
		}

		for _, task := range tasks {
			partitionMap := task.File.Partition()
			foundPartition := false

			for _, pVal := range partitionMap {
				switch v := pVal.(type) {
				case int64:
					epochSet[v] = struct{}{}
					foundPartition = true
				case int:
					epochSet[int64(v)] = struct{}{}
					foundPartition = true
				case int32:
					epochSet[int64(v)] = struct{}{}
					foundPartition = true
				case float64:
					epochSet[int64(v)] = struct{}{}
					foundPartition = true
				}
			}

			if !foundPartition {
				batches, err := c.scan(ctx, tableName, []string{"epoch"})

				if err == nil {
					for batch, bErr := range batches {
						if bErr != nil {
							continue
						}

						col := batch.Column(0)

						for rowIdx := range int(batch.NumRows()) {
							epochSet[num(col, rowIdx)] = struct{}{}
						}
					}
				}

				break
			}
		}
	}

	checkTable(Positions)
	checkTable(Executions)
	checkTable(Decisions)
	checkTable(Models)

	if len(epochSet) == 0 {
		checkTable(SpotTicker)
		checkTable(Measurements)
	}

	epochs := make([]int64, 0, len(epochSet))

	for epoch := range epochSet {
		epochs = append(epochs, epoch)
	}

	slices.Sort(epochs)

	c.cachedEpochs = make([]int64, len(epochs))
	copy(c.cachedEpochs, epochs)
	c.epochsLoaded = time.Now()

	return epochs, nil
}

// Runs projects all known epochs as HindsightRun descriptors.
func (c *Catalog) Runs(ctx context.Context) ([]HindsightRun, error) {
	epochs, err := c.Epochs(ctx)

	if err != nil {
		return nil, err
	}

	runs := make([]HindsightRun, 0, len(epochs))

	for _, epoch := range epochs {
		positions, err := c.Positions(ctx, epoch, 0)

		if err != nil {
			return nil, err
		}

		startedAt := time.Now().UTC()

		if epoch > 1000000000 {
			startedAt = time.Unix(epoch, 0).UTC()
		}

		if len(positions) > 0 && positions[0].EntryAt != nil {
			startedAt = positions[0].EntryAt.UTC()
		}

		runs = append(runs, HindsightRun{
			ID:           strconv.FormatInt(epoch, 10),
			StartedAt:    startedAt.Format(time.RFC3339),
			BuildID:      "symm",
			Integrity:    "COMPLETE",
			Positions:    len(positions),
		})
	}

	return runs, nil
}

// Lifecycle projects positions, decisions, and executions for an epoch.
func (c *Catalog) Lifecycle(ctx context.Context, epoch int64) ([]HindsightLifecycleEvent, error) {
	positions, err := c.Positions(ctx, epoch, 0)

	if err != nil {
		return nil, err
	}

	executions, err := c.Executions(ctx, epoch, 0)

	if err != nil {
		return nil, err
	}

	events := make([]HindsightLifecycleEvent, 0, len(positions)*2+len(executions))

	for _, pos := range positions {
		decisionID := fmt.Sprintf("%s-%d", pos.Symbol, pos.Tick)

		if pos.EntryAt != nil {
			entryAtStr := pos.EntryAt.UTC().Format(time.RFC3339)

			events = append(events, HindsightLifecycleEvent{
				DecisionID: decisionID,
				Symbol:     pos.Symbol,
				Kind:       "position_open",
				Action:     "ENTER",
				At:         entryAtStr,
				CaptureSeq: pos.Tick,
			})

			lastPriceStr := ""
			lastQtyStr := ""
			feeStr := ""

			if pos.EntryPrice != nil {
				lastPriceStr = pos.EntryPrice.String()
			}

			if pos.Qty != nil {
				lastQtyStr = pos.Qty.String()
			}

			if pos.EntryFee != nil {
				feeStr = pos.EntryFee.String()
			}

			events = append(events, HindsightLifecycleEvent{
				DecisionID: decisionID,
				Symbol:     pos.Symbol,
				Kind:       "entry_fill",
				Action:     "BUY",
				At:         entryAtStr,
				CaptureSeq: pos.Tick,
				Execution: &HindsightExecutionFact{
					AvgPrice:    lastPriceStr,
					LastPrice:   lastPriceStr,
					CumQty:      lastQtyStr,
					LastQty:     lastQtyStr,
					FeeUsdEquiv: feeStr,
					FillAt:      entryAtStr,
				},
			})
		}

		if pos.ExitAt != nil {
			exitAtStr := pos.ExitAt.UTC().Format(time.RFC3339)

			exitPriceStr := ""
			qtyStr := ""
			feeStr := ""

			if pos.ExitPrice != nil {
				exitPriceStr = pos.ExitPrice.String()
			}

			if pos.Qty != nil {
				qtyStr = pos.Qty.String()
			}

			if pos.ExitFee != nil {
				feeStr = pos.ExitFee.String()
			}

			events = append(events, HindsightLifecycleEvent{
				DecisionID: decisionID,
				Symbol:     pos.Symbol,
				Kind:       "exit_fill",
				Action:     "SELL",
				At:         exitAtStr,
				CaptureSeq: pos.Tick,
				Execution: &HindsightExecutionFact{
					AvgPrice:    exitPriceStr,
					LastPrice:   exitPriceStr,
					CumQty:      qtyStr,
					LastQty:     qtyStr,
					FeeUsdEquiv: feeStr,
					FillAt:      exitAtStr,
				},
			})

			events = append(events, HindsightLifecycleEvent{
				DecisionID: decisionID,
				Symbol:     pos.Symbol,
				Kind:       "position_close",
				Action:     "EXIT",
				At:         exitAtStr,
				CaptureSeq: pos.Tick,
			})
		}
	}

	for _, exec := range executions {
		side := exec.Side

		if side == "" {
			side = "BUY"
		}

		kind := "entry_fill"

		if side == "SELL" || side == "sell" {
			kind = "exit_fill"
		}

		venueAtStr := exec.VenueAt.UTC().Format(time.RFC3339)
		lastPriceStr := ""
		lastQtyStr := ""
		cumQtyStr := ""
		avgPriceStr := ""
		feeStr := ""

		if exec.LastPrice != nil {
			lastPriceStr = exec.LastPrice.String()
		}

		if exec.LastQty != nil {
			lastQtyStr = exec.LastQty.String()
		}

		if exec.CumQty != nil {
			cumQtyStr = exec.CumQty.String()
		}

		if exec.AvgPrice != nil {
			avgPriceStr = exec.AvgPrice.String()
		}

		if exec.FeeUsdEquiv != nil {
			feeStr = exec.FeeUsdEquiv.String()
		}

		events = append(events, HindsightLifecycleEvent{
			DecisionID: fmt.Sprintf("%s-%d", exec.Symbol, exec.Tick),
			Symbol:     exec.Symbol,
			Kind:       kind,
			Action:     side,
			At:         venueAtStr,
			CaptureSeq: exec.Tick,
			Execution: &HindsightExecutionFact{
				OrderID:       exec.OrderID,
				ClientOrderID: strconv.FormatInt(exec.OrderUserRef, 10),
				ExecID:        exec.ExecID,
				Side:          side,
				OrderStatus:   exec.OrderStatus,
				LastQty:       lastQtyStr,
				LastPrice:     lastPriceStr,
				CumQty:        cumQtyStr,
				AvgPrice:      avgPriceStr,
				FeeUsdEquiv:   feeStr,
				FillAt:        venueAtStr,
			},
		})
	}

	sort.Slice(events, func(left, right int) bool {
		return events[left].CaptureSeq < events[right].CaptureSeq
	})

	return events, nil
}

type observationPoint struct {
	tick     int64
	venueAt  time.Time
	symbol   string
	price    float64
	bid      float64
	ask      float64
	bidQty   float64
	askQty   float64
	tradeQty float64
	isTicker bool
	isTrade  bool
}

type runTimelineIndex struct {
	epoch             int64
	indexedAt         time.Time
	minTick           int64
	maxTick           int64
	minAt             time.Time
	maxAt             time.Time
	totalObservations int
	symbols           []HindsightSymbolSummary
	symbolPoints      map[string][]observationPoint
	allPoints         []observationPoint
}

func newRunTimelineIndex(epoch int64, tickers []SpotTickerRow, trades []SpotTradeRow) *runTimelineIndex {
	symbolSummaries := make(map[string]*HindsightSymbolSummary)
	symbolPoints := make(map[string][]observationPoint)
	allPoints := make([]observationPoint, 0, len(tickers)+len(trades))

	recordSummary := func(symbol string, tick int64, venueAt time.Time, isTicker, isTrade bool, price float64) {
		summary := symbolSummaries[symbol]

		if summary == nil {
			venueStr := venueAt.UTC().Format(time.RFC3339)
			summary = &HindsightSymbolSummary{
				Symbol:        symbol,
				FirstSequence: tick,
				LastSequence:  tick,
				FirstAt:       venueStr,
				LastAt:        venueStr,
			}
			symbolSummaries[symbol] = summary
		}

		summary.Observations++

		if price > 0 {
			summary.Defined++
		}

		if isTicker {
			summary.Tickers++
		}

		if isTrade {
			summary.Trades++
		}

		if tick < summary.FirstSequence {
			summary.FirstSequence = tick
			summary.FirstAt = venueAt.UTC().Format(time.RFC3339)
		}

		if tick > summary.LastSequence {
			summary.LastSequence = tick
			summary.LastAt = venueAt.UTC().Format(time.RFC3339)
		}

		summary.InsufficientData = summary.Observations < 10
	}

	for _, ticker := range tickers {
		price := ticker.Last

		if price <= 0 && ticker.Bid > 0 && ticker.Ask > 0 {
			price = (ticker.Bid + ticker.Ask) / 2
		}

		sym := internSymbol(ticker.Symbol)
		recordSummary(sym, ticker.Tick, ticker.VenueAt, true, false, price)

		point := observationPoint{
			tick:     ticker.Tick,
			venueAt:  ticker.VenueAt,
			symbol:   sym,
			price:    price,
			bid:      ticker.Bid,
			ask:      ticker.Ask,
			bidQty:   ticker.BidQty,
			askQty:   ticker.AskQty,
			isTicker: true,
		}

		allPoints = append(allPoints, point)
		symbolPoints[sym] = append(symbolPoints[sym], point)
	}

	for _, trade := range trades {
		sym := internSymbol(trade.Symbol)
		recordSummary(sym, trade.Tick, trade.VenueAt, false, true, trade.Price)

		point := observationPoint{
			tick:     trade.Tick,
			venueAt:  trade.VenueAt,
			symbol:   sym,
			price:    trade.Price,
			tradeQty: trade.Qty,
			isTrade:  true,
		}

		allPoints = append(allPoints, point)
		symbolPoints[sym] = append(symbolPoints[sym], point)
	}

	symbols := make([]HindsightSymbolSummary, 0, len(symbolSummaries))

	for _, summary := range symbolSummaries {
		symbols = append(symbols, *summary)
	}

	sort.Slice(symbols, func(left, right int) bool {
		return symbols[left].Observations > symbols[right].Observations
	})

	sort.Slice(allPoints, func(left, right int) bool {
		return allPoints[left].tick < allPoints[right].tick
	})

	for sym := range symbolPoints {
		pts := symbolPoints[sym]
		sort.Slice(pts, func(left, right int) bool {
			return pts[left].tick < pts[right].tick
		})
	}

	var minTick int64
	var maxTick int64
	var minAt time.Time
	var maxAt time.Time

	if len(allPoints) > 0 {
		minTick = allPoints[0].tick
		maxTick = allPoints[len(allPoints)-1].tick
		minAt = allPoints[0].venueAt
		maxAt = allPoints[len(allPoints)-1].venueAt
	}

	return &runTimelineIndex{
		epoch:             epoch,
		indexedAt:         time.Now(),
		minTick:           minTick,
		maxTick:           maxTick,
		minAt:             minAt,
		maxAt:             maxAt,
		totalObservations: len(allPoints),
		symbols:           symbols,
		symbolPoints:      symbolPoints,
		allPoints:         allPoints,
	}
}

func (index *runTimelineIndex) appendNewRows(tickers []SpotTickerRow, trades []SpotTradeRow) {
	newPoints := make([]observationPoint, 0, len(tickers)+len(trades))
	symbolSummaries := make(map[string]*HindsightSymbolSummary, len(index.symbols))

	for idx := range index.symbols {
		symbolSummaries[index.symbols[idx].Symbol] = &index.symbols[idx]
	}

	recordSummary := func(symbol string, tick int64, venueAt time.Time, isTicker, isTrade bool, price float64) {
		summary := symbolSummaries[symbol]

		if summary == nil {
			venueStr := venueAt.UTC().Format(time.RFC3339)
			summary = &HindsightSymbolSummary{
				Symbol:        symbol,
				FirstSequence: tick,
				LastSequence:  tick,
				FirstAt:       venueStr,
				LastAt:        venueStr,
			}
			symbolSummaries[symbol] = summary
		}

		summary.Observations++

		if price > 0 {
			summary.Defined++
		}

		if isTicker {
			summary.Tickers++
		}

		if isTrade {
			summary.Trades++
		}

		if tick < summary.FirstSequence {
			summary.FirstSequence = tick
			summary.FirstAt = venueAt.UTC().Format(time.RFC3339)
		}

		if tick > summary.LastSequence {
			summary.LastSequence = tick
			summary.LastAt = venueAt.UTC().Format(time.RFC3339)
		}

		summary.InsufficientData = summary.Observations < 10
	}

	for _, ticker := range tickers {
		price := ticker.Last

		if price <= 0 && ticker.Bid > 0 && ticker.Ask > 0 {
			price = (ticker.Bid + ticker.Ask) / 2
		}

		sym := internSymbol(ticker.Symbol)
		recordSummary(sym, ticker.Tick, ticker.VenueAt, true, false, price)

		point := observationPoint{
			tick:     ticker.Tick,
			venueAt:  ticker.VenueAt,
			symbol:   sym,
			price:    price,
			bid:      ticker.Bid,
			ask:      ticker.Ask,
			bidQty:   ticker.BidQty,
			askQty:   ticker.AskQty,
			isTicker: true,
		}

		newPoints = append(newPoints, point)
		index.symbolPoints[sym] = append(index.symbolPoints[sym], point)
	}

	for _, trade := range trades {
		sym := internSymbol(trade.Symbol)
		recordSummary(sym, trade.Tick, trade.VenueAt, false, true, trade.Price)

		point := observationPoint{
			tick:     trade.Tick,
			venueAt:  trade.VenueAt,
			symbol:   sym,
			price:    trade.Price,
			tradeQty: trade.Qty,
			isTrade:  true,
		}

		newPoints = append(newPoints, point)
		index.symbolPoints[sym] = append(index.symbolPoints[sym], point)
	}

	index.allPoints = append(index.allPoints, newPoints...)

	if len(newPoints) > 0 {
		sort.Slice(index.allPoints, func(left, right int) bool {
			return index.allPoints[left].tick < index.allPoints[right].tick
		})

		index.maxTick = index.allPoints[len(index.allPoints)-1].tick
		index.maxAt = index.allPoints[len(index.allPoints)-1].venueAt
	}

	symbols := make([]HindsightSymbolSummary, 0, len(symbolSummaries))

	for _, summary := range symbolSummaries {
		symbols = append(symbols, *summary)
	}

	sort.Slice(symbols, func(left, right int) bool {
		return symbols[left].Observations > symbols[right].Observations
	})

	index.symbols = symbols
	index.totalObservations = len(index.allPoints)
	index.indexedAt = time.Now()
}

func (c *Catalog) getOrBuildTimelineIndex(ctx context.Context, epoch int64) (*runTimelineIndex, error) {
	c.cacheMu.RLock()
	index := c.timelineIndex[epoch]

	if index != nil && time.Since(index.indexedAt) < 10*time.Second {
		c.cacheMu.RUnlock()

		return index, nil
	}

	c.cacheMu.RUnlock()

	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()

	if c.timelineIndex == nil {
		c.timelineIndex = make(map[int64]*runTimelineIndex)
	}

	index = c.timelineIndex[epoch]

	if index != nil && time.Since(index.indexedAt) < 10*time.Second {
		return index, nil
	}

	if index != nil {
		newTickers, err := c.SpotTickerScan(
			ctx, epoch, afterTick(index.maxTick), 0,
			"tick", "symbol", "venue_at", "received_at", "bid", "bid_qty", "ask", "ask_qty", "last",
		)

		if err != nil {
			return index, nil
		}

		newTrades, err := c.SpotTradeScan(
			ctx, epoch, afterTick(index.maxTick), 0,
			"tick", "symbol", "venue_at", "received_at", "price", "qty",
		)

		if err != nil {
			return index, nil
		}

		if len(newTickers) == 0 && len(newTrades) == 0 {
			index.indexedAt = time.Now()

			return index, nil
		}

		index.appendNewRows(newTickers, newTrades)

		return index, nil
	}

	return c.buildTimelineIndex(ctx, epoch)
}

func (c *Catalog) buildTimelineIndex(ctx context.Context, epoch int64) (*runTimelineIndex, error) {
	tickers, err := c.SpotTickerScan(
		ctx, epoch, iceberg.AlwaysTrue{}, 0,
		"tick", "symbol", "venue_at", "received_at", "bid", "bid_qty", "ask", "ask_qty", "last",
	)

	if err != nil {
		return nil, err
	}

	trades, err := c.SpotTradeScan(
		ctx, epoch, iceberg.AlwaysTrue{}, 0,
		"tick", "symbol", "venue_at", "received_at", "price", "qty",
	)

	if err != nil {
		return nil, err
	}

	index := newRunTimelineIndex(epoch, tickers, trades)
	c.timelineIndex[epoch] = index

	return index, nil
}

// Timeline projects bucketed market timeline evidence from SpotTicker and SpotTrade.
func (c *Catalog) Timeline(ctx context.Context, query TimelineQuery) (*HindsightTimeline, error) {
	epoch, err := strconv.ParseInt(query.Run, 10, 64)

	if err != nil {
		epoch = 1
	}

	index, err := c.getOrBuildTimelineIndex(ctx, epoch)

	if err != nil {
		return nil, err
	}

	selectedSymbol := query.Symbol

	if selectedSymbol == "" && len(index.symbols) > 0 {
		selectedSymbol = index.symbols[0].Symbol
	}

	symbolPoints := index.symbolPoints[selectedSymbol]

	minTick := index.minTick
	maxTick := index.maxTick
	minAt := index.minAt
	maxAt := index.maxAt

	if len(symbolPoints) > 0 {
		minTick = symbolPoints[0].tick
		maxTick = symbolPoints[len(symbolPoints)-1].tick
		minAt = symbolPoints[0].venueAt
		maxAt = symbolPoints[len(symbolPoints)-1].venueAt
	}

	fromSeq := minTick
	toSeq := maxTick

	if query.From > 0 {
		fromSeq = query.From
	}

	if query.To > 0 && query.To >= fromSeq {
		toSeq = query.To
	}

	bucketCount := query.Buckets

	if bucketCount <= 0 {
		bucketCount = 200
	}

	buckets := make([]HindsightTimelineBucket, bucketCount)
	step := float64(max(1, toSeq-fromSeq+1)) / float64(bucketCount)

	pointIdx := sort.Search(len(symbolPoints), func(searchIdx int) bool {
		return symbolPoints[searchIdx].tick >= fromSeq
	})

	for bucketIdx := 0; bucketIdx < bucketCount; bucketIdx++ {
		bStart := fromSeq + int64(float64(bucketIdx)*step)
		bEnd := fromSeq + int64(float64(bucketIdx+1)*step) - 1

		if bucketIdx == bucketCount-1 {
			bEnd = toSeq
		}

		bucket := HindsightTimelineBucket{
			Index:        bucketIdx,
			FromSequence: bStart,
			ToSequence:   bEnd,
			FromAt:       minAt.Add(time.Duration(bucketIdx) * time.Second).UTC().Format(time.RFC3339),
			ToAt:         minAt.Add(time.Duration(bucketIdx+1) * time.Second).UTC().Format(time.RFC3339),
		}

		var firstPrice float64
		var lastPrice float64
		var maxPrice = -math.MaxFloat64
		var minPrice = math.MaxFloat64
		var sumSpread float64
		var spreadCount int
		var sumDepth float64
		var depthCount int

		for pointIdx < len(symbolPoints) && symbolPoints[pointIdx].tick <= bEnd {
			point := symbolPoints[pointIdx]

			if point.tick >= bStart {
				bucket.Observations++

				if bucket.ObservedFromSequence == 0 || point.tick < bucket.ObservedFromSequence {
					bucket.ObservedFromSequence = point.tick
					bucket.ObservedFromAt = point.venueAt.UTC().Format(time.RFC3339)
				}

				if point.tick > bucket.ObservedToSequence {
					bucket.ObservedToSequence = point.tick
					bucket.ObservedToAt = point.venueAt.UTC().Format(time.RFC3339)
				}

				if point.isTicker {
					bucket.Tickers++

					if point.bid > 0 && point.ask > point.bid && point.price > 0 {
						sumSpread += (point.ask - point.bid) / point.price
						spreadCount++
					}

					if point.bidQty > 0 || point.askQty > 0 {
						sumDepth += point.bidQty + point.askQty
						depthCount++
					}
				}

				if point.isTrade {
					bucket.Trades++
					bucket.TradeQty += point.tradeQty
				}

				if point.price > 0 {
					if !bucket.Defined {
						firstPrice = point.price
						bucket.Defined = true
					}

					lastPrice = point.price

					if point.price > maxPrice {
						maxPrice = point.price
					}

					if point.price < minPrice {
						minPrice = point.price
					}
				}
			}

			pointIdx++
		}

		if bucket.Defined {
			bucket.Open = firstPrice
			bucket.Close = lastPrice
			bucket.High = maxPrice
			bucket.Low = minPrice
		}

		if spreadCount > 0 {
			bucket.SpreadFraction = sumSpread / float64(spreadCount)
			bucket.HasSpreadFraction = true
		}

		if depthCount > 0 {
			bucket.TouchDepth = sumDepth / float64(depthCount)
			bucket.HasTouchDepth = true
		}

		bucket.CaptureRate = float64(bucket.Observations)
		bucket.HasCaptureRate = true

		buckets[bucketIdx] = bucket
	}

	coordinate := query.Coordinate

	if coordinate == "" {
		coordinate = "midpoint"
	}

	axis := query.Axis

	if axis == "" {
		axis = "time"
	}

	policy := HindsightDiscoveryPolicy{
		Coordinate:        coordinate,
		FloorExcursion:    0.005,
		ExcursionSigmas:   2.0,
		ExcursionHorizon:  32,
		RetraceFraction:   0.5,
		RegimeWindow:      64,
		RegimeBaseline:    128,
		VolatilityRatio:   1.5,
		SpreadRatio:       1.5,
		DepthRatio:        0.5,
		ArrivalRatio:      2.0,
		MinRegimeSpan:     16,
		MinObservations:   32,
		MaxEpisodesPerSet: 128,
	}

	timeline := &HindsightTimeline{
		Run:        query.Run,
		Symbol:     selectedSymbol,
		Coordinate: coordinate,
		Policy:     policy,
		Axis:       axis,
		Span: HindsightTimelineSpan{
			FromSequence: fromSeq,
			ToSequence:   toSeq,
			FromAt:       minAt.UTC().Format(time.RFC3339),
			ToAt:         maxAt.UTC().Format(time.RFC3339),
		},
		RunSpan: HindsightTimelineSpan{
			FromSequence: minTick,
			ToSequence:   maxTick,
			FromAt:       minAt.UTC().Format(time.RFC3339),
			ToAt:         maxAt.UTC().Format(time.RFC3339),
		},
		Buckets: buckets,
		Discovery: HindsightDiscovery{
			Symbol:           selectedSymbol,
			Coordinate:       coordinate,
			Policy:           policy,
			Observations:     len(symbolPoints),
			Defined:          len(symbolPoints),
			Episodes:         []any{},
			InsufficientData: len(symbolPoints) < 10,
		},
		Streams: []HindsightStreamSpan{
			{
				Stream:       "spot",
				Epoch:        epoch,
				FromSequence: minTick,
				ToSequence:   maxTick,
				FromAt:       minAt.UTC().Format(time.RFC3339),
				ToAt:         maxAt.UTC().Format(time.RFC3339),
				Frames:       len(symbolPoints),
			},
		},
		Symbols:           index.symbols,
		TotalObservations: index.totalObservations,
		TotalSymbols:      len(index.symbols),
		IndexedAt:         index.indexedAt.UTC().Format(time.RFC3339),
	}

	return timeline, nil
}

// Captures lists market frames strictly after afterTick for a given epoch.
func (c *Catalog) Captures(ctx context.Context, epoch int64, afterTick int64) ([]HindsightCapture, error) {
	epochStr := strconv.FormatInt(epoch, 10)
	captures := make([]HindsightCapture, 0, 48)

	index, err := c.getOrBuildTimelineIndex(ctx, epoch)

	if err == nil && index != nil && len(index.allPoints) > 0 {
		startIdx := sort.Search(len(index.allPoints), func(searchIdx int) bool {
			return index.allPoints[searchIdx].tick > afterTick
		})

		endIdx := min(len(index.allPoints), startIdx+48)
		slice := index.allPoints[startIdx:endIdx]

		for _, point := range slice {
			kind := "ticker"
			stream := "spot_ticker"

			if point.isTrade {
				kind = "trade"
				stream = "spot_trade"
			}

			captures = append(captures, HindsightCapture{
				Identity: HindsightCaptureIdentity{
					Run:            epochStr,
					Sequence:       point.tick,
					Stream:         stream,
					StreamEpoch:    1,
					StreamSequence: point.tick,
				},
				Kind:       kind,
				Endpoint:   "spot",
				ReceivedAt: point.venueAt.UTC().Format(time.RFC3339Nano),
			})
		}
	}

	if len(captures) == 0 {
		tickers, err := c.SpotTickerScan(ctx, epoch, afterTickExpr(afterTick), 48)

		if err != nil {
			return nil, err
		}

		for _, ticker := range tickers {
			captures = append(captures, HindsightCapture{
				Identity: HindsightCaptureIdentity{
					Run:            epochStr,
					Sequence:       ticker.Tick,
					Stream:         "spot_ticker",
					StreamEpoch:    1,
					StreamSequence: ticker.Tick,
				},
				Kind:       "ticker",
				Endpoint:   "spot",
				ReceivedAt: ticker.ReceivedAt.UTC().Format(time.RFC3339Nano),
			})
		}

		trades, err := c.SpotTradeScan(ctx, epoch, afterTickExpr(afterTick), 48)

		if err != nil {
			return nil, err
		}

		for _, trade := range trades {
			captures = append(captures, HindsightCapture{
				Identity: HindsightCaptureIdentity{
					Run:            epochStr,
					Sequence:       trade.Tick,
					Stream:         "spot_trade",
					StreamEpoch:    1,
					StreamSequence: trade.Tick,
				},
				Kind:       "trade",
				Endpoint:   "spot",
				ReceivedAt: trade.ReceivedAt.UTC().Format(time.RFC3339Nano),
			})
		}
	}

	l3, err := c.SpotLevel3Scan(ctx, epoch, afterTickExpr(afterTick), 48)

	if err != nil {
		return nil, err
	}

	for _, order := range l3 {
		captures = append(captures, HindsightCapture{
			Identity: HindsightCaptureIdentity{
				Run:            epochStr,
				Sequence:       order.Tick,
				Stream:         "spot_level3",
				StreamEpoch:    1,
				StreamSequence: order.Tick,
			},
			Kind:       "l3_touch",
			Endpoint:   "spot",
			ReceivedAt: order.ReceivedAt.UTC().Format(time.RFC3339Nano),
		})
	}

	sort.Slice(captures, func(left, right int) bool {
		return captures[left].Identity.Sequence < captures[right].Identity.Sequence
	})

	if len(captures) > 48 {
		captures = captures[:48]
	}

	return captures, nil
}

func afterTickExpr(tick int64) iceberg.BooleanExpression {
	return iceberg.GreaterThan(iceberg.Reference("tick"), tick)
}

// EnvelopeAt resolves one capture frame envelope at seq.
func (c *Catalog) EnvelopeAt(ctx context.Context, epoch int64, seq int64) (*HindsightEnvelope, error) {
	epochStr := strconv.FormatInt(epoch, 10)
	index, _ := c.getOrBuildTimelineIndex(ctx, epoch)

	if index != nil && len(index.allPoints) > 0 {
		matchIdx := sort.Search(len(index.allPoints), func(searchIdx int) bool {
			return index.allPoints[searchIdx].tick >= seq
		})

		if matchIdx < len(index.allPoints) && index.allPoints[matchIdx].tick == seq {
			point := index.allPoints[matchIdx]

			if point.isTicker {
				ticker := SpotTickerRow{
					Epoch:      epoch,
					Tick:       point.tick,
					Symbol:     point.symbol,
					VenueAt:    point.venueAt,
					ReceivedAt: point.venueAt,
					Bid:        point.bid,
					BidQty:     point.bidQty,
					Ask:        point.ask,
					AskQty:     point.askQty,
					Last:       point.price,
				}
				payloadBytes, _ := sonic.Marshal(ticker)
				identity := HindsightCaptureIdentity{
					Run:            epochStr,
					Sequence:       seq,
					Stream:         "spot_ticker",
					StreamEpoch:    1,
					StreamSequence: seq,
				}
				manifest := HindsightEnvelopeManifest{
					Workload:   "market",
					DomainKind: "spot_ticker",
					Symbol:     point.symbol,
				}
				manifest.Envelope.Origin = identity

				return &HindsightEnvelope{
					Run:      epochStr,
					Sequence: seq,
					Capture: HindsightCapture{
						Identity:   identity,
						Kind:       "ticker",
						Endpoint:   "spot",
						ReceivedAt: point.venueAt.UTC().Format(time.RFC3339Nano),
					},
					Payload:   string(payloadBytes),
					Manifests: []HindsightEnvelopeManifest{manifest},
					Witnesses: []any{},
				}, nil
			}

			if point.isTrade {
				trade := SpotTradeRow{
					Epoch:      epoch,
					Tick:       point.tick,
					Symbol:     point.symbol,
					VenueAt:    point.venueAt,
					ReceivedAt: point.venueAt,
					Price:      point.price,
					Qty:        point.tradeQty,
				}
				payloadBytes, _ := sonic.Marshal(trade)
				identity := HindsightCaptureIdentity{
					Run:            epochStr,
					Sequence:       seq,
					Stream:         "spot_trade",
					StreamEpoch:    1,
					StreamSequence: seq,
				}
				manifest := HindsightEnvelopeManifest{
					Workload:   "market",
					DomainKind: "spot_trade",
					Symbol:     point.symbol,
				}
				manifest.Envelope.Origin = identity

				return &HindsightEnvelope{
					Run:      epochStr,
					Sequence: seq,
					Capture: HindsightCapture{
						Identity:   identity,
						Kind:       "trade",
						Endpoint:   "spot",
						ReceivedAt: point.venueAt.UTC().Format(time.RFC3339Nano),
					},
					Payload:   string(payloadBytes),
					Manifests: []HindsightEnvelopeManifest{manifest},
					Witnesses: []any{},
				}, nil
			}
		}
	}

	tickers, err := c.SpotTickerScan(
		ctx, epoch, iceberg.EqualTo(iceberg.Reference("tick"), seq), 1,
	)

	if err == nil && len(tickers) > 0 {
		ticker := tickers[0]
		payloadBytes, _ := sonic.Marshal(ticker)
		identity := HindsightCaptureIdentity{
			Run:            epochStr,
			Sequence:       seq,
			Stream:         "spot_ticker",
			StreamEpoch:    1,
			StreamSequence: seq,
		}
		manifest := HindsightEnvelopeManifest{
			Workload:   "market",
			DomainKind: "spot_ticker",
			Symbol:     ticker.Symbol,
		}
		manifest.Envelope.Origin = identity

		return &HindsightEnvelope{
			Run:      epochStr,
			Sequence: seq,
			Capture: HindsightCapture{
				Identity:   identity,
				Kind:       "ticker",
				Endpoint:   "spot",
				ReceivedAt: ticker.ReceivedAt.UTC().Format(time.RFC3339Nano),
			},
			Payload:   string(payloadBytes),
			Manifests: []HindsightEnvelopeManifest{manifest},
			Witnesses: []any{},
		}, nil
	}

	trades, err := c.SpotTradeScan(
		ctx, epoch, iceberg.EqualTo(iceberg.Reference("tick"), seq), 1,
	)

	if err == nil && len(trades) > 0 {
		trade := trades[0]
		payloadBytes, _ := sonic.Marshal(trade)
		identity := HindsightCaptureIdentity{
			Run:            epochStr,
			Sequence:       seq,
			Stream:         "spot_trade",
			StreamEpoch:    1,
			StreamSequence: seq,
		}
		manifest := HindsightEnvelopeManifest{
			Workload:   "market",
			DomainKind: "spot_trade",
			Symbol:     trade.Symbol,
		}
		manifest.Envelope.Origin = identity

		return &HindsightEnvelope{
			Run:      epochStr,
			Sequence: seq,
			Capture: HindsightCapture{
				Identity:   identity,
				Kind:       "trade",
				Endpoint:   "spot",
				ReceivedAt: trade.ReceivedAt.UTC().Format(time.RFC3339Nano),
			},
			Payload:   string(payloadBytes),
			Manifests: []HindsightEnvelopeManifest{manifest},
			Witnesses: []any{},
		}, nil
	}

	l3, err := c.SpotLevel3Scan(
		ctx, epoch, iceberg.EqualTo(iceberg.Reference("tick"), seq), 1,
	)

	if err == nil && len(l3) > 0 {
		order := l3[0]
		payloadBytes, _ := sonic.Marshal(order)
		identity := HindsightCaptureIdentity{
			Run:            epochStr,
			Sequence:       seq,
			Stream:         "spot_level3",
			StreamEpoch:    1,
			StreamSequence: seq,
		}
		manifest := HindsightEnvelopeManifest{
			Workload:   "market",
			DomainKind: "spot_level3",
			Symbol:     order.Symbol,
		}
		manifest.Envelope.Origin = identity

		return &HindsightEnvelope{
			Run:      epochStr,
			Sequence: seq,
			Capture: HindsightCapture{
				Identity:   identity,
				Kind:       "l3_touch",
				Endpoint:   "spot",
				ReceivedAt: order.ReceivedAt.UTC().Format(time.RFC3339Nano),
			},
			Payload:   string(payloadBytes),
			Manifests: []HindsightEnvelopeManifest{manifest},
			Witnesses: []any{},
		}, nil
	}

	return nil, errnie.Error(errnie.Err(errnie.NotFound, fmt.Sprintf("envelope not found at tick %d", seq), nil))
}

// ResidentAt resolves causally held measurements at or before seq for a given epoch.
func (c *Catalog) ResidentAt(
	ctx context.Context, epoch int64, symbol string, seq int64, budget int,
) (*HindsightResident, error) {
	if budget <= 0 {
		budget = 64
	}

	startTick := max(0, seq-int64(budget))
	rangeFilter := iceberg.BooleanExpression(iceberg.NewAnd(
		iceberg.GreaterThanEqual(iceberg.Reference("tick"), startTick),
		iceberg.LessThanEqual(iceberg.Reference("tick"), seq),
	))

	if symbol != "" {
		rangeFilter = iceberg.NewAnd(rangeFilter, iceberg.EqualTo(iceberg.Reference("symbol"), symbol))
	}

	measurements, err := c.MeasurementsScan(
		ctx, epoch, rangeFilter, 0,
		"tick", "source", "symbol", "observed_at", "maturity", "snr", "snr_defined", "metrics",
	)

	if err != nil {
		return nil, err
	}

	held := make(map[string]MeasurementRow)

	for _, measurement := range measurements {
		held[measurement.Source] = measurement
	}

	sources := make([]string, 0, len(held))

	for src := range held {
		sources = append(sources, src)
	}

	slices.Sort(sources)

	signals := make([]ResidentMeasurement, 0, len(sources))
	epochStr := strconv.FormatInt(epoch, 10)

	for _, src := range sources {
		measurement := held[src]
		metrics := make([]ResidentMetric, 0, len(measurement.Metrics))
		metricKeys := make([]string, 0, len(measurement.Metrics))

		for metricKey := range measurement.Metrics {
			metricKeys = append(metricKeys, metricKey)
		}

		slices.Sort(metricKeys)

		for _, metricKey := range metricKeys {
			metricVal := measurement.Metrics[metricKey]

			metrics = append(metrics, ResidentMetric{
				Key:             metricKey,
				Label:           metricKey,
				Raw:             metricVal,
				Normalized:      metricVal,
				HasNormalized:   true,
				Standardized:    metricVal,
				HasStandardized: true,
			})
		}

		sig := ResidentMeasurement{
			Source:     measurement.Source,
			Identity:   measurement.Source,
			AtNs:       measurement.ObservedAt.UnixNano(),
			AgeNs:      time.Duration(seq - measurement.Tick).Nanoseconds(),
			HasAge:     true,
			Carried:    measurement.Tick < seq,
			Maturity:   measurement.Maturity,
			SNR:        measurement.SNR,
			SNRDefined: measurement.SNRDefined,
			Metrics:    metrics,
		}

		sig.Origin.Origin = HindsightCaptureIdentity{
			Run:            epochStr,
			Sequence:       measurement.Tick,
			Stream:         measurement.Source,
			StreamEpoch:    1,
			StreamSequence: measurement.Tick,
		}

		signals = append(signals, sig)
	}

	return &HindsightResident{
		Run:          epochStr,
		Symbol:       symbol,
		Sequence:     seq,
		Ordinal:      0,
		At:           time.Now().UTC().Format(time.RFC3339),
		Signals:      signals,
		Categories:   []any{},
		Perspectives: []any{},
		Examined:     len(measurements),
		ReachedBack:  budget,
		Exhausted:    false,
	}, nil
}
