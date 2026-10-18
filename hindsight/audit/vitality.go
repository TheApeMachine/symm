package audit

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/system"
)

/*
MetricVitality tracks running statistics for a single metric stream to determine
whether the metric is actively varying, stagnant, or dead (constant).
*/
type MetricVitality struct {
	Name      string
	Count     float64
	ZeroCount float64
	Min       float64
	Max       float64
	Moments   statistic.Moments
}

func (vitality *MetricVitality) Update(value float64) {
	if vitality.Count == 0 {
		vitality.Min = value
		vitality.Max = value
	}

	vitality.Count++
	vitality.Min = math.Min(vitality.Min, value)
	vitality.Max = math.Max(vitality.Max, value)

	if math.Abs(value) <= core.Resolution {
		vitality.ZeroCount++
	}

	vitality.Moments.Update(value)
}

func (vitality *MetricVitality) Variance() float64 {
	if vitality.Count < 2 {
		return 0
	}

	return vitality.Moments.M2 / (vitality.Count - 1)
}

func (vitality *MetricVitality) ZeroFraction() float64 {
	if vitality.Count == 0 {
		return 0
	}

	return vitality.ZeroCount / vitality.Count
}

/*
symbolVitality holds one symbol's price range and the SIGNALS.md metrics
observed on it. A symbol whose price never changes is not judged.
*/
type symbolVitality struct {
	priceMin   float64
	priceMax   float64
	priceCount int
	metrics    map[string]*MetricVitality
}

/*
VitalityStage checks the signals named in SIGNALS.md on a few symbols whose
price actually moved. A quiet symbol is ignored. A signal passes when its
number changes on those symbols. fee_rate only has to be present.
*/
type VitalityStage struct {
	*runtime.System
	ch         chan *data.Value
	violations []*Violation
	catalog    []catalogMetric
	index      map[string]catalogMetric
	bySymbol   map[string]*symbolVitality
	moved      map[string]float64
	flatZero   map[string]bool
	frozen     map[string]bool
	missing    map[string]bool
	active     []string
	totalTicks int64
}

func NewVitalityStage(ctx context.Context) *VitalityStage {
	catalog := signalCatalog()

	return &VitalityStage{
		System:     runtime.NewSystem(ctx, "audit:vitality"),
		ch:         make(chan *data.Value, 64),
		violations: make([]*Violation, 0),
		catalog:    catalog,
		bySymbol:   make(map[string]*symbolVitality),
		moved:      make(map[string]float64),
		flatZero:   make(map[string]bool),
		frozen:     make(map[string]bool),
		missing:    make(map[string]bool),
	}
}

func (stage *VitalityStage) Channel() chan *data.Value {
	return stage.ch
}

func (stage *VitalityStage) Run() error {
	errnie.Info(fmt.Sprintf("%s: Running audit...", stage.Name()))

	stage.indexCatalog()

	for measurement := range stage.ch {
		if measurement == nil {
			stage.violations = append(stage.violations, NewViolation(
				"unknown",
				"measurement",
				"Nil measurement encountered in tape",
				nil,
				nil,
			))

			continue
		}

		stage.recordMetrics(measurement)
	}

	if len(stage.bySymbol) == 0 {
		stage.violations = append(stage.violations, insufficient("vitality", "no metric series observed"))
	}

	stage.auditVitality()

	return nil
}

func (stage *VitalityStage) indexCatalog() {
	stage.index = make(map[string]catalogMetric, len(stage.catalog))

	for _, metric := range stage.catalog {
		stage.index[metric.key()] = metric
	}
}

func (stage *VitalityStage) symbol(name string) *symbolVitality {
	book, exists := stage.bySymbol[name]

	if !exists {
		book = &symbolVitality{metrics: make(map[string]*MetricVitality)}
		stage.bySymbol[name] = book
	}

	return book
}

/*
recordMetrics keeps price range for every symbol, and keeps a SIGNALS.md
metric only. Everything else on the tape is ignored, which is what stops a
quiet coin from turning one signal into hundreds of series.
*/
func (stage *VitalityStage) recordMetrics(measurement *data.Value) {
	stage.totalTicks++

	if measurement.Scope == "" {
		return
	}

	book := stage.symbol(measurement.Scope)

	for label, val := range floatMetrics(measurement) {
		if isPriceLabel(label) && val > 0 {
			if book.priceCount == 0 {
				book.priceMin = val
				book.priceMax = val
			}

			book.priceMin = math.Min(book.priceMin, val)
			book.priceMax = math.Max(book.priceMax, val)
			book.priceCount++
		}

		key := measurement.Origin + "." + label
		if _, wanted := stage.index[key]; !wanted {
			continue
		}

		stat, exists := book.metrics[key]

		if !exists {
			stat = &MetricVitality{Name: key}
			book.metrics[key] = stat
		}

		stat.Update(val)
	}
}

func isPriceLabel(label string) bool {
	switch label {
	case "best_price:bid", "best_price:ask", "price":
		return true
	default:
		return false
	}
}

