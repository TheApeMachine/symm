package leadlag

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/bus"
	"github.com/theapemachine/symm/focus"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/market/perspectives/types"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/adaptive"
	"github.com/theapemachine/symm/rawdump"
	signalpool "github.com/theapemachine/symm/signal"
)

const (
	minLagFraction      = 0.35
	publishInterval     = 200 * time.Millisecond
	rawSubscriberID     = "signal/leadlag:raw"
	leadlagMarketSymbol = "market"
)

var leadlagDefaultBandEdges = []float64{0.25, 0.55, 0.75}

/*
Signal detects altcoins lagging a moving anchor pair (BTC/EUR) and maps the
lead-lag structure onto the anchor perspective. It is cross-asset: each
follower's verdict is its lagged Hayashi-Yoshida correlation against the anchor.

Anchor movement is derived from the anchor ticker ring over the lag search
window and scored against the anchor's own adaptive move baseline — not Kraken's
24h change_pct. Measurements publish for every symbol in the universe; focus
applies only to UI chart streams elsewhere.

Confidence is classification clarity — margin to the lag-fraction or correlation
boundary; SNR is how surprising that clarity is versus the follower's own recent
baseline, not the lag correlation strength.

| Category           | Lag structure                              |
|:-------------------|:-------------------------------------------|
| Anchor Stall       | anchor not moving — no lead to follow      |
| Decoupled Move     | follower uncorrelated with the anchor      |
| Inefficient Lag    | follows the anchor with an exploitable lag |
| Synchronized Drift | moves with the anchor, no usable lag       |

The cross-correlation recompute is throttled (publishInterval); it is O(ring ×
maxLagBars) per follower and would otherwise saturate a core at ticker rate.
*/
type Signal struct {
	ctx             context.Context
	cancel          context.CancelFunc
	pool            *qpool.Q
	broadcasts      map[string]*qpool.BroadcastGroup
	subscribers     map[string]*qpool.Subscriber
	symbols         sync.Map
	anchorBaseline  moveBaseline
	followerScratch []string
	lastPublishMu   sync.Mutex
	lastPublish     time.Time
	floor           *adaptive.SNRField
	rawDump         *rawdump.Writer
}

func NewSignal(ctx context.Context, pool *qpool.Q) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:            ctx,
		cancel:         cancel,
		pool:           pool,
		broadcasts:     make(map[string]*qpool.BroadcastGroup),
		subscribers:    make(map[string]*qpool.Subscriber),
		anchorBaseline: *newMoveBaseline(),
		floor:          adaptive.NewSNRField(),
		rawDump:        rawdump.Open("leadlag"),
	}

	for _, channel := range []string{"raw"} {
		signal.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		signal.subscribers[channel] = signal.broadcasts[channel].Subscribe(rawSubscriberID, 1024)
	}

	signal.broadcasts["measurements"] = bus.Group(pool, "measurements", 10*time.Millisecond)
	signal.broadcasts["ui"] = bus.Group(pool, "ui", 10*time.Millisecond)

	errnie.Info("signal/leadlag ready", "signal/leadlag")

	return signal
}

func (signal *Signal) Tick() error {
	for {
		select {
		case <-signal.ctx.Done():
			return signal.ctx.Err()
		case message := <-signal.subscribers["raw"].Incoming:
			if message == nil || message.Value == nil {
				continue
			}

			sm, ok := signalpool.SocketMessageFromValue(message.Value)

			if !ok {
				continue
			}

			switch sm.Channel {
			case public.TickerChannel:
				tickers := signalpool.GetTickers(sm)

				ingested := false

				for _, ticker := range tickers {
					if ticker.Last <= 0 {
						continue
					}

					at, err := signal.timestamp(ticker)

					if err != nil {
						errnie.Error(err, "leadlag: timestamp %s", ticker.Symbol)
						continue
					}

					stored, _ := signal.symbols.LoadOrStore(ticker.Symbol, newSymbolState())
					stored.(*symbolState).observeTicker(ticker.Last, at)
					ingested = true
				}

				if !ingested {
					continue
				}

				if err := signal.publish(); err != nil {
					errnie.Error(err, "leadlag: publish")
				}
			}
		}
	}
}

// timestamp parses the ticker's wire timestamp.
func (signal *Signal) timestamp(row market.TickerUpdate) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000000Z"} {
		if at, err := time.Parse(layout, row.Timestamp); err == nil {
			return at, nil
		}
	}

	return time.Time{}, fmt.Errorf("ticker timestamp is required for %s", row.Symbol)
}

