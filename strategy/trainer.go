package strategy

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmstore "github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/system"
	"golang.org/x/sync/errgroup"
)

type Trainer struct {
	*runtime.System
	client       *store.Client
	price        *broker.Price
	symbols      []string
	OnFrame      func()
	SetWorkTotal func(int64)
	writeMu      sync.Mutex
	tickIndexMu  sync.Mutex
	tickIndexes  map[int64]map[uint64]string
}

func NewTrainer(
	ctx context.Context,
	client *store.Client,
	price *broker.Price,
	symbols ...string,
) *Trainer {
	trainer := &Trainer{
		System:  runtime.NewSystem(ctx, "trainer"),
		client:  client,
		price:   price,
		symbols: symbols,
	}

	return trainer
}

/*
ExcursionKeyCount is listed detector objects for the epoch (before GET filtering).
*/
func (trainer *Trainer) ExcursionKeyCount(epoch int64) int {
	n := 0

	for _, job := range trainer.detectionJobs(epoch) {
		if job.epoch == epoch {
			n++
		}
	}

	return n
}

/*
WorkTotal is the number of detection jobs train will run.
*/
func (trainer *Trainer) WorkTotal(epochs ...int64) int64 {
	return int64(len(trainer.detectionJobs(epochs...)))
}

/*
parseDetectionStorageKey reads class and epoch from detector/<class>/…/<epoch>/<tick>.bin.
*/
func parseDetectionStorageKey(key string) (class string, epoch uint64, ok bool) {
	head := data.ExcursionOrigin + "/"

	if !strings.HasPrefix(key, head) {
		return "", 0, false
	}

	parts := strings.Split(strings.TrimPrefix(key, head), "/")

	if len(parts) < 3 {
		return "", 0, false
	}

	class = parts[0]
	epochPart := parts[len(parts)-2]

	if len(epochPart) != 19 {
		return "", 0, false
	}

	parsed, err := strconv.ParseUint(epochPart, 10, 64)

	if err != nil {
		return "", 0, false
	}

	return class, parsed, true
}

/*
balanceDetectionJobs keeps every up and down detection but caps chop and flat
at the up count so background classes cannot dominate training.
*/
func balanceDetectionJobs(jobs []detectionJob) []detectionJob {
	byClass := make(map[string][]detectionJob, len(data.ExcursionClasses))

	for _, job := range jobs {
		class, _, ok := parseDetectionStorageKey(job.key)

		if !ok {
			continue
		}

		byClass[class] = append(byClass[class], job)
	}

	up := byClass["up"]
	capN := len(up)
	out := make([]detectionJob, 0, len(jobs))

	out = append(out, up...)
	out = append(out, byClass["down"]...)

	chop := byClass["chop"]
	if len(chop) > capN {
		errnie.Info(fmt.Sprintf("[trainer] cap chop: %d → %d (up=%d)", len(chop), capN, capN))
		chop = chop[:capN]
	}

	out = append(out, chop...)

	flat := byClass["flat"]
	if len(flat) > capN {
		errnie.Info(fmt.Sprintf("[trainer] cap flat: %d → %d (up=%d)", len(flat), capN, capN))
		flat = flat[:capN]
	}

	out = append(out, flat...)

	return out
}

/*
detectionJobs lists every detector class train reads (data.ExcursionClasses).
Chop and flat are capped to the number of up detections. When epochs is
non-empty, only keys whose storage epoch is in that set are kept.
*/
func (trainer *Trainer) detectionJobs(epochs ...int64) []detectionJob {
	allowed := map[int64]bool{}

	for _, epoch := range epochs {
		if epoch > 0 {
			allowed[epoch] = true
		}
	}

	filterEpochs := len(allowed) > 0
	jobs := make([]detectionJob, 0)
	seen := make(map[string]bool)

	for _, class := range data.ExcursionClasses {
		prefix := data.ExcursionOrigin + "/" + class + "/"

		for _, key := range trainer.client.ListKeys(prefix) {
			if !strings.HasSuffix(key, ".bin") || seen[key] {
				continue
			}

			seen[key] = true

			_, epoch, ok := parseDetectionStorageKey(key)

			if !ok {
				continue
			}

			if filterEpochs && !allowed[int64(epoch)] {
				continue
			}

			jobs = append(jobs, detectionJob{epoch: int64(epoch), key: key})
		}
	}

	return balanceDetectionJobs(jobs)
}

