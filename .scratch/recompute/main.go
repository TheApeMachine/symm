// Scratch read-only recomputation of audit claims (dot dir: not built by ./...).
package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/system"
)

func main() {
	viper.SetConfigType("yml")
	viper.SetConfigFile("cmd/cfg/config.yml")

	if err := viper.ReadInConfig(); err != nil {
		panic(err)
	}

	system.Cfg = system.NewConfig()
	ctx := context.Background()
	catalog := tables.Open(ctx)
	epoch, _ := strconv.ParseInt(os.Args[1], 10, 64)
	keep, _ := strconv.Atoi(os.Args[2])

	all := []*data.Measurement{}

	for m, err := range catalog.Timeline(ctx, epoch, "", 0, 0, tables.SensorySources...) {
		if err != nil {
			panic(err)
		}

		all = append(all, m)
	}

	tickSet := map[int64]bool{}

	for _, m := range all {
		tickSet[m.Tick] = true
	}

	ticks := []int64{}

	for t := range tickSet {
		ticks = append(ticks, t)
	}

	sort.Slice(ticks, func(i, j int) bool { return ticks[i] < ticks[j] })
	fmt.Println("stored distinct ticks:", len(ticks), "measurements:", len(all))

	if keep > 0 && keep < len(ticks) {
		ticks = ticks[:keep]
	}

	maxTick := ticks[len(ticks)-1]
	ms := []*data.Measurement{}

	for _, m := range all {
		if m.Tick <= maxTick {
			ms = append(ms, m)
		}
	}

	sort.SliceStable(ms, func(i, j int) bool {
		if ms[i].Tick != ms[j].Tick {
			return ms[i].Tick < ms[j].Tick
		}

		return ms[i].SeqIdx < ms[j].SeqIdx
	})

	fmt.Println("audited window ticks:", len(ticks), "measurements:", len(ms), "max tick", maxTick)

	// Stage 0: excitation_mass breaches, per source and overall.
	type acc struct {
		n, neg      int
		min, max    float64
		tiny        int
		sources     map[string]bool
		maxZ        float64
		maxZSymbol  string
		maxZRaw     float64
	}

	stats := map[string]*acc{}
	labels := map[string]bool{}
	sourceLabels := map[string]bool{}
	var maxAbsZ float64
	var maxZWhere string
	hugeZ := 0
	metricTotal := 0

	for _, m := range ms {
		for entry := range m.Read() {
			if entry == nil || entry.Metric == nil {
				continue
			}

			metricTotal++
			name := entry.Metric.Label
			labels[name] = true
			sourceLabels[m.Source+":"+entry.Key] = true
			a := stats[name]

			if a == nil {
				a = &acc{min: math.Inf(1), max: math.Inf(-1), sources: map[string]bool{}}
				stats[name] = a
			}

			v := entry.Metric.Raw
			a.n++
			a.sources[m.Source] = true
			a.min = math.Min(a.min, v)
			a.max = math.Max(a.max, v)

			if v < 0 {
				a.neg++

				if v > -1e-9 {
					a.tiny++
				}
			}

			z := math.Abs(entry.Metric.Standardized)

			if z > maxAbsZ {
				maxAbsZ = z
				maxZWhere = fmt.Sprintf("%s:%s %s raw=%g", m.Source, entry.Key, m.Label, v)
			}

			if z > 1e6 {
				hugeZ++
			}
		}
	}

	for _, name := range []string{"excitation_mass:buy", "excitation_mass:sell"} {
		a := stats[name]
		fmt.Printf("%s n=%d negative=%d (of which > -1e-9: %d) min=%g max=%g sources=%v\n", name, a.n, a.neg, a.tiny, a.min, a.max, a.sources)
	}

	shared := 0

	for name, a := range stats {
		if len(a.sources) > 1 {
			shared++
			_ = name
		}
	}

	fmt.Printf("distinct metric labels=%d, distinct source:key=%d, labels shared by >1 source=%d, metric entries=%d\n", len(labels), len(sourceLabels), shared, metricTotal)
	fmt.Printf("max |z|=%g at %s; entries with |z|>1e6: %d\n", maxAbsZ, maxZWhere, hugeZ)

	// Stage 0.5: drift and inversions, per symbol (audit) vs per symbol+source.
	drifts := []float64{}
	lastBySymbol := map[string]int64{}
	lastBySymbolSource := map[string]int64{}
	invSymbol, invSymbolSource := 0, 0

	for _, m := range ms {
		if m.At.IsZero() {
			continue
		}

		if m.Timestamp > 0 {
			drifts = append(drifts, float64(m.Timestamp-m.At.UnixNano())/1e6)
		}

		at := m.At.UnixNano()

		if last, ok := lastBySymbol[m.Label]; ok && at < last {
			invSymbol++
		}

		lastBySymbol[m.Label] = at
		key := m.Label + "|" + m.Source

		if last, ok := lastBySymbolSource[key]; ok && at < last {
			invSymbolSource++
		}

		lastBySymbolSource[key] = at
	}

	sort.Float64s(drifts)
	sum := 0.0

	for _, d := range drifts {
		sum += d
	}

	neg := 0

	for _, d := range drifts {
		if d < 0 {
			neg++
		}
	}

	fmt.Printf("drift n=%d mean=%.2fms median=%.2fms p95=%.2fms max=%.1fms negative=%d; inversions per symbol=%d, per symbol+source=%d\n",
		len(drifts), sum/float64(len(drifts)), drifts[len(drifts)/2], drifts[int(0.95*float64(len(drifts)-1))], drifts[len(drifts)-1], neg, invSymbol, invSymbolSource)

	// Stage 1: sentiment dead cells.
	for _, key := range []string{"directional_agreement", "return_mad", "magnitude_mad"} {
		values := map[float64]int{}
		count := 0

		for _, m := range ms {
			if m.Source != "sentiment" {
				continue
			}

			for entry := range m.Read() {
				if entry != nil && entry.Key == key {
					values[entry.Metric.Raw]++
					count++
				}
			}
		}

		fmt.Printf("sentiment:%s observations=%d distinct raw values=%d %v\n", key, count, len(values), firstKeys(values))
	}

	// Ticks with more than one symbol.
	symbolsPerTick := map[int64]map[string]bool{}

	for _, m := range ms {
		if symbolsPerTick[m.Tick] == nil {
			symbolsPerTick[m.Tick] = map[string]bool{}
		}

		symbolsPerTick[m.Tick][m.Label] = true
	}

	multi := 0

	for _, set := range symbolsPerTick {
		if len(set) > 1 {
			multi++
		}
	}

	fmt.Printf("ticks with >1 symbol: %d of %d\n", multi, len(symbolsPerTick))

	// V3 artifact check: the stateless grid, the audit's per-(tick,symbol)
	// last-write keying, past half only, nothing future touched.
	grid := store.NewGrid()
	past := map[int64]bool{}

	for _, t := range ticks[:len(ticks)/2] {
		past[t] = true
	}

	token := func(m *data.Measurement) string {
		frame := data.NewMeasurement(m.Epoch, m.Label, "check", m.SeqIdx, m.Tick)
		frame.At = m.At
		frame.From = m.From
		frame.Peers(m)
		frame.Write()

		return string(grid.Observe(frame))
	}

	last := map[string]string{}
	repeatSame := 0

	for _, m := range ms {
		if past[m.Tick] {
			last[fmt.Sprintf("%d|%s", m.Tick, m.Label)] = token(m)
		}
	}

	differs, pastN := 0, 0

	for _, m := range ms {
		if !past[m.Tick] {
			continue
		}

		pastN++
		tok := token(m)

		if tok != last[fmt.Sprintf("%d|%s", m.Tick, m.Label)] {
			differs++
		}

		if tok == token(m) {
			repeatSame++
		}
	}

	fmt.Printf("past measurements=%d, token != last token of same (tick,symbol)=%d, deterministic repeats=%d\n", pastN, differs, repeatSame)
}

func firstKeys(m map[float64]int) []string {
	out := []string{}

	for k, n := range m {
		if len(out) < 4 {
			out = append(out, fmt.Sprintf("%g×%d", k, n))
		}
	}

	return out
}