// publish recomputes lead-lag against the anchor for every follower, throttled.
func (signal *Signal) publish() error {
	if !signal.throttle() {
		return nil
	}

	anchorName := focus.AnchorSymbol()
	anchorRaw, ok := signal.symbols.Load(anchorName)

	if !ok {
		return nil
	}

	anchor := anchorRaw.(*symbolState)
	move := signal.anchorMoveStatus(anchor)

	if !move.ready {
		return nil
	}

	if !move.moved {
		return signal.publishAnchorStall(anchorName, anchor, move.stallMargin)
	}

	return signal.publishFollowers(anchorName, anchor)
}

func (signal *Signal) publishFollowers(anchorName string, anchor *symbolState) error {
	followers := signal.followerScratch[:0]

	signal.symbols.Range(func(key, value any) bool {
		symbol := key.(string)

		if symbol == anchorName {
			return true
		}

		followers = append(followers, symbol)

		return true
	})

	signal.followerScratch = followers
	readings := make([]leadlagGaugeReading, 0, len(followers))
	tasks := make([]chan *qpool.QValue[any], 0, len(followers))

	for _, symbolName := range followers {
		raw, ok := signal.symbols.Load(symbolName)

		if !ok {
			continue
		}

		follower := raw.(*symbolState)

		tasks = append(tasks, signal.pool.ScheduleFast(signal.ctx, func(context.Context) (any, error) {
			measurement, standout, err := signal.measureFollower(anchor, follower)

			if err != nil {
				return nil, fmt.Errorf("leadlag: measure %s: %w", symbolName, err)
			}

			if measurement.Source == types.SourceNone {
				return nil, nil
			}

			measurement.Symbol = symbolName
			measurement.Last = follower.lastPrice()

			if err := signal.sendMeasurement(&measurement, standout); err != nil {
				return nil, fmt.Errorf("leadlag: snr %s: %w", symbolName, err)
			}

			return leadlagGaugeReading{measurement: measurement, standout: standout}, nil
		}))
	}

	var err error

	for _, task := range tasks {
		value := <-task

		if value.Error != nil {
			err = errors.Join(err, value.Error)
			continue
		}

		reading, ok := value.Value.(leadlagGaugeReading)

		if ok {
			readings = append(readings, reading)
		}
	}

	return errors.Join(err, signal.publishMarketGauge(readings))
}

type leadlagGaugeReading struct {
	measurement types.Measurement
	standout    float64
}

func (signal *Signal) publishAnchorStall(
	anchorName string,
	anchor *symbolState,
	stallMargin float64,
) error {
	var joined error
	followers := signal.followerScratch[:0]

	signal.symbols.Range(func(key, value any) bool {
		symbol := key.(string)

		if symbol != anchorName {
			followers = append(followers, symbol)
		}

		return true
	})

	signal.followerScratch = followers
	readings := make([]leadlagGaugeReading, 0, len(followers)+1)
	measurement, standout, err := signal.measureStall(anchor, stallMargin)

	if err != nil {
		return fmt.Errorf("leadlag: measure anchor stall: %w", err)
	}

	measurement.Symbol = anchorName
	measurement.Last = anchor.lastPrice()
	readings = append(readings, leadlagGaugeReading{measurement: measurement, standout: standout})
	joined = errors.Join(joined, signal.sendMeasurement(&measurement, standout))

	for _, symbolName := range followers {
		raw, ok := signal.symbols.Load(symbolName)

		if !ok {
			continue
		}

		follower := raw.(*symbolState)
		measurement, standout, err := signal.measureStall(follower, stallMargin)

		if err != nil {
			joined = errors.Join(joined, fmt.Errorf("leadlag: measure follower stall %s: %w", symbolName, err))
			continue
		}

		measurement.Symbol = symbolName
		measurement.Last = follower.lastPrice()
		readings = append(readings, leadlagGaugeReading{measurement: measurement, standout: standout})
		joined = errors.Join(joined, signal.sendMeasurement(&measurement, standout))
	}

	return errors.Join(joined, signal.publishMarketGauge(readings))
}

func (signal *Signal) publishMarketGauge(readings []leadlagGaugeReading) error {
	aggregate, ok := aggregateLeadlagReadings(readings)

	if !ok {
		return nil
	}

	return signal.sendDashboardGauge(&aggregate.measurement, aggregate.standout)
}

