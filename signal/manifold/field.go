package manifold

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/numeric/physics"
)

/*
Field owns the shared GPU manifold solver and projects the live universe into it.
*/
type Field struct {
	config          physics.Config
	solver          *physics.Solver
	universe        *universe
	lastStepAt      time.Time
	lastReading     physics.Reading
	lastCarriers    []fieldCarrier
	readings        sync.Map
	pendingDeposits []cellDeposit
	pendingWhales   []whaleCarrier
	stepMu          sync.Mutex
}

type whaleCarrier struct {
	symbol     string
	oscillator physics.Oscillator
}

type fieldCarrier struct {
	role       string
	symbol     string
	oscillator physics.Oscillator
}

type cellDeposit struct {
	cellX uint32
	cellY uint32
	cellZ uint32
	rho   float64
	momX  float64
	momY  float64
	momZ  float64
	eInt  float64
}

type symbolReading struct {
	reading physics.Reading
	price   float64
	at      time.Time
}

func newField() (*Field, error) {
	config, configErr := physics.NewConfigFromViper()

	if configErr != nil {
		return nil, configErr
	}

	solver, solverErr := physics.NewSolver(config)

	if solverErr != nil {
		return nil, solverErr
	}

	universe, universeErr := newUniverse(config)

	if universeErr != nil {
		solver.Close()
		return nil, universeErr
	}

	return &Field{
		config:   config,
		solver:   solver,
		universe: universe,
	}, nil
}

func (field *Field) Close() {
	if field == nil || field.solver == nil {
		return
	}

	field.solver.Close()
	field.solver = nil
}

func (field *Field) RegisterSymbols(symbols []string) {
	field.universe.registerSymbols(symbols)
}

func (field *Field) SpotSymbolForIdentity(identity krakenmarket.InstrumentIdentity) string {
	return spotSymbolForBase(field.universe, identity.Base)
}

func (field *Field) FeedTicker(row krakenmarket.TickerUpdate, at time.Time) error {
	state := field.universe.loadSymbol(row.Symbol)

	if state == nil {
		return fmt.Errorf("manifold: symbol %q unavailable", row.Symbol)
	}

	if !at.IsZero() {
		state.lastEventAt = at
	}

	price := row.Last

	if price <= 0 {
		price = (row.Ask + row.Bid) / 2
	}

	if price <= 0 {
		return nil
	}

	field.recordPrice(state, price, at)

	return nil
}

func (field *Field) FeedBook(update krakenmarket.Book, at time.Time) error {
	return field.feedBookIdentity(update, at)
}

func (field *Field) FeedFuturesBook(update krakenmarket.Book, at time.Time) error {
	return field.feedBookIdentity(update, at)
}

func (field *Field) feedBookIdentity(update krakenmarket.Book, at time.Time) error {
	identity := update.InstrumentIdentity()
	state := field.universe.loadIdentity(identity)

	if state == nil {
		return fmt.Errorf("manifold: instrument %q unavailable", identity.Symbol)
	}

	if update.IsSnapshot() {
		state.bookReady = true
	}

	if !state.bookReady {
		return nil
	}

	state.book.Fold(update, state.bookDepth)

	if !at.IsZero() {
		state.lastEventAt = at
	}

	if len(state.book.Bids) == 0 || len(state.book.Asks) == 0 {
		return nil
	}

	midPrice := (state.book.Bids[0].Price + state.book.Asks[0].Price) / 2

	if midPrice <= 0 {
		return fmt.Errorf("manifold: mid price must be positive for %q", update.Symbol)
	}

	state.midPrice = midPrice

	if state.lane == krakenmarket.InstrumentLaneSpot {
		field.recordPrice(state, midPrice, at)
	}

	return field.maybeStep(at)
}

func (field *Field) FeedTrade(trade *krakenmarket.TradeUpdate, at time.Time) error {
	state := field.universe.loadSymbol(trade.Symbol)

	if state == nil {
		return fmt.Errorf("manifold: symbol %q unavailable", trade.Symbol)
	}

	if !at.IsZero() {
		state.lastEventAt = at
	}

	field.recordPrice(state, trade.Price, at)
	state.recordTradeQty(trade.Qty, viperGetCapacity())

	if state.midPrice <= 0 {
		state.midPrice = trade.Price
	}

	offsetTicks := (trade.Price - state.midPrice) / state.tickSize
	coords := field.universe.coords(state, offsetTicks)
	cellVolume := field.config.CellVolume()
	rho := trade.Qty / cellVolume

	if trade.Qty >= state.whaleQtyThreshold() {
		field.pendingWhales = append(field.pendingWhales, whaleCarrier{
			symbol: trade.Symbol,
			oscillator: field.whaleOscillatorFromTrade(
				state,
				trade,
				coords,
				rho,
			),
		})

		return field.maybeStep(at)
	}

	momentum := rho * tradeSideSign(trade.Side)

	field.pendingDeposits = append(field.pendingDeposits, cellDeposit{
		cellX: coords.cellX,
		cellY: coords.cellY,
		cellZ: coords.cellZ,
		rho:   rho,
		momX:  momentum,
		eInt:  rho * field.config.CV,
	})

	return field.maybeStep(at)
}

