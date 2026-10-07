package audit

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"

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
AnalyzePrecursorSeparation tests the two specific causal hypotheses:
1. Hypothesis A -> B: Profitable ignition precursor vs friction/negative controls and background.
2. Hypothesis B -> C: Late exhaustion precursor vs healthy early holding tape.
Uses identical production tokenization for event and control, strictly excludes excursion windows
from background, and treats absence of evidence as INSUFFICIENT_DATA (never PASS).
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
	if catalog == nil {
		return Stage5PrecursorSeparation{
			SummaryText: "Precursor Separation: INSUFFICIENT_DATA (Catalog unavailable).",
			Passed:      false,
		}
	}

	if len(ticks) == 0 || grid == nil || grid.RegionCount() == 0 {
		return Stage5PrecursorSeparation{
			SummaryText: "Precursor Separation: INSUFFICIENT_DATA (No observations or formed grid regions).",
			Passed:      false,
		}
	}

	if permutations <= 0 {
		permutations = 50
	}

	// 1. Map all ticks to region tokens using the production data flow
	stream := store.NewStream()
	tickTokens := make(map[int64]string, len(ticks))

	for _, tick := range ticks {
		measGroup := tickMeasurements[tick]
		if len(measGroup) == 0 {
			continue
		}

		observed := strategy.ChannelsFrom(measGroup...)
		deforms := stream.Deform(observed.Raw)
		excited := observed.Excite(deforms)

		tok := grid.LitRegion(excited)
		if len(tok) > 0 {
			tickTokens[tick] = fmt.Sprintf("R%d", tok[0])
		}
	}

	// 2. Retrieve excursions from storage or in-memory detector
	var detections []*data.Measurement
	for det, err := range catalog.Detections(ctx, epoch) {
		if err != nil {
			break
		}
		if det != nil && det.Label == symbol {
			detections = append(detections, det)
		}
	}

	if len(detections) == 0 {
		inMemDetections, err := detectInMemory(ctx, catalog, epoch, symbol)
		if err == nil && len(inMemDetections) > 0 {
			detections = inMemDetections
		}
	}

	if len(detections) == 0 {
		return Stage5PrecursorSeparation{
			DetectionsFound: 0,
			SummaryText:     "Precursor Separation: INSUFFICIENT_DATA (Zero excursions detected; absence of evidence is undefined, never PASS).",
			Passed:          false,
			IgnitionHypothesis: PrecursorHypothesis{
				Name:        "A -> B Ignition Precursor",
				Description: "Profitable excursion precursor vs negative/ordinary controls",
				Status:      "INSUFFICIENT_DATA",
				Passed:      false,
			},
			ExhaustionHypothesis: PrecursorHypothesis{
				Name:        "B -> C Exhaustion Precursor",
				Description: "Exhaustion precursor vs healthy holding run",
				Status:      "INSUFFICIENT_DATA",
				Passed:      false,
			},
		}
	}

	// 3. Parse excursion intervals and build strict non-excursion background
	var excursions []excursionInterval
	classList := make([]string, 0)
	classSet := make(map[string]struct{})

	for _, det := range detections {
		class := det.Meta("type")
		if class == "" {
			class = "unknown"
		}
		if _, exists := classSet[class]; !exists {
			classSet[class] = struct{}{}
			classList = append(classList, class)
		}

		bTick := int64(getMeasurementMetric(det, "b_tick"))
		startTick := int64(getMeasurementMetric(det, "start_tick"))
		cTick := int64(getMeasurementMetric(det, "c_tick"))

		if bTick <= startTick && cTick > bTick {
			pad := max(int64(4), cTick-bTick)
			startTick = max(int64(0), bTick-pad)
		}

		if cTick <= bTick {
			cTick = bTick + max(int64(4), bTick-startTick)
		}

		excursions = append(excursions, excursionInterval{
			class:     class,
			startTick: startTick,
			bTick:     bTick,
			cTick:     cTick,
		})
	}
	sort.Strings(classList)

	// Build background tokens strictly excluding all [startTick, cTick] windows
	backgroundTokens := make(map[string]int)
	for _, tick := range ticks {
		inExcursion := false
		for _, ex := range excursions {
			if tick >= ex.startTick && tick <= ex.cTick {
				inExcursion = true
				break
			}
		}

		if !inExcursion {
			if tok, ok := tickTokens[tick]; ok {
				backgroundTokens[tok]++
			}
		}
	}

	// 4. Test Hypothesis 1: A -> B Ignition Precursor
	ignitionTokens := make(map[string]int)
	ignitionControlTokens := make(map[string]int)

	ignitionTokenCount := 0
	ignitionControlCount := 0

	for _, ex := range excursions {
		isProfitable := ex.class == "up" || ex.class == "down"

		for t := ex.startTick; t <= ex.bTick; t++ {
			tok, ok := tickTokens[t]
			if !ok {
				continue
			}

			if isProfitable {
				ignitionTokens[tok]++
				ignitionTokenCount++
			} else {
				ignitionControlTokens[tok]++
				ignitionControlCount++
			}
		}
	}

	// If control count is low, add from disjoint background
	if ignitionControlCount < 20 {
		for tok, count := range backgroundTokens {
			ignitionControlTokens[tok] += count
			ignitionControlCount += count
		}
	}

	ignHypothesis := evaluateHypothesis(
		"A -> B Ignition Precursor",
		"Profitable ignition precursor [start, B] vs negative controls and background",
		ignitionTokens,
		ignitionControlTokens,
		ignitionTokenCount,
		ignitionControlCount,
		permutations,
	)

	// 5. Test Hypothesis 2: B -> C Exhaustion Precursor
	exhaustionTokens := make(map[string]int)
	holdingTokens := make(map[string]int)

	exhaustionCount := 0
	holdingCount := 0

	for _, ex := range excursions {
		if ex.class != "up" && ex.class != "down" {
			continue
		}

		span := ex.cTick - ex.bTick
		if span <= 1 {
			continue
		}

		midTick := ex.bTick + span/2

		for t := ex.bTick; t <= midTick; t++ {
			if tok, ok := tickTokens[t]; ok {
				holdingTokens[tok]++
				holdingCount++
			}
		}

		for t := midTick + 1; t <= ex.cTick; t++ {
			if tok, ok := tickTokens[t]; ok {
				exhaustionTokens[tok]++
				exhaustionCount++
			}
		}
	}

	exhHypothesis := evaluateHypothesis(
		"B -> C Exhaustion Precursor",
		"Late exhaustion run [mid, C] vs early holding state [B, mid]",
		exhaustionTokens,
		holdingTokens,
		exhaustionCount,
		holdingCount,
		permutations,
	)

	passed := ignHypothesis.Passed && (exhHypothesis.Status == "INSUFFICIENT_DATA" || exhHypothesis.Passed)

	summary := fmt.Sprintf(
		"Precursor: %d excursions (%s). Hypothesis A->B (Ignition): %s (JSD=%.3f bits vs Null-95=%.3f, N=%d). "+
			"Hypothesis B->C (Exhaustion): %s (JSD=%.3f bits, N=%d). Background control = %d tokens.",
		len(detections), strings.Join(classList, "/"),
		ignHypothesis.Status, ignHypothesis.DivergenceBits, ignHypothesis.NullDivergence95, ignHypothesis.EventTokenCount,
		exhHypothesis.Status, exhHypothesis.DivergenceBits, exhHypothesis.EventTokenCount,
		len(backgroundTokens),
	)

	return Stage5PrecursorSeparation{
		DetectionsFound:      len(detections),
		ExcursionsFound:      classList,
		IgnitionHypothesis:   ignHypothesis,
		ExhaustionHypothesis: exhHypothesis,
		BackgroundTokens:     backgroundTokens,
		SummaryText:          summary,
		Passed:               passed,
	}
}

