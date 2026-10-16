package strategy

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmstore "github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
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
		symbolChannels := make(map[string]chan *data.Value)
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

		for val := range store.Values(trainer.client, prefixes...) {
			if val == nil || val.Epoch != uint64(epoch) {
				continue
			}

			label := val.Scope
			channel, found := symbolChannels[label]

			if !found {
				channel = make(chan *data.Value, 256)
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

			channel <- val
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
	channel <-chan *data.Value,
) (map[string]*pathTally, map[string]*pathStats, error) {
	_ = epoch
	type timeBucket struct {
		peers []*data.Value
	}

	buckets := make(map[uint64]*timeBucket)
	var excursions []*data.Value

	for val := range channel {
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		default:
		}

		if val == nil {
			continue
		}

		if val.Origin == "detector" {
			class := val.Role
			errnie.Info(fmt.Sprintf("[trainer] detector excursion class: %s", class))

			if class != "up" && class != "down" && class != "up_friction" {
				continue
			}

			excursions = append(excursions, val)
			continue
		}

		if !slices.Contains(types.SignalSourceStrings, val.Origin) {
			continue
		}

		tick := val.Tick
		bucket, exists := buckets[tick]

		if !exists {
			bucket = &timeBucket{}
			buckets[tick] = bucket
		}

		bucket.peers = append(bucket.peers, val)
	}

	if len(buckets) == 0 || len(excursions) == 0 {
		return nil, nil, nil
	}

	ticks := make([]uint64, 0, len(buckets))

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
		at := time.Unix(0, int64(bucket.peers[0].Timestamp))
		if bucket.peers[0].Timestamp < 1e11 {
			at = time.Unix(int64(bucket.peers[0].Timestamp), 0)
		}

		frame := data.NewValue("frame", "frame", bucket.peers[0].Scope)
		frame.Timestamp = bucket.peers[0].Timestamp
		frame.Tick = tick
		for _, peer := range bucket.peers {
			frame.Layers = append(frame.Layers, peer.Layers...)
			frame.Index = append(frame.Index, peer.Index...)
		}

		regions := grid.Observe(frame)

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
		inValue := [2][]float64{
			regionBrightness,
			{0.0, 0.0, float64(step), float64(at.UnixNano()) / 1e9},
		}

		for val := range coder.Next(transport.NewOne(unsafe.Pointer(&inValue)).Next(nil)) {
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
		bPrice, cPrice, bTick, cTick := extractExcursionData(excursion)

		if bPrice.Sign() == 0 || cPrice.Sign() == 0 {
			continue
		}

		startAt := time.Unix(0, bTick)
		if bTick < 1e11 {
			startAt = time.Unix(bTick, 0)
		}
		endAt := time.Unix(0, cTick)
		if cTick < 1e11 {
			endAt = time.Unix(cTick, 0)
		}

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
		isUp := excursion.Role == "up"

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


		val := data.NewValue("policy", action, symbol)
		val.Epoch = uint64(epoch)
		val.FormatPrefix()
		data.AppendVector(val, []float64{
			gain / float64(visits),
			holdSeconds / float64(visits),
			float64(visits),
			gain / float64(visits),
			confidence,
		}, "gain", "hold_seconds", "visits", "mean_reward", "confidence", "path:"+path)

		if trainer.storeTee != nil {
			trainer.storeTee.Push(val)
			counts[action]++
			continue
		}

		if trainer.client != nil {
			if _, err := trainer.client.Write(val.Bytes()); err != nil {
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

func extractExcursionData(val *data.Value) (*decimal.Decimal, *decimal.Decimal, int64, int64) {
	bPrice := decimal.NewFromInt64(0)
	cPrice := decimal.NewFromInt64(0)
	var bTick, cTick int64

	if val == nil {
		return bPrice, cPrice, 0, 0
	}

	for _, layer := range val.Layers {
		if len(layer) >= 80 {
			floats := unsafe.Slice((*float64)(unsafe.Pointer(&layer[0])), len(layer)/8)
			if len(floats) >= 10 {
				bTick = int64(floats[3])
				cTick = int64(floats[5])
				bPrice = decimal.NewFromFloat64(floats[8])
				cPrice = decimal.NewFromFloat64(floats[9])
				return bPrice, cPrice, bTick, cTick
			}
		}
	}

	return bPrice, cPrice, bTick, cTick
}