func (field *Field) Reading(symbol string) (physics.Reading, float64, time.Time, bool) {
	raw, ok := field.readings.Load(symbol)

	if !ok {
		return physics.Reading{}, 0, time.Time{}, false
	}

	row, rowOk := raw.(symbolReading)

	if !rowOk {
		return physics.Reading{}, 0, time.Time{}, false
	}

	return row.reading, row.price, row.at, true
}

func (field *Field) recordPrice(state *UniverseState, price float64, at time.Time) {
	if price <= 0 || at.IsZero() {
		return
	}

	if state.lastPrice <= 0 {
		state.lastPrice = price
		return
	}

	if price == state.lastPrice {
		return
	}

	logReturn := math.Log(price / state.lastPrice)
	state.lastPrice = price
	state.returns = append(state.returns, logReturn)

	capacity := viperGetCapacity()

	if len(state.returns) > capacity {
		state.returns = state.returns[len(state.returns)-capacity:]
	}

	field.universe.recomputeRanks()
}

func (field *Field) maybeStep(at time.Time) error {
	if at.IsZero() {
		return fmt.Errorf("manifold: step event time must be set")
	}

	interval := field.config.IntegrationInterval()

	if !field.lastStepAt.IsZero() && at.Sub(field.lastStepAt) < interval {
		return nil
	}

	return field.integrate(at)
}

func (field *Field) integrate(at time.Time) error {
	field.stepMu.Lock()
	defer field.stepMu.Unlock()

	if err := field.solver.ResetDeposits(); err != nil {
		return errnie.Error(err)
	}

	for _, deposit := range field.pendingDeposits {
		if depositErr := field.solver.DepositCell(
			deposit.cellX,
			deposit.cellY,
			deposit.cellZ,
			deposit.rho,
			deposit.momX,
			deposit.momY,
			deposit.momZ,
			deposit.eInt,
		); depositErr != nil {
			return errnie.Error(depositErr)
		}
	}

	field.pendingDeposits = field.pendingDeposits[:0]

	oscillators := make([]physics.Oscillator, 0)
	carriers := make([]fieldCarrier, 0)

	field.universe.states.Range(func(_, value any) bool {
		state, ok := value.(*UniverseState)

		if !ok || !state.bookReady || state.midPrice <= 0 {
			return true
		}

		if depositErr := field.depositBook(state); depositErr != nil {
			errnie.Error(depositErr)
			return true
		}

		if state.lane == krakenmarket.InstrumentLaneSpot {
			oscillator := field.oscillatorFromState(state)
			oscillators = append(oscillators, oscillator)
			carriers = append(carriers, fieldCarrier{
				role:       "symbol",
				symbol:     state.symbol,
				oscillator: oscillator,
			})
		}

		return true
	})

	if len(oscillators) == 0 && len(field.pendingWhales) == 0 {
		return nil
	}

	for _, whale := range field.pendingWhales {
		oscillators = append(oscillators, whale.oscillator)
		carriers = append(carriers, fieldCarrier{
			role:       "whale",
			symbol:     whale.symbol,
			oscillator: whale.oscillator,
		})
	}

	field.pendingWhales = field.pendingWhales[:0]

	oscillators, carriers = capCarriers(oscillators, carriers, field.config.MaxModes)

	if len(oscillators) == 0 {
		return nil
	}

	if err := field.solver.SetOscillators(oscillators); err != nil {
		return errnie.Error(err)
	}

	reading, stepErr := field.solver.Step()

	if stepErr != nil {
		return errnie.Error(stepErr)
	}

	field.lastReading = reading
	field.lastStepAt = at
	field.lastCarriers = carriers

	field.universe.states.Range(func(_, value any) bool {
		state, ok := value.(*UniverseState)

		if !ok || state.lane != krakenmarket.InstrumentLaneSpot {
			return true
		}

		field.readings.Store(state.symbol, symbolReading{
			reading: reading,
			price:   state.lastPrice,
			at:      at,
		})

		return true
	})

	return nil
}

