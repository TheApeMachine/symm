// Scratch read-only recompute of stored z-scores under the old and the
// guarded standardizer rule (dot dir: not built by ./...).
package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/system"
)

// welford mirrors nomagique/data.standardizer.step exactly.
type welford struct{ count, mean, m2 float64 }

func (w *welford) step(value float64, guarded bool) (float64, float64) {
	center, scale := w.mean, 0.0

	if w.count > 1 {
		scale = math.Sqrt(w.m2 / (w.count - 1))
	}

	if guarded && (w.count < 9 || scale <= math.Sqrt(2.220446049250313e-16)*math.Max(math.Abs(value), math.Abs(center))) {
		scale = 0
	}

	w.count++
	delta := value - w.mean
	w.mean += delta / w.count
	w.m2 += delta * (value - w.mean)

	return center, scale
}

type row struct {
	tick, seq int64
	raw, z    float64
	source    string
	label     string
	metric    string
}

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
	streams := map[string][]row{}

	for m, err := range catalog.Timeline(ctx, epoch, "", 0, 0, tables.SensorySources...) {
		if err != nil {
			panic(err)
		}

		for entry := range m.Read() {
			if entry == nil || entry.Metric == nil {
				continue
			}

			key := m.Source + "|" + m.Label + "|" + entry.Key
			streams[key] = append(streams[key], row{
				tick: m.Tick, seq: m.SeqIdx, raw: entry.Metric.Raw, z: entry.Metric.Standardized,
				source: m.Source, label: m.Label, metric: entry.Key,
			})
		}
	}

	_ = data.UnitCount

	shown := 0
	metricOver := map[string]int{}
	metricMax := map[string]float64{}
	metricAt := map[string]string{}
	metricCtx := map[string]string{}

	type agg struct {
		n, oldMatch          int
		storedOver, newOver  int
		storedMax, newMax    float64
		storedWhere, newWhere string
		refused              int
		newOverEarly         int
	}

	bySource := map[string]*agg{}
	total, matched, undefinedNew := 0, 0, 0

	for key, rows := range streams {
		sort.SliceStable(rows, func(i, j int) bool {
			if rows[i].tick != rows[j].tick {
				return rows[i].tick < rows[j].tick
			}

			return rows[i].seq < rows[j].seq
		})

		old, guarded := &welford{}, &welford{}
		a := bySource[rows[0].source]

		if a == nil {
			a = &agg{}
			bySource[rows[0].source] = a
		}

		for idx, r := range rows {
			oc, oscale := old.step(r.raw, false)
			gc, gs := guarded.step(r.raw, true)
			oz, gz := 0.0, 0.0

			if oscale > 0 {
				oz = (r.raw - oc) / oscale
			}

			if gs > 0 {
				gz = (r.raw - gc) / gs
			}

			if oscale > 0 && gs == 0 {
				undefinedNew++
				a.refused++
			}

			a.n++
			total++

			if oz == r.z || math.Abs(oz-r.z) <= 1e-9*math.Max(1, math.Abs(r.z)) {
				a.oldMatch++
				matched++
			}

			if math.Abs(r.z) > 1e3 {
				a.storedOver++
			}

			if math.Abs(gz) > 1e3 {
				metricOver[r.source+"|"+r.metric]++

				if math.Abs(gz) > metricMax[r.source+"|"+r.metric] {
					metricMax[r.source+"|"+r.metric] = math.Abs(gz)
					metricAt[r.source+"|"+r.metric] = fmt.Sprintf("%s|%d|%d", r.label, r.tick, r.seq)
					lo := max(0, idx-3)
					ctx := ""

					for _, p := range rows[lo : idx+1] {
						ctx += fmt.Sprintf(" %.4g", p.raw)
					}

					metricCtx[r.source+"|"+r.metric] = fmt.Sprintf("n=%d center=%.4g scale=%.4g raws:%s", idx, gc, gs, ctx)
				}

				a.newOver++

				if idx <= 3 {
					a.newOverEarly++
				}
			}

			if math.Abs(r.z) > a.storedMax {
				a.storedMax = math.Abs(r.z)
				a.storedWhere = fmt.Sprintf("%s #%d raw=%g", key, idx, r.raw)
			}

			if math.Abs(gz) > a.newMax {
				a.newMax = math.Abs(gz)
				a.newWhere = fmt.Sprintf("%s #%d raw=%g", key, idx, r.raw)
			}

			if len(os.Args) > 2 && strings.Contains(","+os.Args[2]+",", ","+key+",") && math.Abs(r.z) > 1e6 {
				lo := max(0, idx-6)
				fmt.Printf("REPRO %s #%d raw=%.17g stored z=%g | old recompute center=%.17g scale=%g z=%g | guarded scale=%g z=%g\n",
					key, idx, r.raw, r.z, oc, oscale, oz, gs, gz)
				shown++

				if shown > 6 {
					os.Exit(0)
				}

				for _, p := range rows[lo:idx] {
					fmt.Printf("   prior raw=%.17g\n", p.raw)
				}
			}
		}
	}

	fmt.Printf("streams=%d entries=%d old-rule recompute matches stored z: %d (%.4f%%)\n", len(streams), total, matched, 100*float64(matched)/float64(total))

	fmt.Printf("entries newly undefined (stored had a z, new rule refuses): %d\n", undefinedNew)

	type mc struct {
		k string
		n int
	}

	ranked := []mc{}

	for k, n := range metricOver {
		ranked = append(ranked, mc{k, n})
	}

	sort.Slice(ranked, func(i, j int) bool { return ranked[i].n > ranked[j].n })

	for i, m := range ranked {
		if i >= 40 {
			break
		}

		fmt.Printf("TOP %-55s >1e3:%5d max=%.3g at %s\n      %s\n", m.k, m.n, metricMax[m.k], metricAt[m.k], metricCtx[m.k])
	}

	sources := []string{}

	for s := range bySource {
		sources = append(sources, s)
	}

	sort.Strings(sources)

	for _, s := range sources {
		a := bySource[s]
		fmt.Printf("%-11s n=%6d match=%6d | stored max|z|=%-9.3g >1e3:%4d | guarded max|z|=%-9.3g >1e3:%4d (<=3 prior:%4d) refused-by-guard:%5d\n",
			s, a.n, a.oldMatch, a.storedMax, a.storedOver, a.newMax, a.newOver, a.newOverEarly, a.refused)
		fmt.Printf("            stored max at %s\n            guarded max at %s\n", a.storedWhere, a.newWhere)
	}
}
