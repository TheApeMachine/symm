package manifold

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	mkernel "github.com/theapemachine/nomagique/physics/manifold"
	"github.com/theapemachine/symm/signal/compute"
)

/*
Field owns the shared GPU manifold solver and projects the live universe into it.
All mutations run on the manifold System tick goroutine; readings publish through sync.Map.
*/
type Field struct {
	config                 mkernel.Config
	solver                 *mkernel.Solver
	universe               *Universe
	lastStepAt             time.Time
	lastIntegratedCarriers int
	lastReading            mkernel.Reading
	lastCarriers           []fieldCarrier
	readings               sync.Map
	pendingDeposits        []cellDeposit
	pendingWhales          []whaleCarrier
	activeWhales           []whaleCarrier
	lastRecreateAt         time.Time
	measurementsCapacity   int
	serial                 *compute.SerialPool
}

type whaleCarrier struct {
	symbol     string
	oscillator mkernel.Oscillator
}

type fieldCarrier struct {
	role       string
	symbol     string
	oscillator mkernel.Oscillator
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
	reading mkernel.Reading
	price   float64
	at      time.Time
}

const (
	solverCarrierPosX = 1.0
	solverCarrierPosY = 0.0
	solverCarrierPosZ = 1.0
)

func NewField() (*Field, error) {
	bookDepth := viper.GetInt("market.book.depth")

	if bookDepth <= 0 {
		bookDepth = viper.GetInt("market.book_depth_levels")
	}

	if bookDepth <= 0 {
		return nil, fmt.Errorf("manifold: book depth must be positive")
	}

	symbolCount := max(len(viper.GetStringSlice("market.default_symbols")), 1)

	gridX := uint32(bookDepth * 4)
	gridY := max(uint32(symbolCount), 3)
	gridZ := uint32(math.Max(3, math.Log2(float64(symbolCount*2))))
	halfWidth := bookDepth * 3

	deltaT := 1.0
	gamma := 5.0 / 3.0
	tickSize := 1.0 / float64(math.Pow(2, float64(bookDepth)))
	maxModes := gridX * gridY

	kernelConfig, configErr := mkernel.NewConfig(
		gridX,
		gridY,
		gridZ,
		tickSize,
		halfWidth,
		deltaT,
		gamma,
		maxModes,
	)

	if configErr != nil {
		return nil, configErr
	}

	kernelConfig.SetSnapshotPublishInterval(100 * time.Millisecond * 5)

	universe, universeErr := NewUniverse(kernelConfig)

	if universeErr != nil {
		return nil, universeErr
	}

	return &Field{
		config:               kernelConfig,
		universe:             universe,
		measurementsCapacity: fieldMeasurementsCapacity(),
	}, nil
}

func (field *Field) ensureSolver() error {
	if field == nil {
		return fmt.Errorf("manifold: field is nil")
	}

	if field.solver != nil {
		return nil
	}

	var solver *mkernel.Solver
	var solverErr error

	gateErr := compute.WithMetalInit(func() error {
		solver, solverErr = mkernel.NewSolver(field.config)

		return solverErr
	})

	if gateErr != nil {
		return gateErr
	}

	if solverErr != nil {
		return solverErr
	}

	field.solver = solver

	return nil
}

func (field *Field) Close() {
	if field == nil || field.solver == nil {
		return
	}

	field.solver.Close()
	field.solver = nil
}

func (field *Field) recreateSolver() error {
	if field.solver != nil {
		field.solver.Close()
	}

	var solver *mkernel.Solver
	var solverErr error

	gateErr := compute.WithMetalInit(func() error {
		solver, solverErr = mkernel.NewSolver(field.config)

		return solverErr
	})

	if gateErr != nil {
		return gateErr
	}

	if solverErr != nil {
		return solverErr
	}

	field.solver = solver
	field.lastReading = mkernel.Reading{}
	field.lastCarriers = nil
	field.lastRecreateAt = time.Now()

	return nil
}

func (field *Field) RegisterSymbols(symbols []string) {
	field.universe.registerSymbols(symbols)
}

func (field *Field) FeedTicker(row TickerUpdate, at time.Time) error {
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

	return field.maybeStep(at)
}

func (field *Field) FeedBook(update BookUpdate, at time.Time) error {
	identity, err := SpotIdentityFromPair(update.Symbol)

	if err != nil {
		return fmt.Errorf("manifold: spot identity for %q: %w", update.Symbol, err)
	}

	return field.feedBookIdentity(identity, update, at)
}