/*
auditVitality picks the symbols whose price moved the most, up to the
configured handful, and asks whether each catalog signal changes there.
*/
func (stage *VitalityStage) auditVitality() {
	if len(stage.bySymbol) == 0 {
		return
	}

	stage.active = stage.activeSymbols()

	if len(stage.active) == 0 {
		stage.violations = append(stage.violations, insufficient("vitality", "no symbol's price moved"))
		return
	}

	for _, metric := range stage.catalog {
		stage.judge(metric)
	}
}

func (stage *VitalityStage) activeSymbols() []string {
	type activity struct {
		symbol string
		span   float64
	}

	ranked := make([]activity, 0, len(stage.bySymbol))

	for symbol, book := range stage.bySymbol {
		if book.priceCount < 2 || book.priceMin <= 0 || book.priceMax <= book.priceMin {
			continue
		}

		ranked = append(ranked, activity{
			symbol: symbol,
			span:   (book.priceMax - book.priceMin) / book.priceMin,
		})
	}

	slices.SortFunc(ranked, func(left, right activity) int {
		if left.span == right.span {
			return strings.Compare(left.symbol, right.symbol)
		}

		if left.span > right.span {
			return -1
		}

		return 1
	})

	limit := system.Cfg.Audit.ActiveSymbols

	if limit > 0 && len(ranked) > limit {
		ranked = ranked[:limit]
	}

	symbols := make([]string, len(ranked))

	for idx, row := range ranked {
		symbols[idx] = row.symbol
	}

	return symbols
}

func (stage *VitalityStage) judge(metric catalogMetric) {
	var (
		count   float64
		low     float64
		high    float64
		best    float64
		started bool
	)

	for _, symbol := range stage.active {
		stat := stage.bySymbol[symbol].metrics[metric.key()]

		if stat == nil || stat.Count == 0 {
			continue
		}

		if !started {
			low = stat.Min
			high = stat.Max
			started = true
		} else {
			low = math.Min(low, stat.Min)
			high = math.Max(high, stat.Max)
		}

		count += stat.Count

		if variance := stat.Variance(); variance > best {
			best = variance
		}
	}

	where := strings.Join(stage.active, ", ")
	name := metric.key()

	if !started || count < 2 {
		stage.missing[name] = true
		stage.violations = append(stage.violations, NewViolation(
			name,
			"vitality.missing",
			fmt.Sprintf("%s was not produced on the active symbols (%s)", name, where),
			nil,
			nil,
		))
		return
	}

	if metric.Static {
		if high <= 0 {
			stage.missing[name] = true
			stage.violations = append(stage.violations, NewViolation(
				name,
				"vitality.missing",
				fmt.Sprintf("%s was not present on the active symbols (%s)", name, where),
				nil,
				nil,
			))
			return
		}

		stage.moved[name] = best
		return
	}

	if high > low {
		stage.moved[name] = best
		return
	}

	if math.Abs(low) <= core.Resolution {
		stage.flatZero[name] = true
		stage.violations = append(stage.violations, NewViolation(
			name,
			"vitality.unmeasured",
			fmt.Sprintf("%s was 0 on every sample from the active symbols (%s), so it never measured anything there", name, where),
			nil,
			nil,
		))
		return
	}

	stage.frozen[name] = true
	stage.violations = append(stage.violations, NewViolation(
		name,
		"vitality.dead_metric",
		fmt.Sprintf("%s stayed at %.3f on the active symbols (%s), so it is not a live measurement", name, low, where),
		nil,
		nil,
	))
}

func (stage *VitalityStage) Violations() []*Violation {
	return stage.violations
}

func (stage *VitalityStage) Report() string {
	var build strings.Builder

	for _, violation := range stage.violations {
		build.WriteString(violation.String())
	}

	return build.String()
}

func (stage *VitalityStage) Data() any {
	type varianceRow struct {
		Name     string  `json:"name"`
		Variance float64 `json:"variance"`
	}

	ranked := make([]varianceRow, 0, len(stage.moved))

	for name, variance := range stage.moved {
		ranked = append(ranked, varianceRow{Name: name, Variance: variance})
	}

	slices.SortFunc(ranked, func(left, right varianceRow) int {
		if left.Variance == right.Variance {
			return strings.Compare(left.Name, right.Name)
		}

		if left.Variance > right.Variance {
			return -1
		}

		return 1
	})

	if len(ranked) > 10 {
		ranked = ranked[:10]
	}

	catalogCount := len(stage.catalog)

	return map[string]any{
		"passed":           len(stage.violations) == 0,
		"total_ticks":      stage.totalTicks,
		"catalog_count":    catalogCount,
		"series_tracked":   catalogCount,
		"active_symbols":   stage.active,
		"healthy_count":    len(stage.moved),
		"dead_count":       len(stage.frozen),
		"unmeasured_count": len(stage.flatZero),
		"missing_count":    len(stage.missing),
		"dead_series":      flaggedKeys(stage.frozen),
		"unmeasured":       flaggedKeys(stage.flatZero),
		"missing":          flaggedKeys(stage.missing),
		"top_variance":     ranked,
	}
}

func flaggedKeys(flags map[string]bool) []string {
	keys := make([]string, 0)

	for key, flagged := range flags {
		if flagged {
			keys = append(keys, key)
		}
	}

	slices.Sort(keys)
	return keys
}
