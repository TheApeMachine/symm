/*
Package analyze turns a signal's raw JSONL dump into a plain-language verdict on
whether the signal is behaving sensibly. It is deliberately schema-agnostic: it
reads each line as a generic object and runs a diagnostic battery over whatever
numeric and categorical fields it finds, so it works on every signal's bespoke
record without special-casing any of them.

The battery is the same read we did by hand on pumpdump — distribution,
tick-to-tick shape (autocorrelation, mean-crossing/flicker rate, jump rate),
how much of the time the series sits at its own baseline, and how often a
categorical field flips — distilled into a verdict per field
(DEAD / FLICKERING / FLAT / NOISY / HEALTHY) plus a one-line headline.
*/
package analyze

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bytedance/sonic"
)

// DefaultMaxRows caps how many lines a single analysis reads, keeping memory and
// latency bounded on multi-million-row dumps. Order is preserved up to the cap so
// the temporal metrics stay meaningful.
const DefaultMaxRows = 500_000

const (
	verdictEmpty     = "EMPTY"
	verdictDead      = "DEAD"
	verdictFlicker   = "FLICKERING"
	verdictFlat      = "FLAT"
	verdictNoisy     = "NOISY"
	verdictHealthy   = "HEALTHY"
	verdictUnstable  = "UNSTABLE"
	verdictConstant  = "CONSTANT"
	verdictOK        = "OK"
	kindNumeric      = "numeric"
	kindCategorical  = "categorical"
	histogramBins    = 24
	topCategoryLimit = 6
)

// Thresholds for the verdict heuristics. Named so the reasoning is legible.
const (
	constantSpread     = 1e-9 // max-min below this ⇒ effectively constant
	deadDispersion     = 1e-4 // std / |mean| below this ⇒ no usable dynamic range
	flickerCrossing    = 0.40 // mean-crossings per step above this ⇒ flips constantly
	flickerAutocorr    = 0.20 // lag-1 autocorrelation below this ⇒ no memory
	smoothAutocorr     = 0.50 // lag-1 autocorrelation above this ⇒ smooth/persistent
	flatExcursionRatio = 1.05 // p99 / median below this ⇒ barely any peaks
	unstableSwitchRate = 0.50 // categorical changes per step above this ⇒ unstable
)

/*
Bin is one histogram bucket over a numeric field's range.
*/
type Bin struct {
	Lo    float64 `json:"lo"`
	Hi    float64 `json:"hi"`
	Count int     `json:"count"`
}

/*
CategoryCount is one (value, frequency) entry for a categorical field.
*/
type CategoryCount struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

/*
FieldReport is the full diagnostic for one field of the dump.
*/
type FieldReport struct {
	Name  string `json:"name"`
	Kind  string `json:"kind"`
	Count int    `json:"count"`

	// Numeric battery (zero-valued and meaningless for categorical fields).
	Min               float64 `json:"min"`
	P1                float64 `json:"p1"`
	Median            float64 `json:"median"`
	Mean              float64 `json:"mean"`
	P90               float64 `json:"p90"`
	P99               float64 `json:"p99"`
	Max               float64 `json:"max"`
	Std               float64 `json:"std"`
	Lag1Autocorr      float64 `json:"lag1_autocorr"`
	MeanCrossingRate  float64 `json:"mean_crossing_rate"`
	JumpRate          float64 `json:"jump_rate"`
	BaselineOccupancy float64 `json:"baseline_occupancy"`
	ZeroFraction      float64 `json:"zero_fraction"`
	Histogram         []Bin   `json:"histogram,omitempty"`

	// Categorical battery (zero-valued for numeric fields).
	Distinct   int             `json:"distinct"`
	SwitchRate float64         `json:"switch_rate"`
	Top        []CategoryCount `json:"top,omitempty"`

	Verdict string   `json:"verdict"`
	Notes   []string `json:"notes,omitempty"`
}

/*
Report is the analysis of one signal's dump file.
*/
type Report struct {
	Signal      string        `json:"signal"`
	File        string        `json:"file"`
	Rows        int           `json:"rows"`
	TotalRows   int           `json:"total_rows,omitempty"`
	Skipped     int           `json:"skipped"`
	Truncated   bool          `json:"truncated"`
	Live        bool          `json:"live,omitempty"`
	Fields      []FieldReport `json:"fields"`
	Headline    string        `json:"headline"`
	GeneratedAt string        `json:"generated_at"`
}

type accumulator struct {
	name    string
	nums    []float64
	strs    []string
	numeric int
	textual int
}