func evaluateHypothesis(
	name string,
	desc string,
	eventTokens map[string]int,
	controlTokens map[string]int,
	eventCount int,
	controlCount int,
	permutations int,
) PrecursorHypothesis {
	if eventCount < 5 || controlCount < 5 {
		return PrecursorHypothesis{
			Name:              name,
			Description:       desc,
			EventTokens:       eventTokens,
			ControlTokens:     controlTokens,
			EventTokenCount:   eventCount,
			ControlTokenCount: controlCount,
			Status:            "INSUFFICIENT_DATA",
			Passed:            false,
		}
	}

	realJSD := computeJSD(eventTokens, controlTokens)

	// Permutation Null: pool tokens and shuffle between event and control groups
	var pool []string
	for tok, count := range eventTokens {
		for i := 0; i < count; i++ {
			pool = append(pool, tok)
		}
	}
	for tok, count := range controlTokens {
		for i := 0; i < count; i++ {
			pool = append(pool, tok)
		}
	}

	rng := rand.New(rand.NewSource(1791))
	nullDivergences := make([]float64, permutations)

	for iter := 0; iter < permutations; iter++ {
		shuffled := append([]string(nil), pool...)
		rng.Shuffle(len(shuffled), func(first, second int) {
			shuffled[first], shuffled[second] = shuffled[second], shuffled[first]
		})

		nullEvent := make(map[string]int)
		nullControl := make(map[string]int)

		for i := 0; i < eventCount; i++ {
			nullEvent[shuffled[i]]++
		}
		for i := eventCount; i < len(shuffled); i++ {
			nullControl[shuffled[i]]++
		}

		nullDivergences[iter] = computeJSD(nullEvent, nullControl)
	}

	sort.Float64s(nullDivergences)
	p95Idx := int(float64(len(nullDivergences)) * 0.95)
	null95 := nullDivergences[min(p95Idx, len(nullDivergences)-1)]

	sepRatio := 0.0
	if null95 > 0 {
		sepRatio = realJSD / null95
	}

	passed := realJSD > null95 && sepRatio >= 1.20
	status := "FAIL"
	if passed {
		status = "PASS"
	}

	return PrecursorHypothesis{
		Name:              name,
		Description:       desc,
		EventTokens:       eventTokens,
		ControlTokens:     controlTokens,
		EventTokenCount:   eventCount,
		ControlTokenCount: controlCount,
		DivergenceBits:    realJSD,
		NullDivergence95:  null95,
		SeparationRatio:   sepRatio,
		Status:            status,
		Passed:            passed,
	}
}

/*
detectInMemory runs Detector on raw trades in-memory without Iceberg persistence.
*/
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