/*
FrameCount is the number of trainable excursions for the epoch (same set Generate runs).
*/
func (trainer *Trainer) FrameCount(epoch int64) int {
	return len(trainer.loadExcursions(epoch))
}

/*
loadExcursions serially reads every listed detection for the epoch.
*/
func (trainer *Trainer) loadExcursions(epoch int64) []*data.Value {
	out := make([]*data.Value, 0)

	for _, job := range trainer.detectionJobs(epoch) {
		if job.epoch != epoch {
			continue
		}

		key := job.key
		val, err := store.ReadValue(trainer.client, key)

		if err != nil {
			errnie.Error(errnie.Err(errnie.IO, "[trainer] excursion "+key, err))
			continue
		}

		if val.Epoch != uint64(epoch) {
			continue
		}

		class := val.Role

		if !slices.Contains(data.ExcursionClasses, class) {
			continue
		}

		if len(trainer.symbols) > 0 && !slices.Contains(trainer.symbols, val.Scope) {
			continue
		}

		out = append(out, val)
	}

	return out
}

func (trainer *Trainer) trainableExcursion(val *data.Value, epoch int64) bool {
	if val == nil || val.Epoch != uint64(epoch) {
		return false
	}

	if !slices.Contains(data.ExcursionClasses, val.Role) {
		return false
	}

	if len(trainer.symbols) > 0 && !slices.Contains(trainer.symbols, val.Scope) {
		return false
	}

	return true
}

func (trainer *Trainer) tickIndexForEpoch(epoch int64) map[uint64]string {
	trainer.tickIndexMu.Lock()
	defer trainer.tickIndexMu.Unlock()

	if trainer.tickIndexes == nil {
		trainer.tickIndexes = make(map[int64]map[uint64]string)
	}

	if index, ok := trainer.tickIndexes[epoch]; ok {
		return index
	}

	index := trainer.tickKeyIndex(epoch)
	trainer.tickIndexes[epoch] = index

	return index
}

func (trainer *Trainer) tickKeyIndex(epoch int64) map[uint64]string {
	index := make(map[uint64]string)

	for _, key := range trainer.client.ListKeys(data.TickPrefix(uint64(epoch))) {
		if tick, ok := tickFromStorageKey(key); ok {
			index[tick] = key
		}
	}

	return index
}

func tickFromStorageKey(key string) (uint64, bool) {
	base := key

	if idx := strings.LastIndex(key, "/"); idx >= 0 {
		base = key[idx+1:]
	}

	base = strings.TrimSuffix(base, ".bin")

	if len(base) != 19 {
		return 0, false
	}

	var tick uint64

	for _, digit := range base {
		if digit < '0' || digit > '9' {
			return 0, false
		}

		tick = tick*10 + uint64(digit-'0')
	}

	return tick, true
}

type detectionJob struct {
	epoch int64
	key   string
}

func (trainer *Trainer) Generate(epochs ...int64) error {
	if trainer.client == nil || trainer.price == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[trainer] trainer requires client and price",
			nil,
		))
	}

	jobs := trainer.detectionJobs(epochs...)

	if len(jobs) == 0 {
		return nil
	}

	if trainer.SetWorkTotal != nil {
		trainer.SetWorkTotal(int64(len(jobs)))
	}

	group, ctx := errgroup.WithContext(trainer.Context())

	for _, job := range jobs {
		job := job

		group.Go(func() error {
			if trainer.OnFrame != nil {
				defer trainer.OnFrame()
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			excursion, err := store.ReadValue(trainer.client, job.key)

			if err != nil {
				errnie.Error(errnie.Err(errnie.IO, "[trainer] excursion "+job.key, err))
				return nil
			}

			trainEpoch := int64(excursion.Epoch)

			if !trainer.trainableExcursion(excursion, trainEpoch) {
				return nil
			}

			return trainer.trainExcursion(job.key, trainEpoch, excursion)
		})
	}

	if err := group.Wait(); err != nil {
		return errnie.Error(err)
	}

	return nil
}