func aggregateLeadlagReadings(
	readings []leadlagGaugeReading,
) (leadlagGaugeReading, bool) {
	if len(readings) == 0 {
		return leadlagGaugeReading{}, false
	}

	best := readings[0]
	confidenceSum := 0.0
	confidenceCount := 0
	snrSum := 0.0
	snrCount := 0
	strengthSum := 0.0
	strengthCount := 0

	for _, reading := range readings {
		measurement := reading.measurement

		if measurement.Confidence > 0 {
			confidenceSum += measurement.Confidence
			confidenceCount++
		}

		if measurement.SNR > 0 {
			snrSum += measurement.SNR
			snrCount++
		}

		if measurement.Strength > 0 {
			strengthSum += measurement.Strength
			strengthCount++
		}

		if measurement.Confidence > best.measurement.Confidence {
			best = reading
		}
	}

	aggregate := best.measurement
	aggregate.Symbol = leadlagMarketSymbol

	if confidenceCount > 0 {
		aggregate.Confidence = confidenceSum / float64(confidenceCount)
	}

	if snrCount > 0 {
		aggregate.SNR = snrSum / float64(snrCount)
	}

	if strengthCount > 0 {
		aggregate.Strength = strengthSum / float64(strengthCount)
	}

	return leadlagGaugeReading{measurement: aggregate, standout: best.standout}, true
}

func (signal *Signal) sendDashboardGauge(
	measurement *types.Measurement,
	standout float64,
) error {
	categoryStandout := standout

	if err := types.AssignCategorySNR(
		measurement, signal.floor, categoryStandout,
	); err != nil {
		return err
	}

	telemetry := numeric.Telemetry{
		Observation: measurement.Strength,
		Edges:       leadlagDefaultBandEdges,
		Labels:      []string{"anchor_stall", "decoupled_move", "synchronized_drift", "inefficient_lag"},
		Calibrated:  true,
		EntropyTrust: 1.0,
	}

	if ui := signal.broadcasts["ui"]; ui != nil {
		ui.Send(&qpool.QValue[any]{
			Value: numeric.GaugePayload(
				measurement.Source.String(),
				measurement.Symbol,
				measurement.Category,
				*measurement,
				telemetry,
			),
		})
	}

	return nil
}

func (signal *Signal) sendMeasurement(
	measurement *types.Measurement,
	standout float64,
) error {
	categoryStandout := standout

	if err := types.AssignCategorySNR(
		measurement, signal.floor, categoryStandout,
	); err != nil {
		return err
	}

	if err := signal.rawDump.Write(rawRecord{
		TimestampUnixNano: time.Now().UTC().UnixNano(),
		Symbol:            measurement.Symbol,
		Category:          measurement.Category,
		Strength:          measurement.Strength,
		Confidence:        measurement.Confidence,
		SNR:               measurement.SNR,
		Standout:          standout,
		Last:              measurement.Last,
		SpreadBPS:         measurement.SpreadBPS,
	}); err != nil {
		return err
	}

	return measurement.Send(signal.pool)
}

// throttle reports whether enough time has passed to recompute; crossLag is
// expensive enough that running it per tick would saturate a core.
func (signal *Signal) throttle() bool {
	signal.lastPublishMu.Lock()
	defer signal.lastPublishMu.Unlock()

	if time.Since(signal.lastPublish) < publishInterval {
		return false
	}

	signal.lastPublish = time.Now()

	return true
}

func (signal *Signal) measureStall(
	symbol *symbolState,
	stallMargin float64,
) (types.Measurement, float64, error) {
	category, confidence, standout := leadlagReading(false, stallMargin, 0, 0)

	if err := symbol.tracked.Observe(category, confidence); err != nil {
		return types.Measurement{}, 0, err
	}

	return types.Measurement{
		Source:     types.SourceLeadLag,
		Category:   category,
		Strength:   stallMargin,
		Confidence: confidence,
	}, standout, nil
}

// measureFollower classifies one follower's lead-lag relationship to the anchor.
func (signal *Signal) measureFollower(
	anchor *symbolState,
	state *symbolState,
) (types.Measurement, float64, error) {
	if bars, corr, ok := state.crossLag(anchor); ok {
		category, confidence, standout := leadlagReading(true, 0, corr, bars)

		if err := state.tracked.Observe(category, confidence); err != nil {
			return types.Measurement{}, 0, err
		}

		return types.Measurement{
			Source:     types.SourceLeadLag,
			Category:   category,
			Strength:   corr / leadlagMinimumLagCorrelation,
			Confidence: confidence,
		}, standout, nil
	}

	corr, ok := state.contemporaneous(anchor)

	if !ok {
		return types.Measurement{}, 0, nil
	}

	category, confidence, standout := leadlagReading(true, 0, corr, 0)

	if err := state.tracked.Observe(category, confidence); err != nil {
		return types.Measurement{}, 0, err
	}

	return types.Measurement{
		Source:     types.SourceLeadLag,
		Category:   category,
		Strength:   corr / leadlagMinimumLagCorrelation,
		Confidence: confidence,
	}, standout, nil
}

func (signal *Signal) Close() error {
	signal.cancel()
	return signal.rawDump.Close()
}
