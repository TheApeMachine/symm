// Scratch inspector (not part of the build: dot directory). Read-only.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sort"
	"strings"
	"sync"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
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

	switch os.Args[1] {
	case "runs":
		runs, err := catalog.Runs(ctx)

		if err != nil {
			panic(err)
		}

		for i, run := range runs {
			if i >= 15 {
				break
			}

			fmt.Printf("%d %s %s\n", run.Epoch, run.StartedAt.Local().Format("2006-01-02 15:04:05.000 MST"), run.Status)
		}
	case "excursions":
		epoch, _ := strconv.ParseInt(os.Args[2], 10, 64)
		byClass := map[string]int{}
		seen := map[string]int{}
		picks := map[string][]*data.Measurement{}

		for detection, err := range catalog.Excursions(ctx, epoch) {
			if err != nil {
				panic(err)
			}

			class := detection.Meta("type")
			byClass[class]++
			start, b, c, _ := tables.DetectionTicks(detection)
			bp, cp, _ := tables.DetectionPrices(detection)
			seen[fmt.Sprintf("%s|%s|%d|%d|%d|%s|%s", class, detection.Label, start, b, c, bp, cp)]++

			if class != "flat" && len(picks[class]) < 3 {
				picks[class] = append(picks[class], detection)
			}
		}

		dupes := 0

		for _, n := range seen {
			if n > 1 {
				dupes += n - 1
			}
		}

		fmt.Println("stored per class:", byClass, "duplicates:", dupes)

		for class, list := range picks {
			for _, detection := range list {
				_, b, c, _ := tables.DetectionTicks(detection)
				bp, cp, _ := tables.DetectionPrices(detection)
				var tb, tc string
				lo, hi := "", ""
				var loF, hiF float64
				first := true

				for trade, err := range catalog.Trades(ctx, epoch, detection.Label) {
					if err != nil {
						panic(err)
					}

					price := data.Pull(trade.Read("price")).Metric.Exact

					if trade.Tick == b {
						tb = price.String()
					}

					if trade.Tick == c {
						tc = price.String()
					}

					if trade.Tick >= b && trade.Tick <= c {
						f := price.Float64()

						if first || f < loF {
							loF, lo = f, price.String()
						}

						if first || f > hiF {
							hiF, hi = f, price.String()
						}

						first = false
					}
				}

				fmt.Printf("%s %s b=%d c=%d stored bp=%s cp=%s | raw trade@b=%s @c=%s | raw min/max over [b,c]=%s/%s\n",
					class, detection.Label, b, c, bp, cp, tb, tc, lo, hi)
			}
		}
	case "keys":
		keys := []string{}

		for key, err := range catalog.ListBlobs(ctx, "") {
			if err != nil {
				panic(err)
			}

			keys = append(keys, key)
		}

		sort.Strings(keys)
		actions := map[string]int{}
		lengths := map[string]map[int]int{}
		paths := map[string][]string{}

		for _, key := range keys {
			parts := strings.Split(key, "/")
			action := parts[len(parts)-1]
			actions[action]++

			if action != "enter.json" && action != "exit.json" {
				continue
			}

			if lengths[action] == nil {
				lengths[action] = map[int]int{}
			}

			lengths[action][len(parts)-1]++
			paths[key] = parts[:len(parts)-1]
		}

		fmt.Println("total keys:", len(keys))
		fmt.Println("by final segment:", actions)

		for action, dist := range lengths {
			ls := []int{}
			atLeast3 := 0

			for l, n := range dist {
				ls = append(ls, l)

				if l >= 3 {
					atLeast3 += n
				}
			}

			sort.Ints(ls)
			fmt.Printf("%s token-length distribution:", action)

			for _, l := range ls {
				fmt.Printf(" %d:%d", l, dist[l])
			}

			fmt.Printf("  (>=3 tokens: %d)\n", atLeast3)
		}

		// Prefixes from the path start, as Step matches them.
		for depth := 1; depth <= 6; depth++ {
			sets := map[string]map[string]bool{}

			for key, tokens := range paths {
				if len(tokens) < depth {
					continue
				}

				prefix := strings.Join(tokens[:depth], "/")
				action := key[strings.LastIndex(key, "/")+1:]

				if sets[prefix] == nil {
					sets[prefix] = map[string]bool{}
				}

				sets[prefix][action] = true
			}

			unambiguous, enterOnly, exitOnly := 0, 0, 0

			for _, set := range sets {
				if len(set) == 1 {
					unambiguous++

					if set["enter.json"] {
						enterOnly++
					} else {
						exitOnly++
					}
				}
			}

			fmt.Printf("depth %d: %d distinct prefixes, %d unambiguous (%d enter-only, %d exit-only)\n",
				depth, len(sets), unambiguous, enterOnly, exitOnly)
		}

		limit := 8

		for _, key := range keys {
			if limit == 0 {
				break
			}

			if len(paths[key]) >= 3 {
				body, err := catalog.GetBlob(ctx, key)
				fmt.Printf("sample %s -> %s %v\n", key, string(body), err)
				limit--
			}
		}

		for _, key := range keys {
			if _, ok := paths[key]; !ok {
				fmt.Println("non-path key:", key)
			}
		}

		if len(os.Args) > 2 && os.Args[2] == "enters" {
			for key, tokens := range paths {
				if strings.HasSuffix(key, "enter.json") {
					body, _ := catalog.GetBlob(ctx, key)
					fmt.Printf("enter len=%d first=%s %s\n", len(tokens), tokens[0], body)
				}
			}
		}

		if len(os.Args) > 2 && os.Args[2] == "stats" {
			var mu sync.Mutex
			var wg sync.WaitGroup
			sem := make(chan struct{}, 32)
			withGain, withHold, empty, failed := 0, 0, 0, 0
			gains := []float64{}
			holds := []float64{}

			for key := range paths {
				wg.Add(1)
				sem <- struct{}{}

				go func() {
					defer wg.Done()
					defer func() { <-sem }()
					body, err := catalog.GetBlob(ctx, key)
					stats := map[string]float64{}

					if err == nil {
						err = json.Unmarshal(body, &stats)
					}

					mu.Lock()
					defer mu.Unlock()

					if err != nil {
						failed++
						return
					}

					if len(stats) == 0 {
						empty++
					}

					if g, ok := stats["gain"]; ok {
						withGain++
						gains = append(gains, g)
					}

					if h, ok := stats["hold_seconds"]; ok {
						withHold++
						holds = append(holds, h)
					}
				}()
			}

			wg.Wait()
			sort.Float64s(gains)
			sort.Float64s(holds)
			q := func(v []float64, p float64) float64 {
				if len(v) == 0 {
					return 0
				}

				return v[int(p*float64(len(v)-1))]
			}

			fmt.Printf("blobs: gain %d, hold %d, empty {} %d, unreadable %d\n", withGain, withHold, empty, failed)
			fmt.Printf("gain min/p25/median/p75/max: %.5f %.5f %.5f %.5f %.5f\n", q(gains, 0), q(gains, .25), q(gains, .5), q(gains, .75), q(gains, 1))
			fmt.Printf("hold_s min/p25/median/p75/max: %.3f %.3f %.3f %.3f %.3f\n", q(holds, 0), q(holds, .25), q(holds, .5), q(holds, .75), q(holds, 1))
		}
	}
}
