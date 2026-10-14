package strategy

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmstore "github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/store"
	"golang.org/x/sync/errgroup"
)

var SensorySources = []string{
	"correlation",
	"cvd",
	"depthflow",
	"hawkes",
	"leadlag",
	"liquidity",
	"morphology",
	"pumpdump",
	"sentiment",
	"toxicity",
}

type Trainer struct {
	*runtime.System
	client   *store.Client
	storeTee *hindsight.StoreTee
	price    *broker.Price
}

func NewTrainer(
	ctx context.Context,
	client *store.Client,
	storeTee *hindsight.StoreTee,
	price *broker.Price,
) *Trainer {
	trainer := &Trainer{
		System:   runtime.NewSystem(ctx, "trainer"),
		client:   client,
		storeTee: storeTee,
		price:    price,
	}

	if err := errnie.Require(map[string]any{
		"client":   client,
		"storeTee": storeTee,
		"price":    price,
	}); err != nil {
		trainer.Error(errnie.Err(
			errnie.Validation,
			"[trainer] cannot run trainer without client, storeTee, and price",
			err,
		))
	}

	return trainer
}

type pathStats struct {
	gain        float64
	holdSeconds float64
	visits      int
}

type pathTally struct {
	ups   int
	downs int
	exits int
}

type frameRecord struct {
	at   time.Time
	path string
}

func (trainer *Trainer) Generate(epochs ...int64) error {
	if trainer.client == nil || trainer.storeTee == nil || trainer.price == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[trainer] trainer requires client, storeTee, and price",
			nil,
		))
	}

	for _, epoch := range epochs {
		if epoch <= 0 {
			continue
		}

		group, ctx := errgroup.WithContext(trainer.Context())
		symbolChannels := make(map[string]chan *data.Measurement)
		var syncMutex sync.Mutex

		allTallies := make(map[string]*pathTally)
		allStats := make(map[string]*pathStats)

		sources := append([]string{"detector"}, SensorySources...)

		for _, source := range sources {
			for measurement := range trainer.client.Get(source + "/") {
				if measurement == nil || measurement.Epoch != epoch {
					continue
				}

				label := measurement.Label
				channel, found := symbolChannels[label]

				if !found {
					channel = make(chan *data.Measurement, 256)
					symbolChannels[label] = channel

					group.Go(func() error {
						tallies, stats, err := trainer.consumeSymbol(ctx, epoch, label, channel)

						if err != nil {
							return errnie.Error(err)
						}

						syncMutex.Lock()
						for path, tally := range tallies {
							merged, exists := allTallies[path]

							if !exists {
								merged = &pathTally{}
								allTallies[path] = merged
							}

							merged.ups += tally.ups
							merged.downs += tally.downs
							merged.exits += tally.exits
						}

						for path, stat := range stats {
							merged, exists := allStats[path]

							if !exists {
								merged = &pathStats{}
								allStats[path] = merged
							}

							merged.gain += stat.gain
							merged.holdSeconds += stat.holdSeconds
							merged.visits += stat.visits
						}
						syncMutex.Unlock()

						return nil
					})
				}

				channel <- measurement
			}
		}

		for _, channel := range symbolChannels {
			close(channel)
		}

		if err := group.Wait(); err != nil {
			return errnie.Error(err)
		}

		if len(allTallies) == 0 {
			continue
		}

		if err := trainer.publishPolicies(allTallies, allStats); err != nil {
			return errnie.Error(err)
		}
	}

	return nil
}