func (field *Field) FeedFuturesBook(update BookUpdate, at time.Time) error {
	identity, err := FuturesIdentityFromProduct(update.Symbol)

	if err != nil {
		return fmt.Errorf("manifold: futures identity for %q: %w", update.Symbol, err)
	}

	return field.feedBookIdentity(identity, update, at)
}

func (field *Field) feedBookIdentity(
	identity InstrumentIdentity,
	update BookUpdate,
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

	if update.Type == "snapshot" || state.tickSize <= 0 {
		if err := state.configureTickFromBook(bids, asks, field.universe.tickSizeFallback()); err != nil {
			return err
		}
	}

	if state.lane == InstrumentLaneSpot {
		field.recordPrice(state, midPrice, at)
	}

	return field.maybeStep(at)
}

func (field *Field) FeedTrade(trade *TradeUpdate, at time.Time) error {
	state := field.universe.loadSymbol(trade.Symbol)

	if state == nil {
		return fmt.Errorf("manifold: symbol %q unavailable", trade.Symbol)
	}

	if !at.IsZero() {
		state.lastEventAt = at
	}

	field.recordPrice(state, trade.Price, at)
	state.recordTradeQty(trade.Qty, field.measurementsCapacity)

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

func (field *Field) Reading(symbol string) (mkernel.Reading, float64, time.Time, bool) {
	raw, ok := field.readings.Load(symbol)

	if !ok {
		return mkernel.Reading{}, 0, time.Time{}, false
	}

	row, rowOk := raw.(symbolReading)

	if !rowOk {
		return mkernel.Reading{}, 0, time.Time{}, false
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
	state.AppendReturn(logReturn, field.measurementsCapacity)

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

	return nil
}

func (field *Field) hasPublishableSnapshot() bool {
	return field.solver != nil &&
		!field.lastStepAt.IsZero() &&
		len(field.lastCarriers) > 0 &&
		readingFinite(field.lastReading)
}

func readingFinite(reading mkernel.Reading) bool {
	return reading.IsFinite()
}

func (field *Field) integrate(at time.Time) (bool, error) {
	if ensureErr := field.ensureSolver(); ensureErr != nil {
		return false, ensureErr
	}

	if err := field.solver.ResetDeposits(); err != nil {
		return false, errnie.Error(err)
	}

	type spotCandidate struct {
		state      *UniverseState
		oscillator mkernel.Oscillator
		carrier    fieldCarrier
	}

	candidates := make([]spotCandidate, 0)

	field.universe.states.Range(func(_, value any) bool {
		state, ok := value.(*UniverseState)

		if !ok || !state.bookReady || state.midPrice <= 0 || state.tickSize <= 0 {
			return true
		}

		if state.lane != InstrumentLaneSpot {
			return true
		}

		oscillator := field.oscillatorFromState(state)

		if !oscillatorFullyFinite(oscillator) {
			return true
		}

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

	oscillators := make([]mkernel.Oscillator, len(candidates))
	carriers := make([]fieldCarrier, len(candidates))

	for index, candidate := range candidates {
		oscillators[index] = oscillatorForSolver(candidate.oscillator)
		carriers[index] = candidate.carrier
	}

	if len(oscillators) == 0 && len(field.activeWhales) == 0 && len(field.pendingWhales) == 0 {
		return false, nil
	}

	field.activeWhales = append(field.activeWhales, field.pendingWhales...)
	field.pendingWhales = field.pendingWhales[:0]

	whaleOscillators := make([]mkernel.Oscillator, 0, len(field.activeWhales))
	whaleCarriers := make([]fieldCarrier, 0, len(field.activeWhales))

	for _, whale := range field.activeWhales {
		whaleOscillators = append(whaleOscillators, oscillatorForSolver(whale.oscillator))
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

	carrierCount := len(solverOscillators)

	if field.lastIntegratedCarriers > 0 && field.lastIntegratedCarriers != carrierCount {
		if recreateErr := field.recreateSolver(); recreateErr != nil {
			return false, errnie.Error(recreateErr)
		}
	}

	activeCarriers := carrierCount

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
			return false, errnie.Error(depositErr)
		}
	}

	field.pendingDeposits = field.pendingDeposits[:0]

	activeSymbols := make(map[string]struct{}, len(solverCarriers))

	for _, carrier := range solverCarriers {
		if carrier.role != "symbol" {
			continue
		}

		activeSymbols[carrier.symbol] = struct{}{}
	}

	var depositErr error

	field.universe.states.Range(func(_, value any) bool {
		state, ok := value.(*UniverseState)

		if !ok || !state.bookReady || state.midPrice <= 0 || state.tickSize <= 0 {
			return true
		}

		if state.lane != InstrumentLaneSpot {
			return true
		}

		if _, active := activeSymbols[state.symbol]; !active {
			return true
		}

		if stepErr := field.depositBook(state, activeCarriers); stepErr != nil {
			depositErr = stepErr

			return false
		}

		return true
	})

	if depositErr != nil {
		return false, errnie.Error(depositErr)
	}

	if err := field.solver.SetOscillators(
		normalizeOscillatorsForSolver(
			solverOscillators,
			field.config.RhoMin,
			field.config.MaxModes,
		),
	); err != nil {
		return false, errnie.Error(err)
	}

	reading, err := field.solver.Step()

	if err != nil {
		return false, errnie.Error(err)
	}

	if !readingFinite(reading) {
		return false, fmt.Errorf("manifold: solver reading is non-finite")
	}

	readOscillators, err := field.solver.ReadOscillators(len(solverOscillators))

	if err != nil {
		return false, errnie.Error(err)
	}

	field.activeWhales = field.whalesFromSolverReadback(solverCarriers, readOscillators)
	field.lastReading = reading
	field.lastStepAt = at
	field.lastIntegratedCarriers = carrierCount
	field.lastCarriers = field.displayCarriers(carriers, solverCarriers, readOscillators)

	field.universe.states.Range(func(_, value any) bool {
		state, ok := value.(*UniverseState)

		if !ok || state.lane != InstrumentLaneSpot {
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

	bids := truncateLevels(state.book.Bids, 1)
	asks := truncateLevels(state.book.Asks, 1)

	for _, level := range bids {
		if depositErr := field.depositBookLevel(
			state, level.Price, level.Qty, activeCarriers, -1,
		); depositErr != nil {
			return depositErr
		}
	}

	for _, level := range asks {
		if depositErr := field.depositBookLevel(
			state, level.Price, level.Qty, activeCarriers, 1,
		); depositErr != nil {
			return depositErr
		}
	}

	return nil
}

func (field *Field) depositBookLevel(
	state *UniverseState,
	price, qty float64,
	activeCarriers int,
	sideSign float64,
) error {
	if qty <= 0 {
		return nil
	}

	offsetTicks := (price - state.midPrice) / state.tickSize
	coords := field.universe.coords(state, offsetTicks)

	rho, rhoErr := field.liquidityRho(state, qty, activeCarriers)

	if rhoErr != nil {
		return rhoErr
	}

	momentum := sideSign * rho

	return field.solver.DepositCell(
		coords.cellX,
		coords.cellY,
		coords.cellZ,
		rho,
		momentum,
		0,
		0,
		rho*field.config.CV,
	)
}

func (field *Field) whalesFromSolverReadback(
	solverCarriers []fieldCarrier,
	readOscillators []mkernel.Oscillator,
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

func displayOscillator(fallback, read mkernel.Oscillator) mkernel.Oscillator {
	if !oscillatorFullyFinite(read) {
		return fallback
	}

	merged := read
	merged.PosX = fallback.PosX
	merged.PosY = fallback.PosY
	merged.PosZ = fallback.PosZ

	return merged
}

func oscillatorStateFinite(oscillator mkernel.Oscillator) bool {
	return oscillatorFullyFinite(oscillator)
}

func oscillatorFullyFinite(oscillator mkernel.Oscillator) bool {
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

	tradeQtys := state.GetTradeQtys()
	if reference <= 0 && len(tradeQtys) > 0 {
		reference = median(tradeQtys) * float64(len(tradeQtys))
	}

	if reference <= 0 {
		return 0, fmt.Errorf("manifold: liquidity reference unavailable for %q", state.symbol)
	}

	carrierCapacity := activeCarriers

	if uint32(activeCarriers) < field.config.MaxModes {
		carrierCapacity = int(field.config.MaxModes)
	}

	return (qty / reference) * field.config.RhoMin / float64(carrierCapacity), nil
}

func (field *Field) displayCarriers(
	symbolCarriers []fieldCarrier,
	solverCarriers []fieldCarrier,
	readOscillators []mkernel.Oscillator,
) []fieldCarrier {
	solverSymbols := make(map[string]mkernel.Oscillator, len(solverCarriers))
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
	trade *TradeUpdate,
	coords Coords,
	rho float64,
) mkernel.Oscillator {
	omega := returnFrequency(state.GetReturns(), field.config.DeltaT)
	energy := math.Max(rho, field.config.RhoMin)
	speed := math.Sqrt(energy)
	phase := 0.0

	if trade.Side == "sell" {
		phase = math.Pi
	}

	return mkernel.Oscillator{
		Phase:     phase,
		Omega:     omega,
		Amplitude: speed,
		PosX:      coords.posX,
		PosY:      coords.posY,
		PosZ:      coords.posZ,
		Heat:      rho,
		VelX:      tradeSideSign(trade.Side) * speed,
	}
}

func (field *Field) oscillatorFromState(state *UniverseState) mkernel.Oscillator {
	returns := state.GetReturns()
	energy := medianAbsolute(returns)
	omega := returnFrequency(returns, field.config.DeltaT)
	coords := field.universe.coords(state, 0)

	return mkernel.Oscillator{
		Phase:     returnAnalyticPhase(returns),
		Omega:     omega,
		Amplitude: math.Sqrt(math.Max(energy, field.config.RhoMin)),
		PosX:      coords.posX,
		PosY:      coords.posY,
		PosZ:      coords.posZ,
		Heat:      energy,
	}
}

func returnAnalyticPhase(returns []float64) float64 {
	if len(returns) < 2 {
		return 0
	}

	tail := returns

	if len(returns) > 8 {
		tail = returns[len(returns)-8:]
	}

	realPart := 0.0
	imagPart := 0.0

	for index, value := range tail {
		weight := float64(index + 1)
		realPart += value * weight

		if index > 0 {
			imagPart += (value - tail[index-1]) * weight
		}
	}

	if realPart == 0 && imagPart == 0 {
		return 0
	}

	angle := math.Atan2(imagPart, realPart)

	if angle < 0 {
		angle += 2 * math.Pi
	}

	return angle
}

func oscillatorForSolver(oscillator mkernel.Oscillator) mkernel.Oscillator {
	solverOscillator := oscillator
	solverOscillator.PosX = solverCarrierPosX
	solverOscillator.PosY = solverCarrierPosY
	solverOscillator.PosZ = solverCarrierPosZ

	return solverOscillator
}

func normalizeOscillatorsForSolver(
	oscillators []mkernel.Oscillator,
	rhoMin float64,
	maxModes uint32,
) []mkernel.Oscillator {
	if len(oscillators) == 0 {
		return oscillators
	}

	normalizationCount := float64(maxModes)

	if normalizationCount <= 0 {
		normalizationCount = float64(len(oscillators))
	}

	normalized := make([]mkernel.Oscillator, len(oscillators))

	for index, oscillator := range oscillators {
		normalized[index] = oscillator
		perCarrierEnergy := math.Max(oscillator.Heat, rhoMin) / normalizationCount
		normalized[index].Amplitude = math.Sqrt(perCarrierEnergy)
		normalized[index].Heat = perCarrierEnergy
	}

	return normalized
}

func returnFrequency(returns []float64, deltaT float64) float64 {
	if deltaT <= 0 {
		return 0
	}

	if len(returns) < 2 {
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

func truncateLevels(levels []BookLevel, depth int) []BookLevel {
	if depth <= 0 || len(levels) <= depth {
		return levels
	}

	return levels[:depth]
}

func capSolverCarriers(
	symbolOscillators []mkernel.Oscillator,
	symbolCarriers []fieldCarrier,
	whaleOscillators []mkernel.Oscillator,
	whaleCarriers []fieldCarrier,
	maxModes uint32,
) ([]mkernel.Oscillator, []fieldCarrier) {
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
	oscillators []mkernel.Oscillator,
	carriers []fieldCarrier,
	maxCount uint32,
) ([]mkernel.Oscillator, []fieldCarrier) {
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

	trimmedOscillators := make([]mkernel.Oscillator, limit)
	trimmedCarriers := make([]fieldCarrier, limit)

	for rank := 0; rank < limit; rank++ {
		sourceIndex := indices[rank]
		trimmedOscillators[rank] = oscillators[sourceIndex]
		trimmedCarriers[rank] = carriers[sourceIndex]
	}

	return trimmedOscillators, trimmedCarriers
}

func fieldMeasurementsCapacity() int {
	capacity := viper.GetInt("signals.manifold.measurements_capacity")

	if capacity <= 0 {
		capacity = 64
	}

	return capacity
}
