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
All mutations run on the manifold System tick goroutine; readings publish through sync.Map.
*/
type Field struct {
	config              physics.Config
	solver              *physics.Solver
	universe            *universe
	lastStepAt          time.Time
	lastReading         physics.Reading
	lastCarriers        []fieldCarrier
	readings            sync.Map
	pendingDeposits     []cellDeposit
	pendingWhales       []whaleCarrier
	activeWhales        []whaleCarrier
	lastSnapshotPublish time.Time
	snapshotPublish     func(time.Time) error
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

func (field *Field) FeedBook(update krakenmarket.BookUpdate, at time.Time) error {
	identity, err := krakenmarket.SpotIdentityFromPair(update.Symbol)

	if err != nil {
		return fmt.Errorf("manifold: spot identity for %q: %w", update.Symbol, err)
	}

	return field.feedBookIdentity(identity, update, at)
}

func (field *Field) FeedFuturesBook(update krakenmarket.BookUpdate, at time.Time) error {
	identity, err := krakenmarket.FuturesIdentityFromProduct(update.Symbol)

	if err != nil {
		return fmt.Errorf("manifold: futures identity for %q: %w", update.Symbol, err)
	}

	return field.feedBookIdentity(identity, update, at)
}

func (field *Field) feedBookIdentity(
	identity krakenmarket.InstrumentIdentity,
	update krakenmarket.BookUpdate,
	at time.Time,
) error {
	state := field.universe.loadIdentity(identity)

	if state == nil {
		return fmt.Errorf("manifold: instrument %q unavailable", identity.Symbol)
	}

	if update.Type == "snapshot" {
		state.bookReady = true
		state.book = update
	}

	if !state.bookReady {
		return nil
	}

	if !at.IsZero() {
		state.lastEventAt = at
	}

	bids := update.Bids
	asks := update.Asks

	if len(bids) == 0 {
		bids = state.book.Bids
	}

	if len(asks) == 0 {
		asks = state.book.Asks
	}

	if len(bids) == 0 || len(asks) == 0 {
		return nil
	}

	midPrice := (bids[0].Price + asks[0].Price) / 2

	if midPrice <= 0 {
		return fmt.Errorf("manifold: mid price must be positive for %q", update.Symbol)
	}

	state.midPrice = midPrice
	if err := state.configureTickFromBook(bids, asks); err != nil {
		return err
	}

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

	if state.tickSize <= 0 {
		return fmt.Errorf("manifold: tick size must be positive for %q", trade.Symbol)
	}

	offsetTicks := (trade.Price - state.midPrice) / state.tickSize
	coords := field.universe.coords(state, offsetTicks)

	rho, rhoErr := field.liquidityRho(state, trade.Qty, 1)

	if rhoErr != nil {
		return rhoErr
	}

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

	stepInterval := field.config.IntegrationInterval()
	shouldIntegrate := field.lastStepAt.IsZero() || at.Sub(field.lastStepAt) >= stepInterval

	if !shouldIntegrate {
		return nil
	}

	integrated, err := field.integrate(at)

	if err != nil {
		return err
	}

	if !integrated {
		return nil
	}

	return field.publishSnapshot(at)
}

func (field *Field) publishSnapshot(at time.Time) error {
	if field.snapshotPublish == nil || !field.hasPublishableSnapshot() {
		return nil
	}

	publishInterval := field.config.SnapshotPublishInterval()

	if publishInterval > 0 &&
		!field.lastSnapshotPublish.IsZero() &&
		at.Sub(field.lastSnapshotPublish) < publishInterval {
		return nil
	}

	field.lastSnapshotPublish = at

	if err := field.snapshotPublish(at); err != nil {
		return err
	}

	return nil
}

func (field *Field) hasPublishableSnapshot() bool {
	return !field.lastStepAt.IsZero() &&
		len(field.lastCarriers) > 0 &&
		readingFinite(field.lastReading)
}

func readingFinite(reading physics.Reading) bool {
	values := []float64{
		reading.PressureGradX,
		reading.PressureGradY,
		reading.PressureGradZ,
		reading.PressureGradNorm,
		reading.Divergence,
		reading.CoherenceMag2,
		reading.GuidanceSpeed,
		reading.ViscosityProxy,
	}

	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
	}

	return true
}

func (field *Field) SetSnapshotPublisher(publish func(time.Time) error) {
	field.snapshotPublish = publish
}

