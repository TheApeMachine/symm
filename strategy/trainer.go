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
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
	"golang.org/x/sync/errgroup"
)

type Trainer struct {
	*runtime.System
	client   *store.Client
	storeTee *hindsight.StoreTee
	price    *broker.Price
	symbols  []string
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
		sources := append([]string{"detector"}, types.SignalSourceStrings...)

		symbols := trainer.symbols

		if len(symbols) == 0 {
			for page := range trainer.client.List("spot:trade/", "/") {
				for _, cp := range page.CommonPrefixes {
					if cp.Prefix == nil {
						continue
					}

					for subPage := range trainer.client.List(*cp.Prefix, "/") {
						for _, subCp := range subPage.CommonPrefixes {
							if subCp.Prefix == nil {
								continue
							}

							symbol := strings.TrimPrefix(*subCp.Prefix, "spot:trade/")
							symbol = strings.Trim(symbol, "/")

							if symbol != "" && !slices.Contains(symbols, symbol) {
								symbols = append(symbols, symbol)
							}
						}
					}
				}
			}
		}

		if len(symbols) == 0 {
			errnie.Warn("[trainer] no symbols found, using default BTC/USD")
			symbols = []string{"BTC/USD"}
		}

		var prefixes []string

		for _, source := range sources {
			for _, symbol := range symbols {
				prefixes = append(
					prefixes,
					fmt.Sprintf("%s/%s/%d", source, symbol, epoch),
				)
			}
		}

		var tallyMu sync.Mutex
		globalTallies := make(map[string]*pathTally)
		globalStats := make(map[string]*pathStats)

		for measurement := range trainer.client.Get(prefixes...) {
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

					tallyMu.Lock()

					for p, t := range tallies {
						gt, exists := globalTallies[p]

						if !exists {
							gt = &pathTally{}
							globalTallies[p] = gt
						}

						gt.ups += t.ups
						gt.downs += t.downs
						gt.exits += t.exits
					}

					for p, s := range stats {
						gs, exists := globalStats[p]

						if !exists {
							gs = &pathStats{}
							globalStats[p] = gs
						}

						gs.gain += s.gain
						gs.holdSeconds += s.holdSeconds
						gs.visits += s.visits
					}

					tallyMu.Unlock()

					return nil
				})
			}

			channel <- measurement
		}

		for _, channel := range symbolChannels {
			close(channel)
		}

		if err := group.Wait(); err != nil {
			return errnie.Error(err)
		}

		if len(globalTallies) > 0 {
			if err := trainer.publishPolicies(epoch, "current", globalTallies, globalStats); err != nil {
				return errnie.Error(err)
			}
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
			errnie.Info(fmt.Sprintf("[trainer] typeEntry: %v", typeEntry))

			if typeEntry == nil {
				continue
			}

			class := typeEntry.Value

			if class != "up" && class != "down" && class != "up_friction" {
				continue
			}

			excursions = append(excursions, measurement)
			continue
		}

		if !slices.Contains(types.SignalSourceStrings, measurement.Source) {
			continue
		}

		tick := measurement.Tick
		bucket, exists := buckets[tick]

		if !exists {
			bucket = &timeBucket{}
			buckets[tick] = bucket
		}

		bucket.peers = append(bucket.peers, measurement)
	}

	if len(buckets) == 0 || len(excursions) == 0 {
		return nil, nil, nil
	}

	ticks := make([]int64, 0, len(buckets))

	for tick := range buckets {
		ticks = append(ticks, tick)
	}

	slices.Sort(ticks)

	frames := make([]frameRecord, 0, len(ticks))
	grid := nmstore.NewGrid(trainer.Context())
	grammar := NewGrammar()
	coder := newPredictiveCoder()
	step := 0
	lastSym := ""
	recentTokens := make([]string, 0, 16)

	for _, tick := range ticks {
		bucket := buckets[tick]
		at := bucket.peers[0].At

		join := data.NewMeasurement(epoch, symbol, "runtime:join", at, at)
		join.Tick = tick
		join.Peers(bucket.peers...)
		join = join.Write()

		regions := grid.Observe(join)

		if len(regions) == 0 {
			continue
		}

		// 1. Vector of 15 region brightnesses
		regionBrightness := make([]float64, 15)
		for _, r := range regions {
			if int(r.Key) < 15 {
				regionBrightness[r.Key] = r.Brightness
			}
		}

		// 2. PredictiveCoder settles over the 15 regions
		step++
		inValue := data.NewValue([2][]float64{
			regionBrightness,
			{0.0, 0.0, float64(step), float64(at.UnixNano()) / 1e9},
		})

		for val := range coder.Next(inValue.Next(nil)) {
			_ = val
		}

		// 3. Dynamic Midpoint Activation Gate:
		active := make([]*nmstore.Region, 0)
		for _, r := range regions {
			if r.Brightness > 0 {
				active = append(active, r)
			}
		}

		if len(active) == 0 {
			continue
		}

		maxB := active[0].Brightness
		minB := active[len(active)-1].Brightness
		midB := (maxB + minB) / 2.0

		var firedTokens []string
		for _, r := range active {
			if r.Brightness >= midB {
				firedTokens = append(firedTokens, fmt.Sprintf("R%02d", r.Key))
			}
		}

		if len(firedTokens) == 0 {
			continue
		}

		chordKey := strings.Join(firedTokens, "/")
		if chordKey == lastSym {
			frames = append(frames, frameRecord{
				at:   at,
				path: "",
			})
			continue
		}
		lastSym = chordKey

		var deepestRule *Rule
		for _, token := range firedTokens {
			rule := grammar.Append(token)
			if rule != nil {
				deepestRule = rule
			}
		}

		recentTokens = append(recentTokens, firedTokens...)

		if len(recentTokens) > 16 {
			recentTokens = recentTokens[len(recentTokens)-16:]
		}

		activePath := ""

		if deepestRule != nil {
			phrase := deepestRule.TerminalNames(grammar)
			if len(phrase) >= 2 {
				activePath = strings.Join(phrase, "/")
			}
		}

		if activePath == "" && len(recentTokens) >= 2 {
			activePath = strings.Join(recentTokens[len(recentTokens)-2:], "/")
		} else if activePath == "" && len(recentTokens) == 1 {
			activePath = recentTokens[0]
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

	horizon := system.Cfg.Learning.PrecursorHorizon

	if horizon <= 0 {
		horizon = 8
	}

	findPath := func(idx int, minIdx int) string {
		if idx < 0 || idx >= len(frames) {
			return ""
		}

		for i := idx; i >= minIdx && i >= 0; i-- {
			if frames[i].path != "" {
				return frames[i].path
			}
		}

		return ""
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

		if startIdx < 0 && len(frames) > 0 {
			startIdx = 0
		}

		endIdx := -1

		for index, frame := range frames {
			if !frame.at.After(endAt) {
				endIdx = index
			}
		}

		minStartLook := max(startIdx-horizon, 0)
		entryPath := findPath(startIdx, minStartLook)

		pnl, cost, err := trainer.price.RoundTrip(symbol, bPrice, cPrice)
		netReturn := 0.0

		if err == nil && cost.Sign() > 0 {
			netReturn = pnl.Div(cost).Float64()
		}

		holdSeconds := endAt.Sub(startAt).Seconds()
		isUp := false

		if typeEntry := data.Pull(excursion.Meta("type")); typeEntry != nil && typeEntry.Value == "up" {
			isUp = true
		}

		if isUp && netReturn > 0 {
			if entryPath != "" {
				getTally(entryPath).ups++
				stat := getStats(entryPath)
				stat.gain += netReturn
				stat.holdSeconds += holdSeconds
				stat.visits++
			}
		}

		if !isUp || netReturn <= 0 {
			if entryPath != "" {
				getTally(entryPath).downs++
				stat := getStats(entryPath)
				stat.gain += netReturn
				stat.holdSeconds += holdSeconds
				stat.visits++
			}
		}

		if isUp && netReturn > 0 && endIdx >= 0 && endIdx < len(frames) {
			exitLook := startIdx

			if exitLook < 0 {
				exitLook = endIdx - horizon
			}

			exitPath := findPath(endIdx, max(0, exitLook))

			if exitPath != "" && exitPath != entryPath {
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
	epoch int64,
	symbol string,
	tallies map[string]*pathTally,
	stats map[string]*pathStats,
) error {
	counts := make(map[string]int)

	for path, tally := range tallies {
		errnie.Info(fmt.Sprintf(
			"[trainer] path %s: ups=%d downs=%d exits=%d",
			path, tally.ups, tally.downs, tally.exits,
		))

		var action string

		totalVisits := tally.ups + tally.downs + tally.exits

		if totalVisits == 0 {
			continue
		}

		if tally.ups > tally.downs && tally.ups > tally.exits {
			action = "enter"
		} else if tally.exits > tally.ups && tally.exits >= tally.downs {
			action = "exit"
		} else if tally.downs >= tally.ups && tally.downs > tally.exits {
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

		confidence := 1.0

		if totalVisits > 0 {
			switch action {
			case "enter":
				confidence = float64(tally.ups) / float64(totalVisits)
			case "exit":
				confidence = float64(tally.exits) / float64(totalVisits)
			case "noop":
				confidence = float64(tally.downs) / float64(totalVisits)
			}
		}

		gainUnit, gainTime := data.GetDimension("policy", "gain")
		holdUnit, holdTime := data.GetDimension("policy", "hold_seconds")
		visitsUnit, visitsTime := data.GetDimension("policy", "visits")
		rewardUnit, rewardTime := data.GetDimension("policy", "mean_reward")
		confidenceUnit, confidenceTime := data.GetDimension("policy", "confidence")

		meas := data.NewMeasurement(
			epoch, symbol, "policy", time.Now().UTC(), time.Now().UTC(),
			&data.StringEntry{Key: "action", Value: action},
			&data.StringEntry{Key: "path", Value: path},
		).Write(
			data.NewMetric("gain", gain/float64(visits), gainUnit, gainTime),
			data.NewMetric("hold_seconds", holdSeconds/float64(visits), holdUnit, holdTime),
			data.NewMetric("visits", float64(visits), visitsUnit, visitsTime),
			data.NewMetric("mean_reward", gain/float64(visits), rewardUnit, rewardTime),
			data.NewMetric("confidence", confidence, confidenceUnit, confidenceTime),
		)

		if trainer.storeTee != nil {
			trainer.storeTee.Push(meas)
			counts[action]++
			continue
		}

		if trainer.client != nil {
			key := fmt.Sprintf("policy/%s/%019d/%s/%s.bin", symbol, epoch, path, action)
			payload, err := meas.MarshalBinary()

			if err != nil {
				errnie.Error(err)
				counts[action]++
				continue
			}

			if err := trainer.client.Put(key, payload); err != nil {
				errnie.Error(err)
			}
		}

		counts[action]++
	}

	errnie.Info(fmt.Sprintf(
		"[trainer] published policies for %s epoch %d (enter=%d exit=%d noop=%d)",
		symbol, epoch, counts["enter"], counts["exit"], counts["noop"],
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
