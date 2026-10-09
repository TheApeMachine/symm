package audit

import (
	"cmp"
	"context"
	"fmt"
	"math"
	"math/rand"
	"slices"
	"sort"
	"strings"

	"github.com/apache/iceberg-go"
	icetable "github.com/apache/iceberg-go/table"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/strategy"
)

type excursionInterval struct {
	symbol    string
	class     string
	startTick int64
	bTick     int64
	cTick     int64
}

/*
AnalyzePrecursorSeparation measures two causal populations:
 1. A -> B: profitable long ignition precursor (up) versus losing/ordinary controls.
 2. B -> C: late holding state versus early holding state inside profitable long excursions.

Excursion windows are loaded directly from the archive rather than requiring them
to fall inside the audit's arbitrary first-N tick sample. Event and control tape
use the same ChannelsFrom -> Stream.Deform -> LitRegion path. Sufficient evidence
is reported as MEASURED; no arbitrary separation ratio is promoted to PASS.
*/
func AnalyzePrecursorSeparation(
	ctx context.Context,
	catalog *tables.Catalog,
	epoch int64,
	symbol string,
	grid *store.Grid,
	ticks []int64,
	tickMeasurements map[int64][]*data.Measurement,
	permutations int,
	detections []*data.Measurement,
	takerFee ...float64,
) Stage5PrecursorSeparation {
	if catalog == nil || grid == nil {
		return insufficientPrecursor("Catalog/grid unavailable.")
	}

	if permutations <= 0 {
		permutations = 50
	}

	fee := 0.008

	if len(takerFee) > 0 && takerFee[0] > 0 {
		fee = takerFee[0]
	}

	if detections == nil {
		var minTick, maxTick int64
		if len(ticks) > 0 {
			minTick = ticks[0]
			maxTick = ticks[len(ticks)-1]
		}

		loaded, err := loadOrDetectExcursions(ctx, catalog, epoch, symbol, fee, minTick, maxTick)

		if err != nil {
			return insufficientPrecursor("Detection retrieval failed: " + err.Error())
		}

		detections = loaded
	}

	if len(detections) == 0 {
		return insufficientPrecursor("Zero excursions detected.")
	}

	excursions := make([]excursionInterval, 0, len(detections))
	classSet := make(map[string]struct{})
	for _, det := range detections {
		class := det.Meta("type")
		if class == "" {
			class = "unknown"
		}
		classSet[class] = struct{}{}

		startTick := int64(getMeasurementMetric(det, "start_tick"))
		bTick := int64(getMeasurementMetric(det, "b_tick"))
		cTick := int64(getMeasurementMetric(det, "c_tick"))

		if bTick <= 0 || cTick <= bTick {
			continue
		}

		lo, _ := strategy.PadWindow(bTick, cTick)
		if startTick > 0 && startTick < lo {
			lo = startTick
		}
		startTick = lo

		if bTick <= startTick {
			continue
		}

		excursions = append(excursions, excursionInterval{
			symbol: det.Label, class: class, startTick: startTick, bTick: bTick, cTick: cTick,
		})
	}

	classList := make([]string, 0, len(classSet))
	for class := range classSet {
		classList = append(classList, class)
	}
	sort.Strings(classList)

	if len(excursions) == 0 {
		result := insufficientPrecursor("Detections contained no valid A < B < C intervals.")
		result.DetectionsFound = len(detections)
		result.ExcursionsFound = classList
		return result
	}

	// Non-excursion background is taken only from the already loaded chronological
	// audit sample. It is supplemental control evidence; event populations do not
	// depend on the sample containing the excursion.
	backgroundTokens := sampledBackgroundTokens(
		grid, ticks, tickMeasurements, excursions,
	)

	ignitionTokens := make(map[string]int)
	ignitionControls := make(map[string]int)
	holdingTokens := make(map[string]int)
	exhaustionTokens := make(map[string]int)

	for _, excursion := range excursions {
		eventTokens, err := archivedIntervalTokens(
			ctx, catalog, epoch, excursion.symbol, grid, excursion.startTick, excursion.cTick, tickMeasurements,
		)
		if err != nil {
			result := insufficientPrecursor("Event-window replay failed: " + err.Error())
			result.DetectionsFound = len(detections)
			result.ExcursionsFound = classList
			return result
		}

		// Current Desk semantics are long-only: only profitable "up" excursions
		// are positive A->B/holding examples. down/up_friction/chop/flat are controls.
		if excursion.class == "up" {
			for tick, token := range eventTokens {
				switch {
				case tick >= excursion.startTick && tick < excursion.bTick:
					ignitionTokens[token]++
				case tick >= excursion.bTick && tick <= excursion.cTick:
					// Split the realized holding interval in half only to form a
					// within-episode early-vs-late comparison. This is descriptive;
					// the audit does not call the late half "exhaustion truth".
					mid := excursion.bTick + (excursion.cTick-excursion.bTick)/2
					if tick <= mid {
						holdingTokens[token]++
					} else {
						exhaustionTokens[token]++
					}
				}
			}
			continue
		}

		for tick, token := range eventTokens {
			if tick >= excursion.startTick && tick < excursion.bTick {
				ignitionControls[token]++
			}
		}
	}

	if totalTokenCount(ignitionControls) < 5 {
		for token, count := range backgroundTokens {
			ignitionControls[token] += count
		}
	}

	ignition := evaluateHypothesis(
		"A -> B Ignition Precursor",
		"Long-only profitable up precursor versus losing/ordinary controls",
		ignitionTokens,
		ignitionControls,
		totalTokenCount(ignitionTokens),
		totalTokenCount(ignitionControls),
		permutations,
	)
	exhaustion := evaluateHypothesis(
		"B -> C Holding Deterioration",
		"Late versus early holding state inside profitable up excursions",
		exhaustionTokens,
		holdingTokens,
		totalTokenCount(exhaustionTokens),
		totalTokenCount(holdingTokens),
		permutations,
	)

	measured := ignition.Status == "MEASURED"

	return Stage5PrecursorSeparation{
		DetectionsFound:      len(detections),
		ExcursionsFound:      classList,
		IgnitionHypothesis:   ignition,
		ExhaustionHypothesis: exhaustion,
		BackgroundTokens:     backgroundTokens,
		SummaryText: fmt.Sprintf(
			"Precursor: %d detections (%s). A->B: %s, JSD %.3f vs |null| p95 %.3f, N=%d/%d. "+
				"B->C: %s, JSD %.3f vs |null| p95 %.3f, N=%d/%d. Background observations=%d.",
			len(detections), strings.Join(classList, "/"),
			ignition.Status, ignition.DivergenceBits, ignition.NullDivergence95,
			ignition.EventTokenCount, ignition.ControlTokenCount,
			exhaustion.Status, exhaustion.DivergenceBits, exhaustion.NullDivergence95,
			exhaustion.EventTokenCount, exhaustion.ControlTokenCount,
			totalTokenCount(backgroundTokens),
		),
		Passed: measured,
	}
}

