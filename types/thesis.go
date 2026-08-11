package types

import (
	"context"
	"slices"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/physics/fluid"
	"github.com/theapemachine/symm/kraken"
)

/*
Thesis owns canonical evidence across every evaluated epoch that contributes to
one decision. It closes only after the planner emits the completed decision set;
broker execution and settlement continue in their own lifecycle.
*/
type Thesis struct {
	ctx          context.Context
	cancel       context.CancelFunc
	subscribers  *sync.Map
	statuses     *sync.Map
	ui           chan []byte
	readinessRev atomic.Uint64
	equityMu     sync.RWMutex
	equity       *kraken.TradeBalanceResult
	Status       Status          `json:"status"`
	Tick         int64           `json:"tick"`
	At           time.Time       `json:"at"`
	LastTickerAt time.Time       `json:"lastTickerAt"`
	LastTradeAt  time.Time       `json:"lastTradeAt"`
	CrossSection *CrossSection   `json:"crossSection"`
	Measurements *sync.Map       `json:"-"`
	Symbols      *sync.Map       `json:"-"`
	Measured     *sync.Map       `json:"-"`
	Tickers      *sync.Map       `json:"-"`
	Trades       *sync.Map       `json:"-"`
	Audit        func(any) error `json:"-"`
	Manifold     fluid.Reading   `json:"-"`
}

/*
NewThesis creates a Thesis with empty durable maps and no tick evidence yet.
*/
func NewThesis(
	ctx context.Context, ui chan []byte,
) *Thesis {
	ctx, cancel := context.WithCancel(ctx)

	return &Thesis{
		ctx:          ctx,
		cancel:       cancel,
		subscribers:  &sync.Map{},
		statuses:     &sync.Map{},
		ui:           ui,
		Status:       READY,
		At:           time.Now().UTC(),
		CrossSection: NewCrossSection(),
		Measurements: &sync.Map{},
		Symbols:      &sync.Map{},
		Measured:     &sync.Map{},
		Tickers:      &sync.Map{},
		Trades:       &sync.Map{},
		Manifold:     fluid.Reading{},
	}
}

func (thesis *Thesis) symbol(symbolName string) *Symbol {
	value, _ := thesis.Symbols.LoadOrStore(symbolName, NewSymbol(symbolName, thesis.ui))

	return value.(*Symbol)
}

/*
Reset clears completed symbol evaluations. With no symbols it clears the full
market state.
*/
func (thesis *Thesis) Reset(symbols ...string) *Thesis {
	if len(symbols) > 0 {
		for _, symbolName := range symbols {
			value, found := thesis.Symbols.Load(symbolName)

			if found {
				symbol, ok := value.(*Symbol)

				if ok && symbol != nil {
					symbol.Reset()
				}
			}

			thesis.Symbols.Delete(symbolName)
			thesis.Tickers.Delete(symbolName)
			thesis.Trades.Delete(symbolName)
		}

		thesis.At = time.Now().UTC()
		return thesis
	}

	thesis.Symbols.Range(func(_, value any) bool {
		symbol := value.(*Symbol)
		symbol.Reset()
		symbol.ResetMarket()
		return true
	})
	thesis.At = time.Now().UTC()
	thesis.CrossSection = NewCrossSection()
	thesis.Tickers.Clear()
	thesis.Trades.Clear()
	thesis.Manifold = fluid.Reading{}
	return thesis
}

func (thesis *Thesis) AppendTicker(ticker kraken.TickerData) *Thesis {
	if ticker.Symbol == "" {
		return thesis
	}

	thesis.symbol(ticker.Symbol).AppendTicker(ticker)

	if ticker.Timestamp.After(thesis.LastTickerAt) {
		thesis.LastTickerAt = ticker.Timestamp
	}

	return thesis
}

func (thesis *Thesis) AppendTrade(trade kraken.TradeData) *Thesis {
	if trade.Symbol == "" {
		return thesis
	}

	thesis.symbol(trade.Symbol).AppendTrade(trade)

	if trade.Timestamp.After(thesis.LastTradeAt) {
		thesis.LastTradeAt = trade.Timestamp
	}

	return thesis
}

