package strategy

import (
	"context"
	"fmt"
	"math/big"
	"math/rand"
	"strconv"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
oracleFee is the 0.26% taker fee testPrice configures, as an exact rate.
*/
var oracleFee = big.NewRat(26, 10000)

/*
oracleClears is round-trip friction priced directly from its definition:
selling at high after buying at low nets more than it cost.
*/
func oracleClears(low, high *big.Rat) bool {
	one := big.NewRat(1, 1)
	net := new(big.Rat).Mul(high, new(big.Rat).Sub(one, oracleFee))
	cost := new(big.Rat).Mul(low, new(big.Rat).Add(one, oracleFee))

	return net.Cmp(cost) > 0
}

/*
argExtreme is the earliest index in [from, to] holding the smallest
(sign < 0) or largest (sign > 0) price, recomputed from the slice.
*/
func argExtreme(prices []*big.Rat, from, to, sign int) int {
	best := from

	for i := from + 1; i <= to; i++ {
		if prices[i].Cmp(prices[best])*sign > 0 {
			best = i
		}
	}

	return best
}

/*
oracleEpisodes derives every episode of a single symbol/epoch tape from the
definitions, rescanning slices instead of keeping running state:

  - legs: a leg from anchor B ends at the first later trade k whose move back
    from the leg's extreme over [B, k-1] clears friction; that extreme is C
    and the next leg's B, and k belongs to the next leg. Before the first leg,
    the tape is undirected until a trade clears friction against the tape's
    extreme over [0, k-1]. The leg still open at the end is not an episode.
  - up_friction: within an undirected or falling stretch, every pair of
    consecutive record lows whose highest trade in between is above the
    earlier low.
  - chop: greedy maximal prefixes whose price range does not clear friction,
    ended by the trade that clears it.
  - flat: maximal runs of two or more equal prices, ended by a price change.

Episodes are rendered "class|b_tick|c_tick" in per-class completion order.
*/
func oracleEpisodes(tape []*data.Measurement) map[string][]string {
	prices := make([]*big.Rat, len(tape))

	for i, trade := range tape {
		prices[i] = data.Pull(trade.Read("price")).Metric.Exact.Rat()
	}

	out := make(map[string][]string)
	add := func(class string, b, c int) {
		if b < c {
			out[class] = append(out[class], fmt.Sprintf("%s|%d|%d", class, tape[b].Tick, tape[c].Tick))
		}
	}

	// falling scans record lows over [from, to) and emits the failed rises
	// between consecutive ones.
	falling := func(from, to int) {
		low := from

		for i := from + 1; i < to; i++ {
			if prices[i].Cmp(prices[low]) < 0 {
				if peak := argExtreme(prices, low, i-1, 1); prices[peak].Cmp(prices[low]) > 0 {
					add(excursionUpShort, low, peak)
				}

				low = i
			}
		}
	}

	n := len(prices)
	dir, b, k := 0, 0, 1

	// Undirected prefix.
	for ; k < n; k++ {
		low := argExtreme(prices, 0, k-1, -1)
		high := argExtreme(prices, 0, k-1, 1)

		if oracleClears(prices[low], prices[k]) {
			falling(0, k)
			dir, b = 1, low
			break
		}

		if oracleClears(prices[k], prices[high]) {
			dir, b = -1, high
			break
		}
	}

	if dir == -1 {
		// The undirected record lows continue into the first down leg.
		k++
		for ; k < n; k++ {
			low := argExtreme(prices, b, k-1, -1)

			if oracleClears(prices[low], prices[k]) {
				break
			}
		}

		falling(0, k)

		if k < n {
			c := argExtreme(prices, b, k-1, -1)
			add(excursionDown, b, c)
			dir, b = 1, c
		}

		k++
	} else if dir == 0 {
		falling(0, n)
	} else {
		k++
	}

	for dir != 0 && k <= n {
		start := k - 1

		for ; k < n; k++ {
			c := argExtreme(prices, b, k-1, dir)

			if dir > 0 && oracleClears(prices[k], prices[c]) {
				break
			}

			if dir < 0 && oracleClears(prices[c], prices[k]) {
				break
			}
		}

		if dir < 0 {
			falling(start, k)
		}

		if k >= n {
			break
		}

		c := argExtreme(prices, b, k-1, dir)
		add(map[int]string{1: excursionUp, -1: excursionDown}[dir], b, c)
		dir, b = -dir, c
		k++
	}

	for s := 0; s < n; {
		j := s + 1

		for ; j < n; j++ {
			low := argExtreme(prices, s, j, -1)
			high := argExtreme(prices, s, j, 1)

			if oracleClears(prices[low], prices[high]) {
				break
			}
		}

		if j < n && prices[argExtreme(prices, s, j-1, -1)].Cmp(prices[argExtreme(prices, s, j-1, 1)]) != 0 {
			add(excursionChop, s, j-1)
		}

		s = j
	}

	for s := 0; s < n; {
		j := s

		for j+1 < n && prices[j+1].Cmp(prices[s]) == 0 {
			j++
		}

		if j+1 < n {
			add(excursionFlat, s, j)
		}

		s = j + 1
	}

	return out
}

/*
episodes renders published detections the way oracleEpisodes does.
*/
func episodes(byClass map[string][]*data.Measurement) map[string][]string {
	out := make(map[string][]string)

	for class, detections := range byClass {
		for _, detection := range detections {
			out[class] = append(out[class], fmt.Sprintf(
				"%s|%d|%d", class, int64(metricRaw(detection, "b_tick")), int64(metricRaw(detection, "c_tick")),
			))
		}
	}

	return out
}

/*
randomTape is a short random walk in cents around 100, with steps on the
scale of the 0.52% round-trip deadband so every class occurs.
*/
func randomTape(t *testing.T, rng *rand.Rand, length int) []*data.Measurement {
	cents := 10000
	prices := make([]string, length)

	for i := range prices {
		cents += rng.Intn(161) - 80
		cents = max(cents, 100)
		prices[i] = strconv.FormatFloat(float64(cents)/100, 'f', 2, 64)
	}

	return priceTape(t, "BTC/USD", prices...)
}

func TestDetector_EveryEpisodeMatchesOracle(t *testing.T) {
	Convey("Given a detector and random short tapes around the friction deadband", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		storeTee := hindsight.NewStoreTee(ctx, "storeTee")
		storeTee.Transition(runtime.READY)
		detector := NewDetector(ctx, storeTee, testPrice(ctx, "BTC/USD"))
		rng := rand.New(rand.NewSource(7))
		seen := make(map[string]int)
		mismatches := 0

		for range 400 {
			tape := randomTape(t, rng, 2+rng.Intn(24))
			So(detector.Scan(tapeSeq(tape)), ShouldBeNil)

			got, want := episodes(drain(storeTee)), oracleEpisodes(tape)

			if fmt.Sprint(got) != fmt.Sprint(want) {
				mismatches++

				if mismatches == 1 {
					So(got, ShouldResemble, want)
				}
			}

			for class, list := range got {
				seen[class] += len(list)
			}
		}

		Convey("Every published episode of every class equals the oracle's", func() {
			So(mismatches, ShouldEqual, 0)

			for _, class := range []string{excursionUp, excursionUpShort, excursionDown, excursionChop, excursionFlat} {
				So(seen[class], ShouldBeGreaterThan, 0)
			}
		})
	})

	Convey("Given a detector and a fixed synthetic tape", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		storeTee := hindsight.NewStoreTee(ctx, "storeTee")
		storeTee.Transition(runtime.READY)
		detector := NewDetector(ctx, storeTee, testPrice(ctx, "BTC/USD"))

		// chop, pump, dump, failed bounce, flat, second pump, fade.
		tape := priceTape(t, "BTC/USD",
			"100", "100.1", "99.9", "100.05", "100", "103", "106", "104", "101",
			"98", "98.3", "97", "97", "97", "97", "100", "102", "101.5", "99",
		)
		byClass := scan(detector, storeTee, tape)
		counts := make(map[string]int)

		for class, list := range byClass {
			counts[class] = len(list)
		}

		t.Logf("synthetic tape episode counts: %v", counts)

		Convey("It publishes every episode, matching the oracle", func() {
			So(episodes(byClass), ShouldResemble, oracleEpisodes(tape))
			So(counts[excursionUp], ShouldEqual, 2)
		})

		Convey("A re-scan reproduces the same episodes (detect idempotency)", func() {
			So(episodes(scan(detector, storeTee, tape)), ShouldResemble, episodes(byClass))
		})
	})
}