func sortFieldAccumulators(accumulators []*accumulator) {
	sort.Slice(accumulators, func(i, j int) bool {
		return accumulators[i].name < accumulators[j].name
	})
}

/*
AnalyzeFile reads a raw JSONL dump and returns its diagnostic report. signal is a
display label (typically the signal name). maxRows <= 0 uses DefaultMaxRows.
*/
func AnalyzeFile(signal, path string, maxRows int) (*Report, error) {
	if maxRows <= 0 {
		maxRows = DefaultMaxRows
	}

	file, err := os.Open(path)

	if err != nil {
		return nil, fmt.Errorf("analyze: open %q: %w", path, err)
	}

	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)

	accumulators := map[string]*accumulator{}
	rows := 0
	skipped := 0
	truncated := false

	for scanner.Scan() {
		if rows >= maxRows {
			truncated = true

			break
		}

		line := strings.TrimSpace(scanner.Text())

		if line == "" {
			continue
		}

		object := map[string]any{}

		if err := sonic.Unmarshal([]byte(line), &object); err != nil {
			skipped++

			continue
		}

		rows++

		for key, value := range object {
			acc := accumulators[key]

			if acc == nil {
				acc = &accumulator{name: key}
				accumulators[key] = acc
			}

			acc.observe(value)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("analyze: scan %q: %w", path, err)
	}

	ordered := make([]*accumulator, 0, len(accumulators))

	for _, acc := range accumulators {
		ordered = append(ordered, acc)
	}

	sortFieldAccumulators(ordered)

	fields := make([]FieldReport, 0, len(ordered))

	for _, acc := range ordered {
		fields = append(fields, acc.report())
	}

	report := &Report{
		Signal:      signal,
		File:        path,
		Rows:        rows,
		TotalRows:   rows,
		Skipped:     skipped,
		Truncated:   truncated,
		Fields:      fields,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}

	report.Headline = headline(report)

	return report, nil
}

func (acc *accumulator) observe(value any) {
	switch typed := value.(type) {
	case float64:
		acc.nums = append(acc.nums, typed)
		acc.numeric++
	case bool:
		acc.strs = append(acc.strs, strconv.FormatBool(typed))
		acc.textual++
	case string:
		// Numbers occasionally arrive as strings; treat them as numeric when they parse.
		if parsed, err := strconv.ParseFloat(typed, 64); err == nil {
			acc.nums = append(acc.nums, parsed)
			acc.numeric++

			return
		}

		acc.strs = append(acc.strs, typed)
		acc.textual++
	case nil:
		// Missing value: ignore so it does not distort the series.
	default:
		acc.strs = append(acc.strs, fmt.Sprintf("%v", typed))
		acc.textual++
	}
}

func (acc *accumulator) report() FieldReport {
	// A field is numeric when its readings are predominantly numbers.
	if acc.numeric >= acc.textual && acc.numeric > 0 {
		return numericReport(acc.name, acc.nums)
	}

	return categoricalReport(acc.name, acc.strs)
}

func numericReport(name string, values []float64) FieldReport {
	values = finiteValues(values)
	report := FieldReport{Name: name, Kind: kindNumeric, Count: len(values)}

	if len(values) == 0 {
		report.Verdict = verdictEmpty

		return report
	}

	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)

	report.Min = sorted[0]
	report.Max = sorted[len(sorted)-1]
	report.P1 = percentile(sorted, 0.01)
	report.Median = percentile(sorted, 0.50)
	report.P90 = percentile(sorted, 0.90)
	report.P99 = percentile(sorted, 0.99)

	mean := sum(values) / float64(len(values))
	report.Mean = mean
	report.Std = std(values, mean)

	report.Lag1Autocorr = lag1Autocorr(values, mean)
	report.MeanCrossingRate = meanCrossingRate(values, mean)
	report.JumpRate = jumpRate(values)
	report.BaselineOccupancy = baselineOccupancy(values, report.Median, report.Std)
	report.ZeroFraction = zeroFraction(values)
	report.Histogram = histogram(sorted)

	report.Verdict, report.Notes = numericVerdict(report)

	return report
}

func finiteValues(values []float64) []float64 {
	finite := make([]float64, 0, len(values))

	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			continue
		}

		finite = append(finite, value)
	}

	return finite
}