/*
AppendEquity retains the latest complete account valuation and wakes only the
global regulator. Account feedback must not start another market-analysis pass.
*/
func (thesis *Thesis) AppendEquity(equity kraken.TradeBalanceResult) error {
	if equity.Equity == nil || equity.Equity.Sign() <= 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"thesis: positive account equity required",
			nil,
		))
	}

	thesis.equityMu.Lock()
	thesis.equity = &equity
	thesis.equityMu.Unlock()

	return nil
}

/*
Equity returns the latest complete account valuation received from the broker.
*/
func (thesis *Thesis) Equity() (kraken.TradeBalanceResult, bool) {
	thesis.equityMu.RLock()
	defer thesis.equityMu.RUnlock()

	if thesis.equity == nil {
		return kraken.TradeBalanceResult{}, false
	}

	return *thesis.equity, true
}

/*
MarshalState captures the complete durable thesis cut before execution or reset.
*/
func (thesis *Thesis) MarshalState() ([]byte, error) {
	if thesis == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"thesis: state required for checkpoint",
			nil,
		))
	}

	equity, hasEquity := thesis.Equity()
	var equityState *kraken.TradeBalanceResult

	if hasEquity {
		equityState = &equity
	}

	return sonic.Marshal(struct {
		Status       Status                     `json:"status"`
		Tick         int64                      `json:"tick"`
		At           time.Time                  `json:"at"`
		LastTickerAt time.Time                  `json:"lastTickerAt"`
		LastTradeAt  time.Time                  `json:"lastTradeAt"`
		CrossSection *CrossSection              `json:"crossSection"`
		Measurements map[string]any             `json:"measurements"`
		Symbols      map[string]any             `json:"symbols"`
		Measured     map[string]any             `json:"measured"`
		Tickers      map[string]any             `json:"tickers"`
		Trades       map[string]any             `json:"trades"`
		Equity       *kraken.TradeBalanceResult `json:"equity,omitempty"`
		Manifold     fluid.Reading              `json:"manifold"`
	}{
		Status:       thesis.Status,
		Tick:         thesis.Tick,
		At:           thesis.At,
		LastTickerAt: thesis.LastTickerAt,
		LastTradeAt:  thesis.LastTradeAt,
		CrossSection: thesis.CrossSection,
		Measurements: syncMapState(thesis.Measurements),
		Symbols:      syncMapState(thesis.Symbols),
		Measured:     syncMapState(thesis.Measured),
		Tickers:      thesis.TickersState(),
		Trades:       thesis.TradesState(),
		Equity:       equityState,
		Manifold:     thesis.Manifold,
	})
}

func (thesis *Thesis) AppendMeasurements(
	sender SourceType,
	measurements []*Measurement,
	_ bool,
) error {
	if len(measurements) > 0 {
		found, _ := thesis.Measurements.LoadOrStore(sender, measurements)

		stored, ok := found.([]*Measurement)

		if !ok {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"thesis: invalid measurement type for source "+string(sender),
				nil,
			))
		}

		combined := append(slices.Clone(stored), measurements...)
		deduped := make([]*Measurement, 0, len(combined))
		seen := make(map[string]bool)

		for _, m := range slices.Backward(combined) {
			key := m.Symbol + "|" + m.Peer
			if !seen[key] {
				seen[key] = true
				deduped = append(deduped, m)
			}
		}

		for i, j := 0, len(deduped)-1; i < j; i, j = i+1, j-1 {
			deduped[i], deduped[j] = deduped[j], deduped[i]
		}

		thesis.Measurements.Store(sender, deduped)

		for _, measurement := range deduped {
			if measurement == nil || measurement.Symbol == "" {
				return errnie.Error(errnie.Err(
					errnie.Validation,
					"thesis: identified symbol measurement required for source "+string(sender),
					nil,
				))
			}

			symbolValue, _ := thesis.Symbols.LoadOrStore(
				measurement.Symbol,
				NewSymbol(measurement.Symbol, thesis.ui),
			)
			symbolValue.(*Symbol).AddMeasurement(measurement)
		}
	}

	return nil
}

