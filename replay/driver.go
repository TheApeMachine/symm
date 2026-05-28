/*
Package replay drives the system off a captured JSONL fixture so the
prediction → calibration → trade loop can be validated against known
inputs without touching the live Kraken WebSocket. The driver behaves as
a drop-in substitute for kraken/client.PublicClient: it broadcasts
instrument snapshots, ticker rows, trade events, and book deltas onto
the same qpool channels the signal layer is already subscribed to.

Time semantics. Every event carries an exchange timestamp; the driver
yields events in event-time order and applies a configurable speedup
between successive timestamps so a single second of historical data can
be replayed in ten milliseconds when only the pipeline determinism is
being checked. ReplaySpeed = 0 emits all events as fast as the consumer
can drain them; the price.Prediction settleDue path takes its event-time
clock from the ticker timestamp itself, so settlement labels remain
reproducible regardless of wall clock.
*/
package replay

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/core"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trade"
)

type rawEnvelope struct {
	Channel string          `json:"channel"`
	Type    string          `json:"type"`
	Data    json.RawMessage `json:"data"`
}

type rawTrade struct {
	Symbol    string  `json:"symbol"`
	Side      string  `json:"side"`
	Qty       float64 `json:"qty"`
	Price     float64 `json:"price"`
	Timestamp string  `json:"timestamp"`
}

type rawInstrumentSnapshot struct {
	Pairs []struct {
		Symbol string `json:"symbol"`
		Base   string `json:"base"`
		Quote  string `json:"quote"`
		Status string `json:"status"`
	} `json:"pairs"`
}

/*
event is one decoded fixture row tagged with the event-time used to
order playback. skip is set when the row is decodable but intentionally
dropped (e.g. trade frame with all rows filtered out). payload is one
of: instrument map, []market.TickerRow, []trade.Data, or
market.BookLevelsDelta.
*/
type event struct {
	at      time.Time
	channel string
	payload any
	skip    bool
}

/*
Driver replays a JSONL fixture into the qpool broadcast groups the live
public client would otherwise feed. It is a drop-in System (Start /
State / Tick / Close) so the booter can swap it in by environment
variable.
*/
type Driver struct {
	ctx        context.Context
	cancel     context.CancelFunc
	pool       *qpool.Q
	broadcasts map[string]*qpool.BroadcastGroup
	path       string
	speed      float64
	events     []event
	once       sync.Once
	loadErr    error
	state      atomic.Value // engine.State
}

/*
NewDriver constructs a replay driver bound to parent context. speed is the
factor applied to inter-event delays: 1.0 plays back at real time, 10 is
ten times faster, 0 emits as fast as possible.
*/
func NewDriver(ctx context.Context, pool *qpool.Q, path string, speed float64) *Driver {
	ctx, cancel := context.WithCancel(ctx)

	driver := &Driver{
		ctx:        ctx,
		cancel:     cancel,
		pool:       pool,
		path:       path,
		speed:      speed,
		broadcasts: make(map[string]*qpool.BroadcastGroup),
	}

	// The engine.State enum only differentiates READY vs BUSY, so the
	// driver tracks just those two: READY when idle (pre-Tick, post-Tick,
	// or after a fixture-level error has been surfaced through Start),
	// BUSY for the duration of Tick. Richer state (LOADING / DONE / ERROR)
	// would require widening engine.State and updating every existing
	// system's State() — deliberately out of scope.
	driver.state.Store(engine.READY)

	for _, channel := range []string{"tick", "trade", "book", "symbols", "subscriptions", "ui"} {
		driver.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
	}

	return driver
}

func (driver *Driver) Start() error {
	return driver.load()
}

func (driver *Driver) State() engine.State {
	if value := driver.state.Load(); value != nil {
		return value.(engine.State)
	}

	return engine.READY
}

func (driver *Driver) Close() error {
	driver.cancel()
	return nil
}

/*
Tick blocks the booter on replay completion. Each event is dispatched in
event-time order; the driver returns nil when the fixture is exhausted
or the parent context is cancelled.
*/
func (driver *Driver) Tick() error {
	if len(driver.events) == 0 {
		if err := driver.load(); err != nil {
			return err
		}
	}

	driver.state.Store(engine.BUSY)
	defer driver.state.Store(engine.READY)

	previous := time.Time{}

	for _, evt := range driver.events {
		if driver.ctx.Err() != nil {
			return driver.ctx.Err()
		}

		if !previous.IsZero() && driver.speed > 0 {
			elapsed := evt.at.Sub(previous)
			scaled := time.Duration(float64(elapsed) / driver.speed)

			if scaled > 0 {
				timer := time.NewTimer(scaled)

				select {
				case <-driver.ctx.Done():
					timer.Stop()
					return driver.ctx.Err()
				case <-timer.C:
				}
			}
		}

		driver.dispatch(evt)
		previous = evt.at
	}

	return nil
}

func (driver *Driver) dispatch(evt event) {
	switch payload := evt.payload.(type) {
	case map[string]*asset.Pair:
		driver.broadcasts["symbols"].Send(&qpool.QValue[any]{Value: payload})
	case []market.TickerRow:
		for _, row := range payload {
			driver.broadcasts["tick"].Send(&qpool.QValue[any]{Value: row})
		}
	case []trade.Data:
		for _, row := range payload {
			driver.broadcasts["trade"].Send(&qpool.QValue[any]{Value: row})
		}
	case market.BookLevelsDelta:
		driver.broadcasts["book"].Send(&qpool.QValue[any]{Value: payload})
	default:
		errnie.Error(fmt.Errorf("replay: unknown payload type %T for channel %s", payload, evt.channel))
	}
}