func categoricalReport(name string, values []string) FieldReport {
	report := FieldReport{Name: name, Kind: kindCategorical, Count: len(values)}

	if len(values) == 0 {
		report.Verdict = verdictEmpty

		return report
	}

	counts := map[string]int{}

	for _, value := range values {
		counts[value]++
	}

	report.Distinct = len(counts)
	report.SwitchRate = switchRate(values)
	report.Top = topCategories(counts)

	report.Verdict, report.Notes = categoricalVerdict(report)

	return report
}

// ---- numeric statistics --------------------------------------------------------

func sum(values []float64) float64 {
	total := 0.0

	for _, value := range values {
		total += value
	}

	return total
}

func std(values []float64, mean float64) float64 {
	if len(values) < 2 {
		return 0
	}

	acc := 0.0

	for _, value := range values {
		delta := value - mean
		acc += delta * delta
	}

	return math.Sqrt(acc / float64(len(values)))
}

// percentile uses the nearest-rank method on an already-sorted slice.
func percentile(sorted []float64, fraction float64) float64 {
	if len(sorted) == 0 {
		return 0
	}

	if len(sorted) == 1 {
		return sorted[0]
	}

	index := int(math.Round(fraction * float64(len(sorted)-1)))

	if index < 0 {
		index = 0
	}

	if index >= len(sorted) {
		index = len(sorted) - 1
	}

	return sorted[index]
}

// lag1Autocorr is the lag-1 autocorrelation: 1 = smooth/persistent, 0 = white
// noise, negative = oscillating tick-to-tick.
func lag1Autocorr(values []float64, mean float64) float64 {
	if len(values) < 2 {
		return 0
	}

	var numerator, denominator float64

	for index := range values {
		delta := values[index] - mean
		denominator += delta * delta

		if index > 0 {
			numerator += delta * (values[index-1] - mean)
		}
	}

	if denominator == 0 {
		return 0
	}

	return numerator / denominator
}

// meanCrossingRate is the fraction of consecutive steps that straddle the mean —
// a direct measure of how often the series flips from one side of its average to
// the other.
func meanCrossingRate(values []float64, mean float64) float64 {
	if len(values) < 2 {
		return 0
	}

	crossings := 0

	for index := 1; index < len(values); index++ {
		if (values[index-1]-mean)*(values[index]-mean) < 0 {
			crossings++
		}
	}

	return float64(crossings) / float64(len(values)-1)
}

// jumpRate is the fraction of steps whose relative change exceeds 50%.
func jumpRate(values []float64) float64 {
	if len(values) < 2 {
		return 0
	}

	jumps := 0

	for index := 1; index < len(values); index++ {
		previous := values[index-1]

		if previous == 0 {
			continue
		}

		if math.Abs(values[index]-previous)/math.Abs(previous) > 0.5 {
			jumps++
		}
	}

	return float64(jumps) / float64(len(values)-1)
}

// baselineOccupancy is the fraction of readings within a band around the median.
// The band is the larger of 10% of |median| and 10% of the standard deviation, so
// it stays meaningful when the median is near zero.
func baselineOccupancy(values []float64, median, standardDev float64) float64 {
	band := math.Max(0.1*math.Abs(median), 0.1*standardDev)

	if band == 0 {
		// A perfectly flat series sits entirely on its own baseline.
		return 1
	}

	within := 0

	for _, value := range values {
		if math.Abs(value-median) <= band {
			within++
		}
	}

	return float64(within) / float64(len(values))
}

func zeroFraction(values []float64) float64 {
	zeros := 0

	for _, value := range values {
		if value == 0 {
			zeros++
		}
	}

	return float64(zeros) / float64(len(values))
}

func histogram(sorted []float64) []Bin {
	low := sorted[0]
	high := sorted[len(sorted)-1]

	if high <= low {
		return nil
	}

	width := (high - low) / float64(histogramBins)
	bins := make([]Bin, histogramBins)

	for index := range bins {
		bins[index].Lo = low + float64(index)*width
		bins[index].Hi = low + float64(index+1)*width
	}

	for _, value := range sorted {
		index := int((value - low) / width)

		if index >= histogramBins {
			index = histogramBins - 1
		}

		if index < 0 {
			index = 0
		}

		bins[index].Count++
	}

	return bins
}

func switchRate(values []string) float64 {
	if len(values) < 2 {
		return 0
	}

	switches := 0

	for index := 1; index < len(values); index++ {
		if values[index] != values[index-1] {
			switches++
		}
	}

	return float64(switches) / float64(len(values)-1)
}

