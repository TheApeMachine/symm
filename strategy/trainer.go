package strategy

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/learning"
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
	priceMu      sync.Mutex
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

func (trainer *Trainer) loadExcursions(epoch int64) []*data.Value {
	jobs := trainer.detectionJobs(epoch)
	keys := make([]string, 0, len(jobs))

	for _, job := range jobs {
		if job.epoch == epoch {
			keys = append(keys, job.key)
		}
	}

	out := make([]*data.Value, 0, len(keys))

	for val, err := range store.ParallelValues(trainer.client, keys) {
		if err != nil {
			errnie.Error(errnie.Err(errnie.IO, "[trainer] excursion", err))
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

	keys := make([]string, len(jobs))

	for i, job := range jobs {
		keys[i] = job.key
	}

	loaded := make([]*data.Value, len(jobs))
	at := 0

	for excursion, err := range store.ParallelValues(trainer.client, keys) {
		if err != nil {
			errnie.Error(errnie.Err(errnie.IO, "[trainer] excursion "+keys[at], err))
			at++
			continue
		}

		loaded[at] = excursion
		at++
	}

	group, ctx := errgroup.WithContext(trainer.Context())

	for i, job := range jobs {
		excursion := loaded[i]

		group.Go(func() error {
			if trainer.OnFrame != nil {
				defer trainer.OnFrame()
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			if excursion == nil {
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
	cTick := uint64(fields["c_tick"])
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

	precursorPath, holdingPath, precursorTicks, holdingTicks, replayErr := trainer.replayTape(
		ticks, aTick, bTick, dTick,
	)

	if replayErr != nil {
		return errnie.Error(replayErr)
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

	pnl, cost, roundErr := trainer.priceRoundTrip(label, ticks, bPrice, cPrice)
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

	wrote, err := trainer.publishExcursion(
		epoch, label, excursion.Role, precursorPath, holdingPath, netReturn,
	)

	if err != nil {
		return errnie.Error(err)
	}

	errnie.Info(fmt.Sprintf(
		"[trainer] %s %s a=%d b=%d c=%d d=%d missing=%d precursor ticks=%d path=%s holding ticks=%d path=%s net=%.4g → %s",
		label, excursion.Role, aTick, bTick, cTick, dTick, missing,
		precursorTicks, pathOrNone(precursorPath),
		holdingTicks, pathOrNone(holdingPath),
		netReturn, wrote,
	))

	return nil
}

/*
publishExcursion writes the two fragment sequences. A profitable up excursion
enters on the precursor path and exits on the holding path. Anything else is
a noop. An empty sequence is not a file.
*/
func (trainer *Trainer) publishExcursion(
	epoch int64,
	label, role, precursorPath, holdingPath string,
	netReturn float64,
) (string, error) {
	type file struct {
		path   string
		action string
	}

	var files []file

	if role == "up" && netReturn > 0 {
		if precursorPath != "" {
			files = append(files, file{precursorPath, "enter"})
		}

		if holdingPath != "" {
			files = append(files, file{holdingPath, "exit"})
		}
	} else {
		seen := map[string]bool{}

		for _, path := range []string{precursorPath, holdingPath} {
			if path == "" || seen[path] {
				continue
			}

			seen[path] = true
			files = append(files, file{path, "noop"})
		}
	}

	if len(files) == 0 {
		return "no file", nil
	}

	names := make([]string, 0, len(files))

	for _, file := range files {
		if err := trainer.writePolicy(epoch, label, file.path, file.action, netReturn); err != nil {
			return "", err
		}

		names = append(names, file.action+" "+policyObjectKey(file.path, file.action))
	}

	return strings.Join(names, ", "), nil
}

func pathOrNone(path string) string {
	if path == "" {
		return "none"
	}

	return path
}

/*
priceRoundTrip sizes the excursion from the instrument layer on the loaded
ticks. The normalizer starts empty in offline training, and its update
replaces every pair, so install and the round trip stay on one lock.
*/
func (trainer *Trainer) priceRoundTrip(
	symbol string,
	ticks []*data.Value,
	entry, exit *decimal.Decimal,
) (*decimal.Decimal, *decimal.Decimal, error) {
	if trainer == nil || trainer.price == nil {
		return nil, nil, errnie.Error(errnie.Err(
			errnie.Validation, "[trainer] price required to size "+symbol, nil,
		))
	}

	trainer.priceMu.Lock()
	defer trainer.priceMu.Unlock()

	installInstrument(trainer.price, symbol, ticks)

	return trainer.price.RoundTrip(symbol, entry, exit)
}

/*
installInstrument teaches the normalizer the latest instrument/<symbol> layer
on ticks, which is the venue lot rule captured with the tape.
*/
func installInstrument(price *broker.Price, symbol string, ticks []*data.Value) {
	if price == nil || symbol == "" {
		return
	}

	var raw []byte

	for _, tick := range ticks {
		if layer := broker.SymbolLayer(tick, data.LayerInstrument, symbol); layer != nil {
			raw = layer
		}
	}

	if raw == nil {
		return
	}

	var pair kraken.InstrumentPair

	if err := sonic.Unmarshal(raw, &pair); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent, "[trainer] instrument/"+symbol, err,
		))
		return
	}

	if pair.Symbol == "" {
		pair.Symbol = symbol
	}

	price.InstallPair(pair)
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
	ordered := make([]struct {
		tick uint64
		key  string
	}, 0, toTick-fromTick+1)

	for tick := fromTick; tick <= toTick; tick++ {
		key, ok := index[tick]

		if !ok {
			missing++
			continue
		}

		ordered = append(ordered, struct {
			tick uint64
			key  string
		}{tick, key})
	}

	keys := make([]string, len(ordered))

	for i, item := range ordered {
		keys[i] = item.key
	}

	at := 0

	for val, err := range store.ParallelValues(trainer.client, keys) {
		if err != nil {
			return nil, missing, err
		}

		// The object name is the tick detect stored. A header that did not
		// round-trip would make every frame look earlier than b_tick, so the
		// holding window would never run and exit.bin would not be written.
		val.Tick = ordered[at].tick
		at++
		vals = append(vals, val)
	}

	slices.SortFunc(vals, func(first, second *data.Value) int {
		return cmp.Compare(first.Tick, second.Tick)
	})

	return vals, missing, nil
}

/*
replayTape splits the loaded tape into a precursor fragment [a, b] and a
holding fragment [b, d]. Ticks before a only warm the grid. Each fragment has
its own predictive coder and Sequitur grammar. The boundary tick is observed
once and fed to both grammars. The two return counts are stored ticks in each
fragment, and the two paths are the sequences those fragments settled.
*/
func (trainer *Trainer) replayTape(
	ticks []*data.Value,
	aTick, bTick, dTick uint64,
) (precursorPath, holdingPath string, precursorTicks, holdingTicks int, err error) {
	ctx := context.Background()

	if trainer != nil && trainer.System != nil {
		ctx = trainer.Context()
	}

	grid := nmstore.NewGrid(ctx)
	precursor := newFragment(grid)
	holding := newFragment(grid)

	for _, val := range ticks {
		if val == nil || val.Tick > dTick {
			continue
		}

		if val.Tick < aTick {
			if _, observeErr := grid.Observe(val); observeErr != nil {
				return "", "", precursorTicks, holdingTicks, errnie.Error(observeErr)
			}

			continue
		}

		var regions []*nmstore.Region

		if val.Tick <= bTick {
			precursorTicks++
			observed, observeErr := precursor.observe(val)

			if observeErr != nil {
				return "", "", precursorTicks, holdingTicks, errnie.Error(observeErr)
			}

			regions = observed
			precursor.consume(val, regions)
		}

		if val.Tick >= bTick {
			holdingTicks++

			if val.Tick > bTick {
				observed, observeErr := holding.observe(val)

				if observeErr != nil {
					return "", "", precursorTicks, holdingTicks, errnie.Error(observeErr)
				}

				regions = observed
			}

			holding.consume(val, regions)
		}
	}

	if gridErr := grid.Error(); gridErr != nil {
		return "", "", precursorTicks, holdingTicks, errnie.Error(gridErr)
	}

	return precursor.lastPhrase, holding.lastPhrase, precursorTicks, holdingTicks, nil
}

/*
fragment is one tape window: its own coder and grammar, on a grid shared with
the other window so priors survive from the warmup into the hold.
*/
type fragment struct {
	grid         *nmstore.Grid
	grammar      *Grammar
	coder        *learning.PredictiveCoder
	step         int
	lastSym      string
	recentTokens []string
	lastPhrase   string
}

func newFragment(grid *nmstore.Grid) *fragment {
	return &fragment{
		grid:         grid,
		grammar:      NewGrammar(),
		coder:        newPredictiveCoder(),
		recentTokens: make([]string, 0, 16),
	}
}

func (fragment *fragment) observe(val *data.Value) ([]*nmstore.Region, error) {
	return fragment.grid.Observe(val)
}

/*
consume gates the lit regions through this fragment's predictive coder and
appends the chord to its grammar. The settled phrase is the fragment's token
sequence.
*/
func (fragment *fragment) consume(val *data.Value, regions []*nmstore.Region) {
	if fragment == nil || val == nil || len(regions) == 0 {
		return
	}

	regionBrightness := make([]float64, 15)

	for _, region := range regions {
		if int(region.Key) < 15 {
			regionBrightness[region.Key] = region.Brightness
		}
	}

	fragment.step++
	sec := float64(val.Timestamp) / 1e9

	if val.Timestamp < 1e11 {
		sec = float64(val.Timestamp)
	}

	inValue := transport.NewValue([2][]float64{
		regionBrightness,
		{0.0, 0.0, float64(fragment.step), sec},
	})

	for range fragment.coder.Next(inValue.Next(nil)) {
	}

	active := make([]*nmstore.Region, 0, len(regions))

	for _, region := range regions {
		if region.Brightness > 0 {
			active = append(active, region)
		}
	}

	if len(active) == 0 {
		return
	}

	maxB := active[0].Brightness
	minB := active[len(active)-1].Brightness
	midB := (maxB + minB) / 2.0
	fired := make([]string, 0, len(active))

	for _, region := range active {
		if region.Brightness >= midB {
			fired = append(fired, fmt.Sprintf("R%02d", region.Key))
		}
	}

	if len(fired) == 0 {
		return
	}

	chord := strings.Join(fired, "/")

	if chord == fragment.lastSym {
		return
	}

	fragment.lastSym = chord
	var deepest *Rule

	for _, token := range fired {
		if rule := fragment.grammar.Append(token); rule != nil {
			deepest = rule
		}
	}

	fragment.recentTokens = append(fragment.recentTokens, fired...)

	if len(fragment.recentTokens) > 16 {
		fragment.recentTokens = fragment.recentTokens[len(fragment.recentTokens)-16:]
	}

	path := ""

	if deepest != nil {
		if phrase := deepest.TerminalNames(fragment.grammar); len(phrase) >= 1 {
			path = strings.Join(phrase, "/")
		}
	}

	if path == "" && len(fragment.recentTokens) >= 2 {
		path = strings.Join(fragment.recentTokens[len(fragment.recentTokens)-2:], "/")
	} else if path == "" && len(fragment.recentTokens) == 1 {
		path = fragment.recentTokens[0]
	}

	if path != "" {
		fragment.lastPhrase = path
	}
}

/*
policyObjectKey is the object key of one learned action. The store key is this
string, not the Value prefix origin/role/scope/epoch/tick.bin.
*/
func policyObjectKey(path, action string) string {
	return "policy/current/" + path + "/" + action + ".bin"
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

	key := policyObjectKey(path, action)

	trainer.writeMu.Lock()
	defer trainer.writeMu.Unlock()

	if err := trainer.client.Put(key, val.Bytes()); err != nil {
		return errnie.Error(err)
	}

	errnie.Info(fmt.Sprintf("[trainer] policy %s/%s → %s", path, action, key))

	return nil
}
