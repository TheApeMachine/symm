package tests

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"sync"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/tests/signal"
	testtypes "github.com/theapemachine/symm/tests/types"
)

/*
Market drives real production websocket consumers with one coherent, replayable
simulated venue timeline.
*/
type Market struct {
	ctx        context.Context
	cancel     context.CancelFunc
	Public     *Conn
	Private    *Conn
	Level3     *Conn
	Symbols    []*testtypes.Symbol
	State      testtypes.MarketState
	Config     testtypes.ScenarioConfig
	public     *websocket.Live
	private    *websocket.Live
	generators map[string]*signal.Generator
	latest     map[string]testtypes.Sample
	history    map[string][]testtypes.Sample
	previous   map[string]testtypes.Sample
	states     map[string]testtypes.MarketState
	candles    map[string]*candleState
	execution  *executionModel
	stack      Stack
	autoFill   bool
	primed     bool
	clockSet   bool
	clockAt    time.Time
	sampleAt   time.Time
	tick       uint64
	factorRNG  *rand.Rand
	factors    []float64
	timeline   []RegimeObservation
	exposure   map[string]map[testtypes.MarketState]uint64
	published  map[string]bool
	sampleMu   sync.RWMutex
}

/*
Drive attaches the production stack whose progress controls market pacing.
*/
func (market *Market) Drive(stack Stack) *Market {
	market.stack = stack

	return market
}

/*
NewMarket creates a deterministic default mechanics scenario.
*/
func NewMarket(
	ctx context.Context,
	symbols []*testtypes.Symbol,
) *Market {
	config := testtypes.NewScenarioConfig(symbols)

	if err := config.Validate(); err != nil {
		panic(err)
	}

	return newMarket(ctx, config, nil)
}

/*
NewMarketWithScenario builds a market from a complete validated replay identity.
*/
func NewMarketWithScenario(
	ctx context.Context,
	config testtypes.ScenarioConfig,
) (*Market, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	config = config.Clone()

	return newMarket(ctx, config, func(private *Conn) {
		if len(config.InitialBalances) == 0 {
			return
		}

		balances := make(map[string]string, len(config.InitialBalances))

		for asset, balance := range config.InitialBalances {
			balances[asset] = strconv.FormatFloat(balance, 'f', -1, 64)
		}

		private.ConfigureAccount(balances, nil)
	}), nil
}

/*
NewMarketWithAccount starts from explicit exchange inventory and fill history.
*/
func NewMarketWithAccount(
	ctx context.Context,
	symbols []*testtypes.Symbol,
	balances map[string]string,
	trades map[string]spot.Trade,
) *Market {
	config := testtypes.NewScenarioConfig(symbols)

	if err := config.Validate(); err != nil {
		panic(err)
	}

	return newMarket(ctx, config, func(private *Conn) {
		private.ConfigureAccount(balances, trades)
	})
}

func newMarket(
	ctx context.Context,
	config testtypes.ScenarioConfig,
	configureAccount func(*Conn),
) *Market {
	config = config.Clone()
	tradingModel := viper.GetString("trading.model")
	viper.Set("trading.model", "real")
	defer viper.Set("trading.model", tradingModel)
	ctx, cancel := context.WithCancel(ctx)
	publicConn := NewConn(ctx)
	privateConn := NewConn(ctx)
	level3Conn := NewConn(ctx)

	for _, conn := range []*Conn{publicConn, privateConn, level3Conn} {
		if err := conn.ConfigureTime(config.StartTime); err != nil {
			panic(err)
		}
	}

	publicConn.ConfigureFaults(config.Faults)
	privateConn.ConfigureFaults(config.Faults)
	level3Conn.ConfigureFaults(config.Faults)

	market := &Market{
		ctx:        ctx,
		cancel:     cancel,
		Public:     publicConn,
		Private:    privateConn,
		Level3:     level3Conn,
		Symbols:    config.Symbols,
		State:      testtypes.Baseline,
		Config:     config,
		generators: make(map[string]*signal.Generator, len(config.Symbols)),
		latest:     make(map[string]testtypes.Sample, len(config.Symbols)),
		history:    make(map[string][]testtypes.Sample, len(config.Symbols)),
		previous:   make(map[string]testtypes.Sample, len(config.Symbols)),
		states:     make(map[string]testtypes.MarketState, len(config.Symbols)),
		candles:    make(map[string]*candleState, len(config.Symbols)),
		factorRNG:  rand.New(rand.NewSource(config.Seed)),
		exposure:   make(map[string]map[testtypes.MarketState]uint64, len(config.Symbols)),
		published:  make(map[string]bool, len(config.Symbols)),
	}

	for _, symbol := range config.Symbols {
		generator := signal.NewGeneratorFromSymbol(symbol)

		if err := generator.ConfigureProfiles(config.Profiles); err != nil {
			panic(err)
		}

		if err := generator.SetTime(config.StartTime); err != nil {
			panic(err)
		}

		market.generators[symbol.Pair] = generator
		market.states[symbol.Pair] = testtypes.Baseline
		market.exposure[symbol.Pair] = map[testtypes.MarketState]uint64{}
	}

	market.Public.Configure(config.Symbols)
	market.Private.Configure(config.Symbols)
	market.Level3.Configure(config.Symbols)

	if configureAccount != nil {
		configureAccount(market.Private)
	}

	market.public = websocket.NewWithClient(
		ctx, nil, false,
		websocket.PublicWebSocketURL, market.Public.Client(),
	)
	market.private = websocket.NewWithClient(
		ctx, nil, true,
		websocket.PrivateWebSocketURL, market.Private.Client(),
	)

	return market
}