func insufficientPrecursor(reason string) Stage5PrecursorSeparation {
	return Stage5PrecursorSeparation{
		IgnitionHypothesis: PrecursorHypothesis{
			Name:        "A -> B Ignition Precursor",
			Description: "Long-only profitable up precursor versus losing/ordinary controls",
			Status:      "INSUFFICIENT_DATA",
		},
		ExhaustionHypothesis: PrecursorHypothesis{
			Name:        "B -> C Holding Deterioration",
			Description: "Late versus early holding state inside profitable up excursions",
			Status:      "INSUFFICIENT_DATA",
		},
		SummaryText: "Precursor Separation: INSUFFICIENT_DATA (" + reason + ")",
		Passed:      false,
	}
}

func sampledBackgroundTokens(
	grid *store.Grid,
	ticks []int64,
	tickMeasurements map[int64][]*data.Measurement,
	excursions []excursionInterval,
) map[string]int {
	result := make(map[string]int)

	for _, tick := range ticks {
		group := tickMeasurements[tick]

		if len(group) == 0 {
			continue
		}

		bySymbol := make(map[string][]*data.Measurement)

		for _, m := range group {
			if m != nil {
				bySymbol[m.Label] = append(bySymbol[m.Label], m)
			}
		}

		for sym, symMeas := range bySymbol {
			train := data.NewMeasurement(
				symMeas[0].Epoch,
				sym,
				"audit",
				symMeas[0].SeqIdx,
				tick,
			)
			train.Peers(symMeas...)
			train.Write()

			tokenBytes := grid.Observe(train)

			if len(tokenBytes) == 0 {
				continue
			}

			inExcursion := false

			for _, excursion := range excursions {
				if excursion.symbol == sym && tick >= excursion.startTick && tick <= excursion.cTick {
					inExcursion = true
					break
				}
			}

			if !inExcursion {
				result[string(tokenBytes)]++
			}
		}
	}

	return result
}

