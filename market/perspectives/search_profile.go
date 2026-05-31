package perspectives

import (
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"time"
)

const profileSampleLimit = 256

type CategoryStat struct {
	Name    string
	Source  string
	Count   int
	MeanSNR float64
	MaxSNR  float64
	P50SNR  float64
	P75SNR  float64
	P90SNR  float64
}

type SearchProfile struct {
	Categories []CategoryStat
}

type ProfileBuilder struct {
	mu     sync.Mutex
	random *rand.Rand
	stats  map[CategoryType]*categoryAccumulator
}

type categoryAccumulator struct {
	count       int
	meanSNR     float64
	maxSNR      float64
	samples     []float64
	sourceCount map[SourceType]int
}

func NewProfileBuilder(random *rand.Rand) *ProfileBuilder {
	if random == nil {
		random = rand.New(rand.NewSource(time.Now().UnixNano()))
	}

	return &ProfileBuilder{
		random: random,
		stats:  make(map[CategoryType]*categoryAccumulator),
	}
}

func (builder *ProfileBuilder) Record(measurement Measurement) {
	if measurement.Category == CategoryTypeNone {
		return
	}

	builder.mu.Lock()
	defer builder.mu.Unlock()

	accumulator := builder.stats[measurement.Category]

	if accumulator == nil {
		accumulator = &categoryAccumulator{sourceCount: make(map[SourceType]int)}
		builder.stats[measurement.Category] = accumulator
	}

	accumulator.record(measurement, builder.random)
}

func (builder *ProfileBuilder) Profile() SearchProfile {
	builder.mu.Lock()
	defer builder.mu.Unlock()

	categories := make([]CategoryStat, 0, len(builder.stats))

	for category, accumulator := range builder.stats {
		if accumulator.count == 0 || accumulator.maxSNR <= 0 {
			continue
		}

		categories = append(categories, accumulator.stat(category))
	}

	sort.Slice(categories, func(leftIndex int, rightIndex int) bool {
		left := categories[leftIndex]
		right := categories[rightIndex]

		if left.Count == right.Count {
			return left.Name < right.Name
		}

		return left.Count > right.Count
	})

	return SearchProfile{Categories: categories}
}

func (profile SearchProfile) Validate() error {
	if len(profile.Categories) == 0 {
		return fmt.Errorf("perspective search profile has no observed categories")
	}

	for _, category := range profile.Categories {
		if _, err := parseCategory(category.Name); err != nil {
			return err
		}
	}

	return nil
}

func (accumulator *categoryAccumulator) record(
	measurement Measurement,
	random *rand.Rand,
) {
	accumulator.count++
	accumulator.sourceCount[measurement.Source]++

	delta := measurement.SNR - accumulator.meanSNR
	accumulator.meanSNR += delta / float64(accumulator.count)

	if measurement.SNR > accumulator.maxSNR {
		accumulator.maxSNR = measurement.SNR
	}

	if measurement.SNR <= 0 {
		return
	}

	if len(accumulator.samples) < profileSampleLimit {
		accumulator.samples = append(accumulator.samples, measurement.SNR)

		return
	}

	sampleIndex := random.Intn(accumulator.count)

	if sampleIndex < profileSampleLimit {
		accumulator.samples[sampleIndex] = measurement.SNR
	}
}

func (accumulator *categoryAccumulator) stat(category CategoryType) CategoryStat {
	samples := append([]float64(nil), accumulator.samples...)
	sort.Float64s(samples)

	return CategoryStat{
		Name:    category.String(),
		Source:  accumulator.primarySource().String(),
		Count:   accumulator.count,
		MeanSNR: accumulator.meanSNR,
		MaxSNR:  accumulator.maxSNR,
		P50SNR:  quantile(samples, 0.50),
		P75SNR:  quantile(samples, 0.75),
		P90SNR:  quantile(samples, 0.90),
	}
}

func (accumulator *categoryAccumulator) primarySource() SourceType {
	bestSource := SourceNone
	bestCount := 0

	for source, count := range accumulator.sourceCount {
		if count > bestCount {
			bestSource = source
			bestCount = count

			continue
		}

		if count == bestCount && count > 0 && source.String() < bestSource.String() {
			bestSource = source
		}
	}

	return bestSource
}

func quantile(sorted []float64, fraction float64) float64 {
	if len(sorted) == 0 {
		return 0
	}

	index := int(float64(len(sorted)-1) * fraction)

	if index < 0 {
		index = 0
	}

	if index >= len(sorted) {
		index = len(sorted) - 1
	}

	return sorted[index]
}
