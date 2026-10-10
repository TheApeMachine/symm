// Scratch read-only: inter-trade dt distribution from stored cvd trade_rate.
package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"strconv"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/hindsight/tables"
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
	buckets := map[string]int{}
	total, nonMicro := 0, 0

	for m, err := range catalog.Timeline(ctx, epoch, "", 0, 0, "cvd") {
		if err != nil {
			panic(err)
		}

		for entry := range m.Read() {
			if entry == nil || entry.Metric == nil || entry.Key != "trade_rate" {
				continue
			}

			dt := 1 / entry.Metric.Raw
			micros := dt * 1e6
			total++

			if math.Abs(micros-math.Round(micros)) > 1e-6*math.Max(1, micros) {
				nonMicro++
			}

			switch {
			case micros < 1.5:
				buckets["a 1us"]++
			case micros < 4.5:
				buckets["b 2-4us"]++
			case micros < 1000:
				buckets["c 5us-1ms"]++
			case micros < 1e6:
				buckets["d 1ms-1s"]++
			default:
				buckets["e >=1s"]++
			}
		}
	}

	fmt.Println("trade dt samples", total, "not whole microseconds:", nonMicro)

	for _, k := range []string{"a 1us", "b 2-4us", "c 5us-1ms", "d 1ms-1s", "e >=1s"} {
		fmt.Printf("  %-10s %d\n", k, buckets[k])
	}
}
