package audit

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"

	"github.com/apache/iceberg-go"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
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
) Stage5PrecursorSeparation {
	if catalog == nil || grid == nil || grid.RegionsFormed() == 0 {
		return insufficientPrecursor("Catalog/grid unavailable.")
	}
	if permutations <= 0 {
		permutations = 50
	}

	var detections []*data.Measurement
	for det, err := range catalog.Detections(ctx, epoch) {
		if err != nil {
			return insufficientPrecursor("Detection read failed: " + err.Error())
		}
		if det != nil && det.Label == symbol {
			detections = append(detections, det)
		}
	}

	if len(detections) == 0 {
		inMemory, err := detectInMemory(ctx, catalog, epoch, symbol)
		if err != nil {
			return insufficientPrecursor("In-memory detection failed: " + err.Error())
		}
		detections = inMemory
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
		if bTick <= startTick || cTick <= bTick {
			continue
		}
		excursions = append(excursions, excursionInterval{
			class: class, startTick: startTick, bTick: bTick, cTick: cTick,
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
			ctx, catalog, epoch, symbol, grid, excursion.startTick, excursion.cTick,
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
	if exhaustion.Status != "INSUFFICIENT_DATA" {
		measured = measured && exhaustion.Status == "MEASURED"
	}

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
	stream := store.NewStream()

	for _, tick := range ticks {
		group := tickMeasurements[tick]
		if len(group) == 0 {
			continue
		}

		observed := strategy.ChannelsFrom(group...)
		deformations := stream.Deform(observed.Raw)
		excited := observed.Excite(deformations)
		lit := grid.LitRegion(excited)
		if len(lit) == 0 {
			continue
		}

		inExcursion := false
		for _, excursion := range excursions {
			if tick >= excursion.startTick && tick <= excursion.cTick {
				inExcursion = true
				break
			}
		}
		if !inExcursion {
			result[fmt.Sprintf("R%d", lit[0])]++
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
) (map[int64]string, error) {
	filter := iceberg.NewAnd(
		iceberg.EqualTo(iceberg.Reference("label"), symbol),
		iceberg.NewAnd(
			iceberg.GreaterThanEqual(iceberg.Reference("tick"), startTick),
			iceberg.LessThanEqual(iceberg.Reference("tick"), endTick),
		),
	)

	grouped := make(map[int64][]*data.Measurement)
	order := make([]int64, 0)
	seen := make(map[int64]struct{})

	for measurement, err := range catalog.Scan(ctx, tables.Measurements, epoch, filter, 0) {
		if err != nil {
			return nil, err
		}
		if measurement == nil {
			continue
		}
		if _, ok := seen[measurement.Tick]; !ok {
			seen[measurement.Tick] = struct{}{}
			order = append(order, measurement.Tick)
		}
		grouped[measurement.Tick] = append(grouped[measurement.Tick], measurement)
	}
	sort.Slice(order, func(i, j int) bool { return order[i] < order[j] })

	stream := store.NewStream()
	result := make(map[int64]string)
	for _, tick := range order {
		observed := strategy.ChannelsFrom(grouped[tick]...)
		deformations := stream.Deform(observed.Raw)
		excited := observed.Excite(deformations)
		lit := grid.LitRegion(excited)
		if len(lit) > 0 {
			result[tick] = fmt.Sprintf("R%d", lit[0])
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

func detectInMemory(
	ctx context.Context,
	catalog *tables.Catalog,
	epoch int64,
	symbol string,
) ([]*data.Measurement, error) {
	parts := strings.Split(symbol, "/")
	base, quote := "BTC", "USD"
	if len(parts) == 2 {
		base, quote = parts[0], parts[1]
	}

	normalizer := spot.NewNormalizer()
	normalizer.Update(&spot.AssetsManagerUpdate{
		NewAssets: map[string]spot.AssetInfo{
			base:  {AltName: base},
			quote: {AltName: quote},
		},
		NewPairs: map[string]spot.AssetPair{
			symbol: {
				WSName:        symbol,
				Base:          base,
				Quote:         quote,
				LotDecimals:   8,
				LotMultiplier: 1,
			},
		},
	})

	price := broker.NewPrice(ctx, nil, nil, nil, normalizer)
	price.SetFee(symbol, kraken.TradeVolumeFee{
		Fee: decimal.NewFromFloat64(0.26),
	})
	price.SetReferenceCash(decimal.NewFromFloat64(10000))

	storeTee := hindsight.NewStoreTee(ctx, "auditDetectorTee")
	storeTee.Transition(runtime.READY)

	detector := strategy.NewDetector(ctx, storeTee, price)

	tradesSeq := catalog.TradesForSymbol(ctx, symbol, epoch)
	if err := detector.Scan(tradesSeq); err != nil {
		return nil, err
	}

	var detections []*data.Measurement
	for {
		ptr := storeTee.Next()
		if ptr == nil {
			break
		}
		m := data.To[*data.Measurement](ptr)
		if m != nil {
			detections = append(detections, m)
		}
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