func (trainer *Trainer) consumeSymbol(
	ctx context.Context,
	epoch int64,
	symbol string,
	channel <-chan *data.Measurement,
) (map[string]*pathTally, map[string]*pathStats, error) {
	type timeBucket struct {
		peers []*data.Measurement
	}

	buckets := make(map[int64]*timeBucket)
	var excursions []*data.Measurement

	for measurement := range channel {
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		default:
		}

		if measurement == nil {
			continue
		}

		if measurement.Source == "detector" {
			typeEntry := data.Pull(measurement.Meta("type"))

			if typeEntry == nil {
				continue
			}

			class := typeEntry.Value

			if class != excursionUp && class != excursionDown && class != excursionUpShort {
				continue
			}

			excursions = append(excursions, measurement)
			continue
		}

		if !slices.Contains(SensorySources, measurement.Source) {
			continue
		}

		timestamp := measurement.At.UnixNano()
		bucket, exists := buckets[timestamp]

		if !exists {
			bucket = &timeBucket{}
			buckets[timestamp] = bucket
		}

		bucket.peers = append(bucket.peers, measurement)
	}

	if len(buckets) == 0 || len(excursions) == 0 {
		return nil, nil, nil
	}

	timestamps := make([]int64, 0, len(buckets))
	for timestamp := range buckets {
		timestamps = append(timestamps, timestamp)
	}
	slices.Sort(timestamps)

	frames := make([]frameRecord, 0, len(timestamps))
	grid := nmstore.NewGrid(trainer.Context())
	grammar := NewGrammar()
	activePath := ""
	lastSym := ""

	for _, timestamp := range timestamps {
		bucket := buckets[timestamp]
		at := time.Unix(0, timestamp).UTC()

		join := data.NewMeasurement(epoch, symbol, "runtime:join", at, at)
		join.Peers(bucket.peers...)
		join = join.Write()

		regions := grid.Observe(join)

		if len(regions) == 0 {
			continue
		}

		sym := fmt.Sprintf("R%02d", regions[0].Key)

		if sym == lastSym {
			frames = append(frames, frameRecord{
				at:   at,
				path: activePath,
			})
			continue
		}

		lastSym = sym
		rule := grammar.Append(sym)
		activePath = ""

		if rule != nil {
			phrase := rule.TerminalNames(grammar)

			if len(phrase) >= 2 {
				activePath = strings.Join(phrase, "/")
			}
		}

		frames = append(frames, frameRecord{
			at:   at,
			path: activePath,
		})
	}

	if err := grid.Error(); err != nil {
		return nil, nil, errnie.Error(err)
	}

	if len(frames) == 0 {
		return nil, nil, nil
	}

	tallies := make(map[string]*pathTally)
	stats := make(map[string]*pathStats)

	getTally := func(pathKey string) *pathTally {
		tally, exists := tallies[pathKey]

		if !exists {
			tally = &pathTally{}
			tallies[pathKey] = tally
		}

		return tally
	}

	getStats := func(pathKey string) *pathStats {
		stat, exists := stats[pathKey]

		if !exists {
			stat = &pathStats{}
			stats[pathKey] = stat
		}

		return stat
	}

	for _, excursion := range excursions {
		bPrice, cPrice := extractPrices(excursion)

		if bPrice.Sign() == 0 || cPrice.Sign() == 0 {
			continue
		}

		startAt := excursion.From
		endAt := excursion.At

		startIdx := -1
		for index, frame := range frames {
			if !frame.at.After(startAt) {
				startIdx = index
			}
		}

		endIdx := -1
		for index, frame := range frames {
			if !frame.at.After(endAt) {
				endIdx = index
			}
		}

		if startIdx < 0 || startIdx >= len(frames) {
			continue
		}

		entryPath := frames[startIdx].path

		if entryPath == "" {
			continue
		}

		pnl, cost, err := trainer.price.RoundTrip(symbol, bPrice, cPrice)
		netReturn := 0.0

		if err == nil && cost.Sign() > 0 {
			netReturn = pnl.Div(cost).Float64()
		}

		holdSeconds := endAt.Sub(startAt).Seconds()
		isUp := false

		if typeEntry := data.Pull(excursion.Meta("type")); typeEntry != nil && typeEntry.Value == excursionUp {
			isUp = true
		}

		if isUp && netReturn > 0 {
			getTally(entryPath).ups++
			stat := getStats(entryPath)
			stat.gain += netReturn
			stat.holdSeconds += holdSeconds
			stat.visits++
		}

		if !isUp || netReturn <= 0 {
			getTally(entryPath).downs++
			stat := getStats(entryPath)
			stat.gain += netReturn
			stat.holdSeconds += holdSeconds
			stat.visits++
		}

		if isUp && endIdx >= 0 && endIdx < len(frames) {
			exitPath := frames[endIdx].path

			if exitPath != "" {
				getTally(exitPath).exits++
				stat := getStats(exitPath)
				stat.gain += netReturn
				stat.holdSeconds += holdSeconds
				stat.visits++
			}
		}
	}

	return tallies, stats, nil
}

func (trainer *Trainer) publishPolicies(
	tallies map[string]*pathTally,
	stats map[string]*pathStats,
) error {
	counts := make(map[string]int)

	for path, tally := range tallies {
		var action string

		if tally.ups >= 2 && tally.downs == 0 && tally.exits == 0 {
			action = "enter"
		}

		if tally.exits > 0 && tally.ups == 0 {
			action = "exit"
		}

		if tally.downs > 0 && tally.ups == 0 && tally.exits == 0 {
			action = "noop"
		}

		if action == "" {
			continue
		}

		entry := stats[path]
		visits := 1
		gain := 0.0
		holdSeconds := 0.0

		if entry != nil && entry.visits > 0 {
			visits = entry.visits
			gain = entry.gain
			holdSeconds = entry.holdSeconds
		}

		trainer.storeTee.Push(data.NewMeasurement(
			0, path, "policy", time.Now().UTC(), time.Now().UTC(),
			&data.StringEntry{Key: "action", Value: action},
		).Write(
			data.NewMetric("gain", gain/float64(visits), data.UnitNone, data.TimescaleTick),
			data.NewMetric("hold_seconds", holdSeconds/float64(visits), data.UnitNone, data.TimescaleTick),
			data.NewMetric("visits", float64(visits), data.UnitNone, data.TimescaleTick),
			data.NewMetric("mean_reward", gain/float64(visits), data.UnitNone, data.TimescaleTick),
		))

		counts[action]++
	}

	errnie.Info(fmt.Sprintf(
		"[trainer] published policies (enter=%d exit=%d noop=%d)",
		counts["enter"], counts["exit"], counts["noop"],
	))

	return nil
}

func extractPrices(measurement *data.Measurement) (*decimal.Decimal, *decimal.Decimal) {
	bPrice := decimal.NewFromInt64(0)
	cPrice := decimal.NewFromInt64(0)

	if entry := data.Pull(measurement.Read("b_price")); entry != nil && entry.Metric != nil {
		if exact := entry.Metric.Exact(); exact != nil {
			bPrice = exact
		}
	}

	if entry := data.Pull(measurement.Read("c_price")); entry != nil && entry.Metric != nil {
		if exact := entry.Metric.Exact(); exact != nil {
			cPrice = exact
		}
	}

	return bPrice, cPrice
}
