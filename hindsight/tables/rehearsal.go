package tables

import (
	"context"
	"math"
	"slices"
	"sort"
	"time"

	"github.com/apache/iceberg-go"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/types"
)

/*
TapePublisher abstracts the handoff between the Hindsight archive walk and the learner cohort.
strategy.Tape satisfies this interface directly without introducing circular dependencies.
*/
type TapePublisher interface {
	Publish(fragment types.ReplayFragment)
	Close()
	SetBudget(budget uint64)
	AddObservations(count uint64)
	AddRuns(count uint64)
}

type observationTick struct {
	tick     int64
	venueAt  time.Time
	symbol   string
	price    float64
	qty      float64
	isTicker bool
}

/*
LoadRehearsalTape walks historical runs in the Iceberg catalog, discovers market
excursion fragments, and publishes them into the training tape up to the observation budget.
*/
func (c *Catalog) LoadRehearsalTape(ctx context.Context, tape TapePublisher) error {
	if c == nil || tape == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[iceberg] LoadRehearsalTape: catalog or tape is nil",
			nil,
		))
	}

	ctx = c.Context(ctx)
	defer tape.Close()

	budget := uint64(viper.GetInt("hindsight.rehearsal.observation_budget"))

	if budget == 0 {
		budget = 500000
	}

	tape.SetBudget(budget)

	epochs, err := c.Epochs(ctx)

	if err != nil {
		return err
	}

	if len(epochs) == 0 {
		return nil
	}

	// Practice on completed runs newest first
	walkEpochs := make([]int64, len(epochs))
	copy(walkEpochs, epochs)
	slices.Reverse(walkEpochs)

	totalObservations := uint64(0)

	for _, epoch := range walkEpochs {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if totalObservations >= budget {
			break
		}

		tickers, err := c.SpotTickerScan(ctx, epoch, iceberg.AlwaysTrue{}, 0)

		if err != nil {
			errnie.Error(err)
			continue
		}

		trades, err := c.SpotTradeScan(ctx, epoch, iceberg.AlwaysTrue{}, 0)

		if err != nil {
			errnie.Error(err)
			continue
		}

		if len(tickers) == 0 && len(trades) == 0 {
			continue
		}

		symbolObservations := make(map[string][]observationTick)

		for _, ticker := range tickers {
			price := ticker.Last

			if price <= 0 && ticker.Bid > 0 && ticker.Ask > 0 {
				price = (ticker.Bid + ticker.Ask) / 2
			}

			if price <= 0 && ticker.Bid > 0 {
				price = ticker.Bid
			}

			if price <= 0 && ticker.Ask > 0 {
				price = ticker.Ask
			}

			if price <= 0 {
				continue
			}

			symbolObservations[ticker.Symbol] = append(symbolObservations[ticker.Symbol], observationTick{
				tick:     ticker.Tick,
				venueAt:  ticker.VenueAt,
				symbol:   ticker.Symbol,
				price:    price,
				qty:      ticker.BidQty + ticker.AskQty,
				isTicker: true,
			})
		}

		for _, trade := range trades {
			if trade.Price <= 0 {
				continue
			}

			symbolObservations[trade.Symbol] = append(symbolObservations[trade.Symbol], observationTick{
				tick:     trade.Tick,
				venueAt:  trade.VenueAt,
				symbol:   trade.Symbol,
				price:    trade.Price,
				qty:      trade.Qty,
				isTicker: false,
			})
		}

		symbols := make([]string, 0, len(symbolObservations))

		for symbol, ticks := range symbolObservations {
			if len(ticks) < 4 {
				continue
			}

			symbols = append(symbols, symbol)
		}

		sort.Strings(symbols)

		symbolFragments := make(map[string][]types.ReplayFragment)
		maxFragments := 0

		for _, symbol := range symbols {
			ticks := symbolObservations[symbol]

			sort.Slice(ticks, func(firstIndex, secondIndex int) bool {
				return ticks[firstIndex].tick < ticks[secondIndex].tick
			})

			fragmentSize := 32

			if len(ticks) < fragmentSize {
				fragmentSize = len(ticks)
			}

			stride := fragmentSize / 2

			if stride < 1 {
				stride = 1
			}

			var fragments []types.ReplayFragment

			for start := 0; start+fragmentSize <= len(ticks); start += stride {
				slice := ticks[start : start+fragmentSize]

				hasMovement := false
				firstPrice := slice[0].price

				for _, item := range slice[1:] {
					if item.price != firstPrice {
						hasMovement = true
						break
					}
				}

				if !hasMovement {
					continue
				}

				fragment := buildFragment(symbol, slice)
				fragments = append(fragments, fragment)
			}

			if len(fragments) > 0 {
				symbolFragments[symbol] = fragments

				if len(fragments) > maxFragments {
					maxFragments = len(fragments)
				}
			}
		}

		if maxFragments == 0 {
			for _, symbol := range symbols {
				ticks := symbolObservations[symbol]

				fragmentSize := 32

				if len(ticks) < fragmentSize {
					fragmentSize = len(ticks)
				}

				stride := fragmentSize / 2

				if stride < 1 {
					stride = 1
				}

				var fragments []types.ReplayFragment

				for start := 0; start+fragmentSize <= len(ticks); start += stride {
					slice := ticks[start : start+fragmentSize]
					fragment := buildFragment(symbol, slice)
					fragments = append(fragments, fragment)
				}

				if len(fragments) > 0 {
					symbolFragments[symbol] = fragments

					if len(fragments) > maxFragments {
						maxFragments = len(fragments)
					}
				}
			}
		}

		fragmentsPublished := 0

		for round := 0; round < maxFragments; round++ {
			if totalObservations >= budget {
				break
			}

			for _, symbol := range symbols {
				frags := symbolFragments[symbol]

				if round >= len(frags) {
					continue
				}

				if totalObservations >= budget {
					break
				}

				frag := frags[round]
				tape.Publish(frag)
				obsCount := uint64(len(frag.Frames))
				totalObservations += obsCount
				tape.AddObservations(obsCount)
				fragmentsPublished++
			}
		}

		if fragmentsPublished > 0 {
			tape.AddRuns(1)
		}
	}

	return nil
}

func buildFragment(symbol string, slice []observationTick) types.ReplayFragment {
	frames := make([][]*data.Measurement[float64], len(slice))
	anchorIdx := len(slice) / 3
	extremumIdx := (2 * len(slice)) / 3

	anchorPrice := slice[anchorIdx].price
	maxDisplacement := 0.0

	for index := anchorIdx + 1; index < len(slice); index++ {
		diff := math.Abs(slice[index].price - anchorPrice)

		if diff > maxDisplacement {
			maxDisplacement = diff
			extremumIdx = index
		}
	}

	for index, item := range slice {
		meas := data.NewMeasurement[float64]("market", map[string]data.Metric[float64]{
			"price": data.NewMetric[float64]("price", data.UnitDimensionless, data.TimescaleInstantaneous, 0, item.price),
			"qty":   data.NewMetric[float64]("qty", data.UnitCount, data.TimescaleInstantaneous, 0, item.qty),
		})
		meas.Label = symbol
		meas.At = item.venueAt
		meas.SeqIdx = item.tick

		frames[index] = []*data.Measurement[float64]{meas}
	}

	return types.ReplayFragment{
		Frames:        frames,
		Symbol:        symbol,
		AnchorIndex:   anchorIdx,
		ExtremumIndex: extremumIdx,
	}
}