func (field *Field) integrate(at time.Time) (bool, error) {
	if err := field.solver.ResetDeposits(); err != nil {
		return false, errnie.Error(err)
	}

	type spotCandidate struct {
		state      *UniverseState
		oscillator physics.Oscillator
		carrier    fieldCarrier
	}

	candidates := make([]spotCandidate, 0)

	field.universe.states.Range(func(_, value any) bool {
		state, ok := value.(*UniverseState)

		if !ok || !state.bookReady || state.midPrice <= 0 || state.tickSize <= 0 {
			return true
		}

		if state.lane != krakenmarket.InstrumentLaneSpot {
			return true
		}

		oscillator := field.oscillatorFromState(state)
		candidates = append(candidates, spotCandidate{
			state:      state,
			oscillator: oscillator,
			carrier: fieldCarrier{
				role:       "symbol",
				symbol:     state.symbol,
				oscillator: oscillator,
			},
		})

		return true
	})

	oscillators := make([]physics.Oscillator, len(candidates))
	carriers := make([]fieldCarrier, len(candidates))

	for index, candidate := range candidates {
		oscillators[index] = candidate.oscillator
		carriers[index] = candidate.carrier
	}

	if len(oscillators) == 0 && len(field.activeWhales) == 0 && len(field.pendingWhales) == 0 {
		return false, nil
	}

	field.activeWhales = append(field.activeWhales, field.pendingWhales...)
	field.pendingWhales = field.pendingWhales[:0]

	whaleOscillators := make([]physics.Oscillator, 0, len(field.activeWhales))
	whaleCarriers := make([]fieldCarrier, 0, len(field.activeWhales))

	for _, whale := range field.activeWhales {
		whaleOscillators = append(whaleOscillators, whale.oscillator)
		whaleCarriers = append(whaleCarriers, fieldCarrier{
			role:       "whale",
			symbol:     whale.symbol,
			oscillator: whale.oscillator,
		})
	}

	solverOscillators, solverCarriers := capSolverCarriers(
		oscillators,
		carriers,
		whaleOscillators,
		whaleCarriers,
		field.config.MaxModes,
	)

	if len(solverOscillators) == 0 {
		return false, nil
	}

	activeCarriers := len(solverOscillators)
	carrierScale := 1.0 / float64(activeCarriers)

	for _, deposit := range field.pendingDeposits {
		if depositErr := field.solver.DepositCell(
			deposit.cellX,
			deposit.cellY,
			deposit.cellZ,
			deposit.rho*carrierScale,
			deposit.momX*carrierScale,
			deposit.momY*carrierScale,
			deposit.momZ*carrierScale,
			deposit.eInt*carrierScale,
		); depositErr != nil {
			return false, errnie.Error(depositErr)
		}
	}

	field.pendingDeposits = field.pendingDeposits[:0]

	selectedSymbols := make(map[string]struct{}, len(solverCarriers))

	for _, carrier := range solverCarriers {
		if carrier.role != "symbol" {
			continue
		}

		selectedSymbols[carrier.symbol] = struct{}{}
	}

	for _, candidate := range candidates {
		if _, selected := selectedSymbols[candidate.state.symbol]; !selected {
			continue
		}

		if depositErr := field.depositBook(candidate.state, activeCarriers); depositErr != nil {
			return false, errnie.Error(depositErr)
		}
	}

	if err := field.solver.SetOscillators(normalizeOscillatorsForSolver(solverOscillators, field.config.RhoMin)); err != nil {
		return false, errnie.Error(err)
	}

	reading, stepErr := field.solver.Step()

	if stepErr != nil {
		return false, errnie.Error(stepErr)
	}

	if !readingFinite(reading) {
		return false, fmt.Errorf("manifold: solver reading is non-finite")
	}

	readOscillators, readErr := field.solver.ReadOscillators(len(solverOscillators))

	if readErr != nil {
		return false, errnie.Error(readErr)
	}

	for index, oscillator := range readOscillators {
		if !oscillatorFullyFinite(oscillator) {
			return false, fmt.Errorf("manifold: solver oscillator[%d] is non-finite", index)
		}
	}

	field.activeWhales = field.whalesFromSolverReadback(solverCarriers, readOscillators)
	field.lastReading = reading
	field.lastStepAt = at
	field.lastCarriers = field.displayCarriers(carriers, solverCarriers, readOscillators)

	field.universe.states.Range(func(_, value any) bool {
		state, ok := value.(*UniverseState)

		if !ok || state.lane != krakenmarket.InstrumentLaneSpot {
			return true
		}

		field.readings.Delete(state.symbol)

		return true
	})

	for _, carrier := range solverCarriers {
		if carrier.role != "symbol" {
			continue
		}

		state := field.universe.loadSymbol(carrier.symbol)
		price := 0.0

		if state != nil {
			price = state.lastPrice
		}

		field.readings.Store(carrier.symbol, symbolReading{
			reading: reading,
			price:   price,
			at:      at,
		})
	}

	return true, nil
}