func archivedIntervalTokens(
	ctx context.Context,
	catalog *tables.Catalog,
	epoch int64,
	symbol string,
	grid *store.Grid,
	startTick int64,
	endTick int64,
	tickMeasurements map[int64][]*data.Measurement,
) (map[int64]string, error) {
	if grid == nil {
		return nil, nil
	}

	tickMap := make(map[int64][]*data.Measurement)

	for tick := startTick; tick <= endTick; tick++ {
		list, exists := tickMeasurements[tick]

		if !exists {
			continue
		}

		for _, m := range list {
			if m != nil && (symbol == "" || m.Label == symbol) {
				tickMap[tick] = append(tickMap[tick], m)
			}
		}
	}

	if len(tickMap) == 0 {
		return nil, nil
	}

	matchingTicks := make([]int64, 0, len(tickMap))

	for tick := range tickMap {
		matchingTicks = append(matchingTicks, tick)
	}

	slices.Sort(matchingTicks)
	result := make(map[int64]string, len(matchingTicks))

	for _, tick := range matchingTicks {
		signals := tickMap[tick]

		if len(signals) == 0 {
			continue
		}

		train := data.NewMeasurement(
			signals[0].Epoch,
			symbol,
			"audit",
			signals[0].SeqIdx,
			tick,
		)
		train.Peers(signals...)
		train.Write()

		tokenBytes := grid.Observe(train)

		if len(tokenBytes) > 0 {
			result[tick] = string(tokenBytes)
		}
	}

	return result, nil
}

func totalTokenCount(tokens map[string]int) int {
	total := 0
	for _, count := range tokens {
		total += count
	}
	return total
}

func evaluateHypothesis(
	name string,
	description string,
	eventTokens map[string]int,
	controlTokens map[string]int,
	eventCount int,
	controlCount int,
	permutations int,
) PrecursorHypothesis {
	if eventCount < 5 || controlCount < 5 {
		return PrecursorHypothesis{
			Name:              name,
			Description:       description,
			EventTokens:       eventTokens,
			ControlTokens:     controlTokens,
			EventTokenCount:   eventCount,
			ControlTokenCount: controlCount,
			Status:            "INSUFFICIENT_DATA",
			Passed:            false,
		}
	}

	realJSD := computeJSD(eventTokens, controlTokens)
	pool := make([]string, 0, eventCount+controlCount)
	for token, count := range eventTokens {
		for index := 0; index < count; index++ {
			pool = append(pool, token)
		}
	}
	for token, count := range controlTokens {
		for index := 0; index < count; index++ {
			pool = append(pool, token)
		}
	}

	rng := rand.New(rand.NewSource(1791))
	nullDivergences := make([]float64, 0, permutations)
	for iteration := 0; iteration < permutations; iteration++ {
		shuffled := append([]string(nil), pool...)
		rng.Shuffle(len(shuffled), func(first, second int) {
			shuffled[first], shuffled[second] = shuffled[second], shuffled[first]
		})

		nullEvent := make(map[string]int)
		nullControl := make(map[string]int)
		for index := 0; index < eventCount; index++ {
			nullEvent[shuffled[index]]++
		}
		for index := eventCount; index < len(shuffled); index++ {
			nullControl[shuffled[index]]++
		}
		nullDivergences = append(
			nullDivergences,
			computeJSD(nullEvent, nullControl),
		)
	}

	sort.Float64s(nullDivergences)
	null95 := empiricalQuantile(nullDivergences, 0.95)
	ratio := 0.0
	if null95 > 0 {
		ratio = realJSD / null95
	}

	return PrecursorHypothesis{
		Name:              name,
		Description:       description,
		EventTokens:       eventTokens,
		ControlTokens:     controlTokens,
		EventTokenCount:   eventCount,
		ControlTokenCount: controlCount,
		DivergenceBits:    realJSD,
		NullDivergence95:  null95,
		SeparationRatio:   ratio,
		Status:            "MEASURED",
		Passed:            true,
	}
}

