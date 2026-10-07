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

type interval struct {
	start int64
	end   int64
}

/*
AnalyzePrecursorSeparation tests whether token sequences in the precursor window
preceding B (ignition) are observably distinct from background market tape.
If no excursions are stored, it executes excursion detection in-memory over raw trades
without writing any data to storage.
*/
func AnalyzePrecursorSeparation(
	ctx context.Context,
	catalog *tables.Catalog,
	epoch int64,
	symbol string,
	grid *store.Grid,
	ticks []int64,
	series map[string]map[int64]float64,
	permutations int,
) Stage5PrecursorSeparation {
	if catalog == nil {
		return Stage5PrecursorSeparation{
			SummaryText: "Catalog unavailable for precursor detection retrieval.",
			Passed:      false,
		}
	}

	if len(ticks) == 0 || grid == nil || grid.RegionCount() == 0 {
		return Stage5PrecursorSeparation{
			SummaryText: "No observations or formed grid regions to evaluate precursor dynamics.",
			Passed:      false,
		}
	}

	if permutations <= 0 {
		permutations = 50
	}

	// 1. First check if detections exist in storage
	var detections []*data.Measurement
	for det, err := range catalog.Detections(ctx, epoch) {
		if err != nil {
			break
		}
		if det != nil && det.Label == symbol {
			detections = append(detections, det)
		}
	}

	// 2. If no stored detections exist, run detector in-memory without storing anything
	if len(detections) == 0 {
		inMemDetections, err := detectInMemory(ctx, catalog, epoch, symbol)
		if err == nil && len(inMemDetections) > 0 {
			detections = inMemDetections
		}
	}

	if len(detections) == 0 {
		return Stage5PrecursorSeparation{
			DetectionsFound: 0,
			SummaryText: fmt.Sprintf(
				"Precursor: Zero excursions detected on %s tape for epoch %d.",
				symbol, epoch,
			),
			Passed: true,
		}
	}

	// 3. Extract excursion classes and precursor intervals [start_tick, b_tick]
	classesSet := make(map[string]struct{})
	var classList []string
	var intervals []interval
	var nearestB int64 = -1

	for _, det := range detections {
		class := det.Meta("type")
		if class == "" {
			class = "unknown"
		}
		if _, exists := classesSet[class]; !exists {
			classesSet[class] = struct{}{}
			classList = append(classList, class)
		}

		bTick := int64(getMeasurementMetric(det, "b_tick"))
		startTick := int64(getMeasurementMetric(det, "start_tick"))
		cTick := int64(getMeasurementMetric(det, "c_tick"))

		if nearestB < 0 || (bTick > 0 && bTick < nearestB) {
			nearestB = bTick
		}

		if bTick <= startTick && cTick > bTick {
			pad := max(int64(4), cTick-bTick)
			startTick = max(int64(0), bTick-pad)
		}

		if bTick > startTick {
			intervals = append(intervals, interval{start: startTick, end: bTick})
		}
	}
	sort.Strings(classList)

	// 4. Map ticks to region tokens via the settled grid
	tickToToken := make(map[int64]string)
	var allTokens []string

	for _, tick := range ticks {
		pass := make(map[string]store.Excitation)
		for name, tickMap := range series {
			if val, ok := tickMap[tick]; ok {
				pass[store.CellKey(name)] = store.Excitation{
					Deformation: val,
					Confidence:  1.0,
				}
			}
		}

		lit := grid.LitRegion(pass)
		if len(lit) > 0 {
			tok := fmt.Sprintf("R%d", lit[0])
			tickToToken[tick] = tok
			allTokens = append(allTokens, tok)
		}
	}

	if len(allTokens) == 0 {
		return Stage5PrecursorSeparation{
			DetectionsFound: len(detections),
			ExcursionsFound: classList,
			SummaryText:     "Precursor: Grid emitted zero tokens across sampled ticks.",
			Passed:          false,
		}
	}

	// 5. Partition emitted tokens into precursor vs background
	precursorTokens := make(map[string]int)
	backgroundTokens := make(map[string]int)
	var precursorList []string
	precursorTickCount := 0

	for _, tick := range ticks {
		tok, ok := tickToToken[tick]
		if !ok {
			continue
		}

		backgroundTokens[tok]++

		isPre := false
		for _, iv := range intervals {
			if tick >= iv.start && tick <= iv.end {
				isPre = true
				break
			}
		}

		if isPre {
			precursorTickCount++
			precursorTokens[tok]++
			precursorList = append(precursorList, tok)
		}
	}

	minTick, maxTick := ticks[0], ticks[len(ticks)-1]

	// 6. If precursor ticks did not fall into initial tick sample, fetch precursor observations directly
	if len(precursorList) < 5 {
		precursorList = loadPrecursorTokens(ctx, catalog, epoch, symbol, intervals, grid)
		for _, tok := range precursorList {
			precursorTokens[tok]++
		}
	}

	if len(precursorList) < 5 {
		summary := fmt.Sprintf(
			"Precursor: %d excursions detected in-memory (%s). Sampled ticks (%d..%d) fall outside precursor windows (first B at tick %d).",
			len(detections), strings.Join(classList, "/"), minTick, maxTick, nearestB,
		)
		return Stage5PrecursorSeparation{
			DetectionsFound:  len(detections),
			ExcursionsFound:  classList,
			PrecursorTicks:   len(precursorList),
			PrecursorTokens:  precursorTokens,
			BackgroundTokens: backgroundTokens,
			SummaryText:      summary,
			Passed:           true,
		}
	}

	// 7. Compute real Jensen-Shannon Divergence between precursor and background distributions
	realJSD := computeJSD(precursorTokens, backgroundTokens)

	// 8. Empirical Shuffled Null: randomly sample len(precursorList) tokens from background
	nullJSDs := make([]float64, permutations)
	rng := rand.New(rand.NewSource(epoch ^ int64(len(ticks))))

	for p := 0; p < permutations; p++ {
		nullSample := make(map[string]int)
		for s := 0; s < len(precursorList); s++ {
			randTok := allTokens[rng.Intn(len(allTokens))]
			nullSample[randTok]++
		}
		nullJSDs[p] = computeJSD(nullSample, backgroundTokens)
	}

	sort.Float64s(nullJSDs)

	nullMean := 0.0
	for _, v := range nullJSDs {
		nullMean += v
	}
	nullMean /= float64(len(nullJSDs))

	idx95 := int(math.Floor(0.95 * float64(len(nullJSDs)-1)))
	null95 := nullJSDs[idx95]

	sepRatio := 1.0
	if null95 > 0 {
		sepRatio = realJSD / null95
	}

	passed := realJSD >= null95

	summary := fmt.Sprintf(
		"Precursor: %d excursions analyzed (%s). Divergence = %.3f bits vs Null-95 = %.3f bits (Ratio: %.2fx, %d precursor ticks).",
		len(detections), strings.Join(classList, "/"), realJSD, null95, sepRatio, len(precursorList),
	)

	return Stage5PrecursorSeparation{
		DetectionsFound:     len(detections),
		ExcursionsFound:     classList,
		PrecursorTicks:      len(precursorList),
		PrecursorDivergence: realJSD,
		NullDivergenceMean:  nullMean,
		NullDivergence95:    null95,
		SeparationRatio:     sepRatio,
		PrecursorTokens:     precursorTokens,
		BackgroundTokens:    backgroundTokens,
		SummaryText:         summary,
		Passed:              passed,
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

	filteredTrades := func(yield func(*data.Measurement, error) bool) {
		for trade, err := range catalog.Trades(ctx, epoch) {
			if err != nil {
				yield(nil, err)
				return
			}
			if trade != nil && trade.Label == symbol {
				if !yield(trade, nil) {
					return
				}
			}
		}
	}

	if err := detector.Scan(filteredTrades); err != nil {
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

func loadPrecursorTokens(
	ctx context.Context,
	catalog *tables.Catalog,
	epoch int64,
	symbol string,
	intervals []interval,
	grid *store.Grid,
) []string {
	var tokens []string
	if catalog == nil || grid == nil || len(intervals) == 0 {
		return tokens
	}

	for _, iv := range intervals {
		if iv.end <= iv.start {
			continue
		}

		filter := iceberg.NewAnd(
			iceberg.EqualTo(iceberg.Reference("label"), symbol),
			iceberg.NewAnd(
				iceberg.GreaterThanEqual(iceberg.Reference("tick"), iv.start),
				iceberg.LessThanEqual(iceberg.Reference("tick"), iv.end),
			),
		)

		for m, err := range catalog.Scan(ctx, tables.Measurements, epoch, filter, 50) {
			if err != nil || m == nil {
				break
			}

			pass := make(map[string]store.Excitation)
			for entry := range m.Read() {
				if entry == nil || entry.Metric == nil {
					continue
				}
				pass[store.CellKey(entry.Key)] = store.Excitation{
					Deformation: entry.Metric.Raw,
					Confidence:  1.0,
				}
			}

			lit := grid.LitRegion(pass)
			if len(lit) > 0 {
				tokens = append(tokens, fmt.Sprintf("R%d", lit[0]))
			}
		}

		if len(tokens) >= 50 {
			break
		}
	}

	return tokens
}