func (trainer *Trainer) trainExcursion(
	detectionKey string,
	epoch int64,
	excursion *data.Value,
) error {
	tag := detectionKey
	if tag == "" && excursion != nil {
		tag = excursion.Prefix
	}

	if excursion == nil {
		return nil
	}

	fields, err := data.DetectExcursionFields(excursion)

	if err != nil {
		return nil
	}

	aTick := uint64(fields["a_tick"])
	bTick := uint64(fields["b_tick"])
	dTick := uint64(fields["d_tick"])

	if aTick == 0 || dTick < aTick || bTick < aTick || bTick > dTick {
		return nil
	}

	tickIndex := trainer.tickIndexForEpoch(epoch)
	replayFrom := tickWarmupBefore(aTick)
	ticks, missing, loadErr := trainer.loadTicksCounted(tickIndex, replayFrom, dTick)

	if loadErr != nil {
		return errnie.Error(loadErr)
	}

	if len(ticks) == 0 {
		errnie.Info(fmt.Sprintf(
			"[trainer] %s: no ticks in [%d,%d] (a=%d d=%d missing=%d index=%d)",
			tag, replayFrom, dTick, aTick, dTick, missing, len(tickIndex),
		))
		return nil
	}

	precursorPath, holdingPath, replayErr := trainer.replayTape(ticks, bTick, dTick)

	if replayErr != nil {
		return errnie.Error(replayErr)
	}

	if precursorPath == "" && holdingPath == "" {
		errnie.Info(fmt.Sprintf(
			"[trainer] %s: no paths after replay (loaded=%d a=%d b=%d d=%d)",
			tag, len(ticks), aTick, bTick, dTick,
		))
		return nil
	}

	bPrice := decimal.NewFromFloat64(fields["b_price"])
	cPrice := decimal.NewFromFloat64(fields["c_price"])

	if bPrice.Sign() == 0 || cPrice.Sign() == 0 {
		return nil
	}

	label := excursion.Scope

	if label == "" {
		label = "current"
	}

	pnl, cost, roundErr := trainer.price.RoundTrip(label, bPrice, cPrice)
	netReturn := 0.0

	if roundErr == nil && pnl != nil && cost != nil && cost.Sign() > 0 {
		netReturn = pnl.Div(cost).Float64()
	} else if bPrice.Sign() > 0 {
		move := cPrice.Sub(bPrice).Div(bPrice).Float64()
		fee := 0.0

		if rate, ok := trainer.price.FeeRate(label); ok {
			fee = 2 * rate
		}

		netReturn = move - fee
	}

	isUp := excursion.Role == "up"

	if isUp && netReturn > 0 {
		if err := trainer.writePolicy(epoch, label, precursorPath, "enter", netReturn); err != nil {
			return errnie.Error(err)
		}

		if holdingPath != "" {
			if err := trainer.writePolicy(epoch, label, holdingPath, "exit", netReturn); err != nil {
				return errnie.Error(err)
			}
		}

		return nil
	}

	if precursorPath != "" {
		if err := trainer.writePolicy(epoch, label, precursorPath, "noop", netReturn); err != nil {
			return errnie.Error(err)
		}
	}

	if holdingPath != "" && holdingPath != precursorPath {
		if err := trainer.writePolicy(epoch, label, holdingPath, "noop", netReturn); err != nil {
			return errnie.Error(err)
		}
	}

	return nil
}

func tickWarmupBefore(aTick uint64) uint64 {
	warmup := uint64(64)

	if system.Cfg != nil && system.Cfg.Learning.PrecursorHorizon > 0 {
		warmup = uint64(system.Cfg.Learning.PrecursorHorizon * 8)
	}

	if aTick > warmup {
		return aTick - warmup
	}

	return 1
}