/*
offlinePrice constructs an in-memory Price system with offline instrument rules and
configured fee schedules without dialing exchange WebSockets or REST endpoints.
*/
func offlinePrice(ctx context.Context, takerFee float64, symbols ...string) *broker.Price {
	if takerFee <= 0 {
		takerFee = 0.008
	}

	normalizer := spot.NewNormalizer()
	assets := make(map[string]spot.AssetInfo)
	pairs := make(map[string]spot.AssetPair)

	for _, sym := range symbols {
		if sym == "" {
			continue
		}

		parts := strings.Split(sym, "/")
		base := sym
		quote := "USD"

		if len(parts) == 2 {
			base = parts[0]
			quote = parts[1]
		}

		assets[base] = spot.AssetInfo{AltName: base}
		assets[quote] = spot.AssetInfo{AltName: quote}
		pairs[sym] = spot.AssetPair{
			WSName:        sym,
			Base:          base,
			Quote:         quote,
			LotDecimals:   8,
			LotMultiplier: 1,
		}
	}

	normalizer.Update(&spot.AssetsManagerUpdate{
		NewAssets: assets,
		NewPairs:  pairs,
	})

	instPairs := make([]kraken.InstrumentPair, 0, len(symbols))
	for _, sym := range symbols {
		if sym == "" {
			continue
		}
		instPairs = append(instPairs, kraken.InstrumentPair{
			Symbol:  sym,
			Quote:   "USD",
			CostMin: decimal.NewFromFloat64(0.01),
			Status:  "online",
		})
	}

	inst := broker.NewOfflineInstrument(ctx, "USD", instPairs...)
	paper := broker.NewPaper(ctx)
	paper.Transition(runtime.READY)

	price := broker.NewPrice(ctx, nil, paper, inst, normalizer)

	for _, sym := range symbols {
		if sym == "" {
			continue
		}

		price.SetFee(sym, kraken.TradeVolumeFee{
			Fee: decimal.NewFromFloat64(takerFee),
		})
	}

	price.SetReferenceCash(decimal.NewFromFloat64(10000))
	price.Transition(runtime.READY)
	return price
}

func detectInMemory(
	ctx context.Context,
	catalog *tables.Catalog,
	epoch int64,
	symbol string,
	takerFee float64,
	minTick, maxTick int64,
) ([]*data.Measurement, error) {
	tbl, err := catalog.Load(ctx, tables.Measurements)
	if err != nil {
		return nil, errnie.Error(err)
	}

	s3Ctx := catalog.Context(ctx)
	filter := iceberg.NewAnd(
		iceberg.EqualTo(iceberg.Reference("epoch"), epoch),
		iceberg.EqualTo(iceberg.Reference("source"), "spot:trade"),
	)

	if symbol != "" {
		filter = iceberg.NewAnd(filter, iceberg.EqualTo(iceberg.Reference("label"), symbol))
	}

	if minTick > 0 {
		filter = iceberg.NewAnd(filter, iceberg.GreaterThanEqual(iceberg.Reference("tick"), minTick))
	}

	if maxTick > 0 && maxTick >= minTick {
		filter = iceberg.NewAnd(filter, iceberg.LessThanEqual(iceberg.Reference("tick"), maxTick))
	}

	tasks, err := tbl.Scan(icetable.WithRowFilter(filter)).PlanFiles(s3Ctx)
	if err != nil {
		return nil, errnie.Error(err)
	}

	if len(tasks) == 0 {
		return nil, nil
	}

	chunkSize := 250
	grouped := make(map[string][]*data.Measurement)

	for taskIndex := 0; taskIndex < len(tasks); taskIndex += chunkSize {
		endIndex := min(taskIndex+chunkSize, len(tasks))

		scanOpts := []icetable.ScanOption{
			icetable.WithRowFilter(filter),
			icetable.WitMaxConcurrency(16),
		}

		_, batches, readErr := tbl.Scan(scanOpts...).ReadTasks(s3Ctx, tasks[taskIndex:endIndex])
		if readErr != nil {
			errnie.Warn(fmt.Sprintf("[audit] failed to read tasks chunk %d-%d: %s", taskIndex, endIndex, readErr))
			continue
		}

		for batch, batchErr := range batches {
			if batchErr != nil {
				continue
			}

			if batch == nil {
				continue
			}

			measurements, mErr := tables.ReadMeasurements(batch)
			batch.Release()

			if mErr != nil {
				continue
			}

			for _, measurement := range measurements {
				if measurement != nil {
					grouped[measurement.Label] = append(grouped[measurement.Label], measurement)
				}
			}
		}
	}

	if len(grouped) == 0 {
		return nil, nil
	}

	symbols := make([]string, 0, len(grouped))
	for sym := range grouped {
		symbols = append(symbols, sym)
	}
	slices.Sort(symbols)

	price := offlinePrice(ctx, takerFee, symbols...)
	storeTee := hindsight.NewStoreTee(ctx, "auditDetectorTee")
	storeTee.Transition(runtime.READY)

	detector := strategy.NewDetector(ctx, storeTee, price)
	if detector.Status() == runtime.ERROR {
		return nil, errnie.Error(detector.Error())
	}

	for _, sym := range symbols {
		trades := grouped[sym]
		slices.SortFunc(trades, func(left, right *data.Measurement) int {
			if cmpResult := cmp.Compare(left.Epoch, right.Epoch); cmpResult != 0 {
				return cmpResult
			}
			if cmpResult := cmp.Compare(left.Tick, right.Tick); cmpResult != 0 {
				return cmpResult
			}
			return cmp.Compare(left.SeqIdx, right.SeqIdx)
		})

		seq := func(yield func(*data.Measurement, error) bool) {
			for _, m := range trades {
				if !yield(m, nil) {
					return
				}
			}
		}

		if err := detector.Scan(seq); err != nil {
			return nil, err
		}
	}

	var detections []*data.Measurement

	for {
		measurement := storeTee.Pop()

		if measurement == nil {
			break
		}

		detections = append(detections, measurement)
	}

	return detections, nil
}