func topCategories(counts map[string]int) []CategoryCount {
	entries := make([]CategoryCount, 0, len(counts))

	for value, count := range counts {
		entries = append(entries, CategoryCount{Value: value, Count: count})
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Count != entries[j].Count {
			return entries[i].Count > entries[j].Count
		}

		return entries[i].Value < entries[j].Value
	})

	if len(entries) > topCategoryLimit {
		entries = entries[:topCategoryLimit]
	}

	return entries
}

// ---- verdicts ------------------------------------------------------------------

func numericVerdict(report FieldReport) (string, []string) {
	notes := []string{}

	if report.ZeroFraction >= 0.25 {
		notes = append(notes, fmt.Sprintf("%.0f%% of values are exactly 0", report.ZeroFraction*100))
	}

	notes = append(notes, fmt.Sprintf(
		"sits within 10%% of its median %.0f%% of the time",
		report.BaselineOccupancy*100,
	))

	spread := report.Max - report.Min
	dispersion := report.Std / (math.Abs(report.Mean) + 1e-12)

	if spread <= constantSpread || dispersion < deadDispersion {
		return verdictDead, append([]string{
			fmt.Sprintf("near-constant at %.6g — no usable dynamic range", report.Median),
		}, notes...)
	}

	if report.MeanCrossingRate > flickerCrossing && report.Lag1Autocorr < flickerAutocorr {
		return verdictFlicker, append([]string{
			fmt.Sprintf(
				"flips across its mean %.0f%% of ticks with autocorrelation %.2f — reads as noise, not signal",
				report.MeanCrossingRate*100, report.Lag1Autocorr,
			),
		}, notes...)
	}

	excursion := report.P99 / nonZero(report.Median)

	if report.Lag1Autocorr >= smoothAutocorr {
		if math.Abs(excursion) < flatExcursionRatio {
			return verdictFlat, append([]string{
				fmt.Sprintf(
					"smooth (autocorrelation %.2f) but barely moves — p99 is only %.2f× the median",
					report.Lag1Autocorr, excursion,
				),
			}, notes...)
		}

		return verdictHealthy, append([]string{
			fmt.Sprintf(
				"steady baseline with real excursions — autocorrelation %.2f, peaks to %.2f× the median",
				report.Lag1Autocorr, excursion,
			),
		}, notes...)
	}

	return verdictNoisy, append([]string{
		fmt.Sprintf(
			"some structure but weak persistence (autocorrelation %.2f)",
			report.Lag1Autocorr,
		),
	}, notes...)
}

func categoricalVerdict(report FieldReport) (string, []string) {
	if report.Distinct <= 1 {
		value := ""

		if len(report.Top) > 0 {
			value = report.Top[0].Value
		}

		return verdictConstant, []string{fmt.Sprintf("always %q — carries no information", value)}
	}

	if report.SwitchRate > unstableSwitchRate {
		return verdictUnstable, []string{fmt.Sprintf(
			"changes value %.0f%% of ticks across %d categories — unstable",
			report.SwitchRate*100, report.Distinct,
		)}
	}

	return verdictOK, []string{fmt.Sprintf(
		"%d categories, switches %.0f%% of ticks",
		report.Distinct, report.SwitchRate*100,
	)}
}

func headline(report *Report) string {
	if report.Rows == 0 {
		return "no rows to analyze"
	}

	counts := map[string]int{}

	for _, field := range report.Fields {
		counts[report.bucket(field.Verdict)]++
	}

	rowLabel := fmt.Sprintf("%d rows", report.Rows)

	if report.Live && report.TotalRows > report.Rows {
		rowLabel = fmt.Sprintf(
			"%d rows (live window of %d total)",
			report.Rows, report.TotalRows,
		)
	}

	if report.Truncated && !report.Live {
		rowLabel = fmt.Sprintf("%d rows (truncated)", report.Rows)
	}

	return fmt.Sprintf(
		"%s, %d fields — %d healthy, %d flat, %d flickering, %d dead",
		rowLabel, len(report.Fields),
		counts[verdictHealthy], counts[verdictFlat],
		counts[verdictFlicker]+counts[verdictUnstable], counts[verdictDead]+counts[verdictConstant],
	)
}

func (report *Report) bucket(verdict string) string {
	switch verdict {
	case verdictHealthy, verdictOK:
		return verdictHealthy
	case verdictFlat, verdictNoisy:
		return verdictFlat
	case verdictFlicker, verdictUnstable:
		return verdictFlicker
	case verdictDead, verdictConstant, verdictEmpty:
		return verdictDead
	default:
		return verdict
	}
}

func nonZero(value float64) float64 {
	if value == 0 {
		return 1e-12
	}

	return value
}