func (field *Field) depositBook(state *UniverseState) error {
	bids := truncateLevels(state.book.Bids, state.bookDepth)
	asks := truncateLevels(state.book.Asks, state.bookDepth)
	midPrice := state.midPrice
	cellVolume := field.config.CellVolume()

	depositSide := func(levels []krakenmarket.BookLevel, sign float64) error {
		for _, level := range levels {
			offsetTicks := (level.Price - midPrice) / state.tickSize
			coords := field.universe.coords(state, offsetTicks)
			rho := level.Qty / cellVolume

			if depositErr := field.solver.DepositCell(
				coords.cellX,
				coords.cellY,
				coords.cellZ,
				rho,
				sign*rho,
				0,
				0,
				rho*field.config.CV,
			); depositErr != nil {
				return depositErr
			}
		}

		return nil
	}

	if err := depositSide(bids, -1); err != nil {
		return err
	}

	return depositSide(asks, 1)
}

func (field *Field) whaleOscillatorFromTrade(
	state *UniverseState,
	trade *krakenmarket.TradeUpdate,
	coords Coords,
	rho float64,
) physics.Oscillator {
	omega := returnFrequency(state.returns, field.config.DeltaT)
	speed := math.Sqrt(math.Max(rho, field.config.RhoMin))
	phase := 0.0

	if trade.Side == "sell" {
		phase = math.Pi
	}

	return physics.Oscillator{
		Phase:     phase,
		Omega:     omega,
		Amplitude: speed,
		PosX:      coords.posX,
		PosY:      coords.posY,
		PosZ:      coords.posZ,
		Heat:      trade.Qty * trade.Price * field.config.RhoMin,
		VelX:      tradeSideSign(trade.Side) * speed,
	}
}

func (field *Field) oscillatorFromState(state *UniverseState) physics.Oscillator {
	energy := medianAbsolute(state.returns)
	omega := returnFrequency(state.returns, field.config.DeltaT)
	coords := field.universe.coords(state, 0)

	return physics.Oscillator{
		Phase:     math.Mod(omega*field.config.DeltaT, 2*math.Pi),
		Omega:     omega,
		Amplitude: math.Sqrt(math.Max(energy, field.config.RhoMin)),
		PosX:      coords.posX,
		PosY:      coords.posY,
		PosZ:      coords.posZ,
		Heat:      energy,
	}
}

func returnFrequency(returns []float64, deltaT float64) float64 {
	if len(returns) < 2 || deltaT <= 0 {
		return 2 * math.Pi / deltaT
	}

	mean := 0.0

	for _, value := range returns {
		mean += value
	}

	mean /= float64(len(returns))

	variance := 0.0

	for _, value := range returns {
		delta := value - mean
		variance += delta * delta
	}

	variance /= float64(len(returns) - 1)

	if variance <= 0 {
		return 2 * math.Pi / deltaT
	}

	return math.Sqrt(variance) / deltaT
}

func tradeSideSign(side string) float64 {
	if side == "sell" {
		return -1
	}

	return 1
}

func truncateLevels(levels []krakenmarket.BookLevel, depth int) []krakenmarket.BookLevel {
	if depth <= 0 || len(levels) <= depth {
		return levels
	}

	return levels[:depth]
}

func capCarriers(
	oscillators []physics.Oscillator,
	carriers []fieldCarrier,
	maxCount uint32,
) ([]physics.Oscillator, []fieldCarrier) {
	limit := int(maxCount)

	if limit <= 0 || len(oscillators) <= limit {
		return oscillators, carriers
	}

	indices := make([]int, len(oscillators))

	for index := range oscillators {
		indices[index] = index
	}

	sort.Slice(indices, func(leftIndex, rightIndex int) bool {
		leftHeat := oscillators[indices[leftIndex]].Heat
		rightHeat := oscillators[indices[rightIndex]].Heat

		return leftHeat > rightHeat
	})

	trimmedOscillators := make([]physics.Oscillator, limit)
	trimmedCarriers := make([]fieldCarrier, limit)

	for rank := 0; rank < limit; rank++ {
		sourceIndex := indices[rank]
		trimmedOscillators[rank] = oscillators[sourceIndex]
		trimmedCarriers[rank] = carriers[sourceIndex]
	}

	return trimmedOscillators, trimmedCarriers
}

func viperGetCapacity() int {
	capacity := viper.GetInt("signals.manifold.measurements_capacity")

	if capacity <= 0 {
		capacity = viper.GetInt("signals.correlation.measurements_capacity")
	}

	if capacity <= 0 {
		return 64
	}

	return capacity
}