/*
keys renders detections by every coordinate cmd/detect keys them on.
*/
func keys(byClass map[string][]*data.Measurement) map[string]bool {
	out := make(map[string]bool)

	for class, detections := range byClass {
		for _, detection := range detections {
			out[fmt.Sprintf(
				"%s|start=%v|b=%v|c=%v|bp=%v|cp=%v", class,
				metricRaw(detection, "start_tick"), metricRaw(detection, "b_tick"), metricRaw(detection, "c_tick"),
				metricRaw(detection, "b_price"), metricRaw(detection, "c_price"),
			)] = true
		}
	}

	return out
}

func TestDetector_GrowingEpochReproducesEarlierRuns(t *testing.T) {
	Convey("Given random tapes scanned first in part and then in full", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		storeTee := hindsight.NewStoreTee(ctx, "storeTee")
		storeTee.Transition(runtime.READY)
		detector := NewDetector(ctx, storeTee, testPrice(ctx, "BTC/USD"))
		rng := rand.New(rand.NewSource(11))
		lost := 0

		for range 300 {
			tape := randomTape(t, rng, 2+rng.Intn(30))
			cut := 1 + rng.Intn(len(tape))

			So(detector.Scan(tapeSeq(tape[:cut])), ShouldBeNil)
			early := keys(drain(storeTee))

			So(detector.Scan(tapeSeq(tape)), ShouldBeNil)
			full := keys(drain(storeTee))

			for key := range early {
				if !full[key] {
					lost++
				}
			}
		}

		Convey("Every detection of the shorter tape is reproduced by the longer one (no detect conflict)", func() {
			So(lost, ShouldEqual, 0)
		})
	})
}