func (driver *Driver) load() error {
	// once.Do captures the error into driver.loadErr so a second call after
	// a first failure still returns the original error. The previous shape
	// stored the error only in a closure-local variable and once.Do
		// suppressed subsequent attempts, so retries silently returned nil.
	driver.once.Do(func() {
		driver.loadErr = driver.loadOnce()
	})

	return driver.loadErr
}

func (driver *Driver) loadOnce() error {
	if driver.path == "" {
		return fmt.Errorf("replay path is empty")
	}

	file, err := os.Open(driver.path)

	if err != nil {
		return fmt.Errorf("open replay file: %w", err)
	}

	defer file.Close()

	events := make([]event, 0, 1024)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1<<20), 1<<24)

	// lastEventTime anchors timeless frames (instrument snapshots, book
	// frames Kraken doesn't stamp) to the most recent stamped event so
	// event-time ordering is preserved without falling back to wall clock.
	// Empty fixture or leading timeless frames default to the Unix epoch
	// so they sort before the first stamped event.
	var (
		lastEventTime time.Time
		lineNum       int
	)

	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()

		if len(line) == 0 {
			continue
		}

		var envelope rawEnvelope

		if err := json.Unmarshal(line, &envelope); err != nil {
			return fmt.Errorf("decode replay line %d: %w", lineNum, err)
		}

		evt, err := driver.decodeEnvelope(envelope, line, lastEventTime)

		if err != nil {
			return fmt.Errorf("decode replay line %d (%s): %w", lineNum, envelope.Channel, err)
		}

		if evt.skip {
			continue
		}

		lastEventTime = evt.at
		events = append(events, evt)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan replay file: %w", err)
	}

	sort.SliceStable(events, func(left, right int) bool {
		return events[left].at.Before(events[right].at)
	})

	driver.events = events

	return nil
}

func (driver *Driver) decodeEnvelope(envelope rawEnvelope, raw []byte, lastEventTime time.Time) (event, error) {
	switch envelope.Channel {
	case core.ChannelTicker:
		rows, err := market.ParseTickerRows(raw)

		if err != nil {
			return event{}, fmt.Errorf("parse ticker: %w", err)
		}

		if len(rows) == 0 {
			return event{skip: true}, nil
		}

		if rows[0].Timestamp == "" {
			return event{}, fmt.Errorf("ticker row 0 missing timestamp")
		}

		eventTime, err := time.Parse(time.RFC3339Nano, rows[0].Timestamp)

		if err != nil {
			return event{}, fmt.Errorf("parse ticker timestamp %q: %w", rows[0].Timestamp, err)
		}

		return event{at: eventTime, channel: "tick", payload: rows}, nil
	case core.ChannelTrades:
		var rows []rawTrade

		if err := json.Unmarshal(envelope.Data, &rows); err != nil {
			return event{}, fmt.Errorf("unmarshal trades: %w", err)
		}

		if len(rows) == 0 {
			return event{skip: true}, nil
		}

		trades := make([]trade.Data, 0, len(rows))
		var eventTime time.Time

		for index, row := range rows {
			if row.Symbol == "" || row.Price <= 0 {
				continue
			}

			if row.Timestamp == "" {
				return event{}, fmt.Errorf("trade row %d missing timestamp", index)
			}

			ts, err := time.Parse(time.RFC3339Nano, row.Timestamp)

			if err != nil {
				return event{}, fmt.Errorf("parse trade row %d timestamp %q: %w", index, row.Timestamp, err)
			}

			if eventTime.IsZero() {
				eventTime = ts
			}

			trades = append(trades, trade.Data{
				Symbol:    row.Symbol,
				Side:      row.Side,
				Qty:       row.Qty,
				Price:     row.Price,
				Timestamp: ts,
			})
		}

		if len(trades) == 0 {
			return event{skip: true}, nil
		}

		return event{at: eventTime, channel: "trade", payload: trades}, nil
	case "instrument":
		var snap rawInstrumentSnapshot

		if err := json.Unmarshal(envelope.Data, &snap); err != nil {
			return event{}, fmt.Errorf("unmarshal instrument snapshot: %w", err)
		}

		pairs := make(map[string]*asset.Pair, len(snap.Pairs))

		for _, pair := range snap.Pairs {
			if pair.Symbol == "" {
				continue
			}

			pairs[pair.Symbol] = &asset.Pair{
				Wsname: pair.Symbol,
				Base:   pair.Base,
				Quote:  pair.Quote,
			}
		}

		// instrument and book frames are not individually timestamped by
		// Kraken. Anchoring to lastEventTime preserves event-time ordering
		// of the stamped frames and keeps the relative order of timeless
		// frames within their own group.
		return event{at: lastEventTime, channel: "symbols", payload: pairs}, nil
	case core.ChannelBook:
		delta, err := market.ParseBookLevelsDelta(raw)

		if err != nil {
			return event{}, fmt.Errorf("parse book: %w", err)
		}

		return event{at: lastEventTime, channel: "book", payload: delta}, nil
	}

	return event{skip: true}, nil
}