func (trainer *Trainer) loadTicksCounted(
	index map[uint64]string,
	fromTick, toTick uint64,
) ([]*data.Value, int, error) {
	vals := make([]*data.Value, 0, toTick-fromTick+1)
	missing := 0

	for tick := fromTick; tick <= toTick; tick++ {
		key, ok := index[tick]

		if !ok {
			missing++
			continue
		}

		val, err := store.ReadValue(trainer.client, key)

		if err != nil {
			return nil, missing, err
		}

		vals = append(vals, val)
	}

	slices.SortFunc(vals, func(first, second *data.Value) int {
		return cmp.Compare(first.Tick, second.Tick)
	})

	return vals, missing, nil
}

/*
replayTape runs grid, predictive coder, and Sequitur over [a,d] and returns the
Sequitur paths settled by b (precursor) and by d (holding).
*/
func (trainer *Trainer) replayTape(
	ticks []*data.Value,
	bTick, dTick uint64,
) (precursorPath, holdingPath string, err error) {
	grid := nmstore.NewGrid(trainer.Context())
	grammar := NewGrammar()
	coder := newPredictiveCoder()
	step := 0
	lastSym := ""
	recentTokens := make([]string, 0, 16)
	lastPhrase := ""

	for _, val := range ticks {
		regions, observeErr := grid.Observe(val)

		if observeErr != nil {
			continue
		}

		if len(regions) == 0 {
			continue
		}

		regionBrightness := make([]float64, 15)

		for _, r := range regions {
			if int(r.Key) < 15 {
				regionBrightness[r.Key] = r.Brightness
			}
		}

		step++
		sec := float64(val.Timestamp) / 1e9

		if val.Timestamp < 1e11 {
			sec = float64(val.Timestamp)
		}

		inValue := transport.NewValue([2][]float64{
			regionBrightness,
			{0.0, 0.0, float64(step), sec},
		})

		for range coder.Next(inValue.Next(nil)) {
		}

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
			trainer.snapshotPaths(val.Tick, bTick, dTick, lastPhrase, &precursorPath, &holdingPath)
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

			if len(phrase) >= 1 {
				activePath = strings.Join(phrase, "/")
			}
		}

		if activePath == "" && len(recentTokens) >= 2 {
			activePath = strings.Join(recentTokens[len(recentTokens)-2:], "/")
		} else if activePath == "" && len(recentTokens) == 1 {
			activePath = recentTokens[0]
		}

		if activePath != "" {
			lastPhrase = activePath
		}

		trainer.snapshotPaths(val.Tick, bTick, dTick, lastPhrase, &precursorPath, &holdingPath)
	}

	if gridErr := grid.Error(); gridErr != nil {
		return "", "", errnie.Error(gridErr)
	}

	return precursorPath, holdingPath, nil
}

func (trainer *Trainer) snapshotPaths(
	tick, bTick, dTick uint64,
	phrase string,
	precursorPath, holdingPath *string,
) {
	if phrase == "" {
		return
	}

	if tick <= bTick {
		*precursorPath = phrase
	}

	if tick >= bTick && tick <= dTick {
		*holdingPath = phrase
	}
}

func (trainer *Trainer) writePolicy(
	epoch int64,
	symbol string,
	path string,
	action string,
	netReturn float64,
) error {
	if path == "" || trainer.client == nil {
		return nil
	}

	if action != "enter" && action != "exit" && action != "noop" {
		return nil
	}

	val := data.NewValue("policy", action, symbol)
	val.Epoch = uint64(epoch)
	data.AppendVector(val, []float64{
		netReturn,
		0,
		1,
		netReturn,
		1,
	}, "gain", "hold_seconds", "visits", "mean_reward", "confidence", "path:"+path)

	val.FormatPrefix()
	val.Prefix = "policy/current/" + path + "/" + action + ".bin"

	trainer.writeMu.Lock()
	defer trainer.writeMu.Unlock()

	if _, err := io.Copy(trainer.client, val); err != nil {
		return errnie.Error(err)
	}

	errnie.Info(fmt.Sprintf("[trainer] policy %s/%s → %s", path, action, val.Prefix))

	return nil
}