func (field *Field) depositBook(state *UniverseState, activeCarriers int) error {
	if state.tickSize <= 0 {
		return fmt.Errorf("manifold: tick size must be positive for %q", state.symbol)
	}

	if activeCarriers <= 0 {
		return fmt.Errorf("manifold: active carrier count must be positive")
	}

	bids := truncateLevels(state.book.Bids, state.bookDepth)
	asks := truncateLevels(state.book.Asks, state.bookDepth)
	midPrice := state.midPrice

	depositSide := func(levels []krakenmarket.BookLevel, sign float64) error {
		for _, level := range levels {
			offsetTicks := (level.Price - midPrice) / state.tickSize
			coords := field.universe.coords(state, offsetTicks)

			rho, rhoErr := field.liquidityRho(state, level.Qty, activeCarriers)

			if rhoErr != nil {
				return rhoErr
			}

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

func (field *Field) whalesFromSolverReadback(
	solverCarriers []fieldCarrier,
	readOscillators []physics.Oscillator,
) []whaleCarrier {
	whales := make([]whaleCarrier, 0)

	for index, carrier := range solverCarriers {
		if carrier.role != "whale" || index >= len(readOscillators) {
			continue
		}

		oscillator := readOscillators[index]

		if !oscillatorStateFinite(oscillator) {
			continue
		}

		whales = append(whales, whaleCarrier{
			symbol:     carrier.symbol,
			oscillator: oscillator,
		})
	}

	return whales
}

func displayOscillator(fallback, read physics.Oscillator) physics.Oscillator {
	if oscillatorFullyFinite(read) {
		return read
	}

	return fallback
}

func oscillatorStateFinite(oscillator physics.Oscillator) bool {
	return oscillatorFullyFinite(oscillator)
}

func oscillatorFullyFinite(oscillator physics.Oscillator) bool {
	values := []float64{
		oscillator.PosX,
		oscillator.PosY,
		oscillator.PosZ,
		oscillator.VelX,
		oscillator.VelY,
		oscillator.VelZ,
		oscillator.Phase,
		oscillator.Omega,
		oscillator.Amplitude,
		oscillator.Heat,
	}

	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
	}

	return true
}

func (field *Field) liquidityRho(state *UniverseState, qty float64, activeCarriers int) (float64, error) {
	if qty <= 0 {
		return 0, fmt.Errorf("manifold: qty must be positive for %q", state.symbol)
	}

	if activeCarriers <= 0 {
		return 0, fmt.Errorf("manifold: active carrier count must be positive")
	}

	reference := visibleBookQty(state)

	if reference <= 0 && len(state.tradeQtys) > 0 {
		reference = median(state.tradeQtys) * float64(len(state.tradeQtys))
	}

	if reference <= 0 {
		return 0, fmt.Errorf("manifold: liquidity reference unavailable for %q", state.symbol)
	}

	return (qty / reference) * field.config.RhoMin / float64(activeCarriers), nil
}

func (field *Field) displayCarriers(
	symbolCarriers []fieldCarrier,
	solverCarriers []fieldCarrier,
	readOscillators []physics.Oscillator,
) []fieldCarrier {
	solverSymbols := make(map[string]physics.Oscillator, len(solverCarriers))
	whaleDisplay := make([]fieldCarrier, 0)

	for index, carrier := range solverCarriers {
		if index >= len(readOscillators) {
			break
		}

		readOscillator := readOscillators[index]

		if carrier.role == "whale" {
			if !oscillatorStateFinite(readOscillator) {
				continue
			}

			whaleDisplay = append(whaleDisplay, fieldCarrier{
				role:       "whale",
				symbol:     carrier.symbol,
				oscillator: readOscillator,
			})

			continue
		}

		solverSymbols[carrier.symbol] = readOscillator
	}

	display := make([]fieldCarrier, 0, len(symbolCarriers)+len(whaleDisplay))

	for _, carrier := range symbolCarriers {
		oscillator, inSolver := solverSymbols[carrier.symbol]

		if !inSolver {
			display = append(display, carrier)
			continue
		}

		display = append(display, fieldCarrier{
			role:       "symbol",
			symbol:     carrier.symbol,
			oscillator: displayOscillator(carrier.oscillator, oscillator),
		})
	}

	display = append(display, whaleDisplay...)

	return display
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

func normalizeOscillatorsForSolver(oscillators []physics.Oscillator, rhoMin float64) []physics.Oscillator {
	carrierCount := float64(len(oscillators))

	if carrierCount <= 0 {
		return oscillators
	}

	normalized := make([]physics.Oscillator, len(oscillators))

	for index, oscillator := range oscillators {
		normalized[index] = oscillator
		perCarrierEnergy := math.Max(oscillator.Heat, rhoMin) / carrierCount
		normalized[index].Amplitude = math.Sqrt(perCarrierEnergy)
		normalized[index].Heat = perCarrierEnergy
	}

	return normalized
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

func capSolverCarriers(
	symbolOscillators []physics.Oscillator,
	symbolCarriers []fieldCarrier,
	whaleOscillators []physics.Oscillator,
	whaleCarriers []fieldCarrier,
	maxModes uint32,
) ([]physics.Oscillator, []fieldCarrier) {
	limit := int(maxModes)
	solverOscillators, solverCarriers := capCarriers(
		symbolOscillators,
		symbolCarriers,
		uint32(limit),
	)

	if len(solverOscillators) >= limit {
		return solverOscillators, solverCarriers
	}

	remaining := uint32(limit - len(solverOscillators))
	trimmedWhaleOscillators, trimmedWhaleCarriers := capCarriers(
		whaleOscillators,
		whaleCarriers,
		remaining,
	)

	solverOscillators = append(solverOscillators, trimmedWhaleOscillators...)
	solverCarriers = append(solverCarriers, trimmedWhaleCarriers...)

	return solverOscillators, solverCarriers
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