/*
MarketTickers returns each symbol's ticker rows not yet seen by source.
*/
func (thesis *Thesis) MarketTickers(source SourceType) []kraken.TickerData {
	out := make([]kraken.TickerData, 0)

	thesis.Tickers.Range(func(key, value any) bool {
		symbolName := key.(string)

		for _, ticker := range value.([]kraken.TickerData) {
			thesis.symbol(symbolName).AppendTicker(ticker)
		}

		thesis.Tickers.Delete(symbolName)
		return true
	})

	thesis.Symbols.Range(func(key, value any) bool {
		symbol := value.(*Symbol)
		out = append(out, symbol.MarketTickers(source)...)

		return true
	})

	sort.SliceStable(out, func(left, right int) bool {
		if out[left].Timestamp.Equal(out[right].Timestamp) {
			return out[left].Symbol < out[right].Symbol
		}

		return out[left].Timestamp.Before(out[right].Timestamp)
	})

	return out
}

func (thesis *Thesis) MarketTrades(source SourceType) []kraken.TradeData {
	out := make([]kraken.TradeData, 0)

	thesis.Trades.Range(func(key, value any) bool {
		symbolName := key.(string)

		for _, trade := range value.([]kraken.TradeData) {
			thesis.symbol(symbolName).AppendTrade(trade)
		}

		thesis.Trades.Delete(symbolName)
		return true
	})

	thesis.Symbols.Range(func(key, value any) bool {
		symbol := value.(*Symbol)
		out = append(out, symbol.MarketTrades(source)...)

		return true
	})

	sort.SliceStable(out, func(left, right int) bool {
		if out[left].Timestamp.Equal(out[right].Timestamp) {
			if out[left].Symbol == out[right].Symbol {
				return out[left].TradeID < out[right].TradeID
			}

			return out[left].Symbol < out[right].Symbol
		}

		return out[left].Timestamp.Before(out[right].Timestamp)
	})

	return out
}

func (thesis *Thesis) TickersState() map[string]any {
	state := make(map[string]any)

	thesis.Symbols.Range(func(key, value any) bool {
		rows := value.(*Symbol).TickersSnapshot()

		if len(rows) > 0 {
			state[key.(string)] = rows
		}

		return true
	})

	return state
}

func (thesis *Thesis) TradesState() map[string]any {
	state := make(map[string]any)

	thesis.Symbols.Range(func(key, value any) bool {
		rows := value.(*Symbol).TradesSnapshot()

		if len(rows) > 0 {
			state[key.(string)] = rows
		}

		return true
	})

	return state
}

func (thesis *Thesis) LatestTicker(symbolName string) (kraken.TickerData, bool) {
	value, found := thesis.Symbols.Load(symbolName)

	if found {
		return value.(*Symbol).LatestTicker()
	}

	stored, legacyFound := thesis.Tickers.Load(symbolName)

	if !legacyFound {
		return kraken.TickerData{}, false
	}

	rows := stored.([]kraken.TickerData)

	if len(rows) == 0 {
		return kraken.TickerData{}, false
	}

	return rows[len(rows)-1], true
}

func (thesis *Thesis) TradesSnapshot(symbolName string) []kraken.TradeData {
	value, found := thesis.Symbols.Load(symbolName)

	if found {
		return value.(*Symbol).TradesSnapshot()
	}

	stored, legacyFound := thesis.Trades.Load(symbolName)

	if !legacyFound {
		return nil
	}

	return slices.Clone(stored.([]kraken.TradeData))
}

func (thesis *Thesis) TradeSymbols() []string {
	symbols := make([]string, 0)

	thesis.Symbols.Range(func(key, value any) bool {
		if len(value.(*Symbol).TradesSnapshot()) > 0 {
			symbols = append(symbols, key.(string))
		}

		return true
	})

	thesis.Trades.Range(func(key, value any) bool {
		symbolName := key.(string)
		rows := value.([]kraken.TradeData)
		_, found := thesis.Symbols.Load(symbolName)

		if !found && len(rows) > 0 {
			symbols = append(symbols, symbolName)
		}

		return true
	})

	sort.Strings(symbols)

	return symbols
}

func (thesis *Thesis) Close() error {
	if thesis.cancel != nil {
		thesis.cancel()
	}

	return nil
}