func getMeasurementMetric(m *data.Measurement, key string) float64 {
	if m == nil {
		return 0
	}
	for entry := range m.Read() {
		if entry != nil && entry.Key == key && entry.Metric != nil {
			return entry.Metric.Raw
		}
	}
	return 0
}

/*
computeJSD calculates the Jensen-Shannon Divergence in bits between two token distributions.
*/
func computeJSD(pCounts, qCounts map[string]int) float64 {
	totalP := 0
	for _, c := range pCounts {
		totalP += c
	}
	totalQ := 0
	for _, c := range qCounts {
		totalQ += c
	}

	if totalP == 0 || totalQ == 0 {
		return 0.0
	}

	allKeys := make(map[string]struct{})
	for k := range pCounts {
		allKeys[k] = struct{}{}
	}
	for k := range qCounts {
		allKeys[k] = struct{}{}
	}

	klPM := 0.0
	klQM := 0.0

	for k := range allKeys {
		p := float64(pCounts[k]) / float64(totalP)
		q := float64(qCounts[k]) / float64(totalQ)
		m := 0.5 * (p + q)

		if p > 0 && m > 0 {
			klPM += p * math.Log2(p/m)
		}
		if q > 0 && m > 0 {
			klQM += q * math.Log2(q/m)
		}
	}

	jsd := 0.5*klPM + 0.5*klQM
	if jsd < 0 {
		return 0.0
	}
	return jsd
}

func loadOrDetectExcursions(
	ctx context.Context,
	catalog *tables.Catalog,
	epoch int64,
	symbol string,
	takerFee float64,
	bounds ...int64,
) ([]*data.Measurement, error) {
	fee := takerFee
	if fee <= 0 {
		fee = 0.008
	}

	var minTick, maxTick int64
	if len(bounds) >= 2 {
		minTick = bounds[0]
		maxTick = bounds[1]
	}

	var detections []*data.Measurement

	for det, err := range catalog.Detections(ctx, epoch) {
		if err != nil {
			return nil, errnie.Error(err)
		}

		if det != nil && (symbol == "" || det.Label == symbol) {
			bTick := int64(getMeasurementMetric(det, "b_tick"))
			if minTick > 0 && bTick < minTick {
				continue
			}
			if maxTick > 0 && bTick > maxTick {
				continue
			}

			detections = append(detections, det)
		}
	}

	if len(detections) == 0 {
		inMemory, err := detectInMemory(ctx, catalog, epoch, symbol, fee, minTick, maxTick)

		if err != nil {
			return nil, errnie.Error(err)
		}

		detections = inMemory
	}

	return detections, nil
}