/*
Transition moves one symbol through its observable precursor to a latent state.
*/
func (market *Market) Transition(
	symbol string,
	state testtypes.MarketState,
) error {
	generator, ok := market.generators[symbol]

	if !ok {
		return fmt.Errorf("market: cannot transition unknown symbol %q", symbol)
	}

	if _, known := market.Config.Profiles[state]; !known {
		return fmt.Errorf("market: cannot transition %s to unknown state %d", symbol, state)
	}

	if !market.primed && market.stack != nil {
		baseline := market.Config.Profiles[testtypes.Baseline]

		for range baseline.Precursor.MinimumObservations {
			market.Tick()
		}

		market.primed = true
	}

	market.State = state
	market.states[symbol] = state
	market.timeline = append(market.timeline, RegimeObservation{
		Tick: market.tick, Symbol: symbol, State: state,
	})
	generator.SetState(state, market.Config.Momentum[state])

	for generator.PrecursorPending() {
		market.Tick()
	}

	return nil
}

/*
TransitionAll arms every declared symbol before advancing their shared timeline.
*/
func (market *Market) TransitionAll(
	states map[string]testtypes.MarketState,
) error {
	for symbol, state := range states {
		if _, known := market.generators[symbol]; !known {
			return fmt.Errorf("market: cannot transition unknown symbol %q", symbol)
		}

		if _, known := market.Config.Profiles[state]; !known {
			return fmt.Errorf("market: cannot transition %s to unknown state %d", symbol, state)
		}
	}

	if !market.primed && market.stack != nil {
		baseline := market.Config.Profiles[testtypes.Baseline]

		for range baseline.Precursor.MinimumObservations {
			market.Tick()
		}

		market.primed = true
	}

	for symbol, state := range states {
		market.generators[symbol].SetState(state, market.Config.Momentum[state])
		market.states[symbol] = state
		market.timeline = append(market.timeline, RegimeObservation{
			Tick: market.tick, Symbol: symbol, State: state,
		})
	}

	for {
		pending := false

		for symbol := range states {
			if market.generators[symbol].PrecursorPending() {
				pending = true
				break
			}
		}

		if !pending {
			return nil
		}

		market.Tick()
	}
}

/*
LastSample returns the latest coherent observation for one symbol.
*/
func (market *Market) LastSample(symbol string) (testtypes.Sample, bool) {
	market.sampleMu.RLock()
	defer market.sampleMu.RUnlock()
	sample, known := market.latest[symbol]

	return sample, known
}

/*
Samples returns an immutable copy of the venue observations published for one
symbol. It lets integration audits inspect the fixture without competing with
production analytical queues.
*/
func (market *Market) Samples(symbol string) []testtypes.Sample {
	market.sampleMu.RLock()
	defer market.sampleMu.RUnlock()

	return append([]testtypes.Sample(nil), market.history[symbol]...)
}

/*
Tick advances every symbol once on one shared seeded factor timeline.
*/
func (market *Market) Tick() {
	market.applySchedule()
	marketShock := market.factorRNG.NormFloat64()
	market.factors = append(market.factors, marketShock)

	for _, symbol := range market.Symbols {
		market.exposure[symbol.Pair][market.states[symbol.Pair]]++
		market.publish(market.generators[symbol.Pair], market.factor(symbol))
	}

	market.tick++
}

func (market *Market) applySchedule() {
	for _, transition := range market.Config.Schedule {
		if transition.Tick != market.tick {
			continue
		}

		market.State = transition.State
		market.states[transition.Symbol] = transition.State
		market.timeline = append(market.timeline, RegimeObservation{
			Tick: market.tick, Symbol: transition.Symbol, State: transition.State,
		})
		market.generators[transition.Symbol].SetState(
			transition.State,
			market.Config.Momentum[transition.State],
		)
	}
}

func (market *Market) factor(symbol *testtypes.Symbol) float64 {
	index := len(market.factors) - 1 - symbol.FactorLagTicks

	if index < 0 {
		return 0
	}

	return market.factors[index]
}
