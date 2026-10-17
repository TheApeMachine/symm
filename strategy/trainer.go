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
	"github.com/theapemachine/symm/nomagique/runtime"
	nmstore "github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/store"
	"golang.org/x/sync/errgroup"
)

type Trainer struct {
	*runtime.System
	client       *store.Client
	price        *broker.Price
	symbols      []string
	OnFrame      func()
	OnDescribe   func(string)
	SetWorkTotal func(int64)
	writeMu      sync.Mutex
	priceMu      sync.Mutex
	tickIndexMu  sync.Mutex
	tickIndexes  map[int64]map[uint64]string
	tickCacheMu  sync.Mutex
	tickCache    map[int64]map[uint64]*data.Value
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
ExcursionKeyCount is listed detections/ objects for the epoch (before GET filtering).
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
parseDetectionStorageKey reads class and epoch from
detections/<class>/…/<epoch>/<tick>.bin.
*/
func parseDetectionStorageKey(key string) (class string, epoch uint64, ok bool) {
	head := data.DetectionsOrigin + "/"

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
detectionJobs lists committed detections under detections/<class>/….
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
		prefix := data.DetectionsOrigin + "/" + class + "/"

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
	sem := make(chan struct{}, 2)

	for i, job := range jobs {
		excursion := loaded[i]

		group.Go(func() error {
			sem <- struct{}{}

			defer func() { <-sem }()

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

	fields, err := excursionMetrics(excursion)

	if err != nil {
		return nil
	}

	aTick := uint64(fields["a_tick"])
	bTick := uint64(fields["b_tick"])
	cTick := uint64(fields["c_tick"])
	dTick := uint64(fields["d_tick"])

	if aTick == 0 || bTick < aTick || cTick < bTick {
		return nil
	}

	tickIndex := trainer.tickIndexForEpoch(epoch)
	precursorSpan := bTick - aTick + 1
	holdingSpan := cTick - bTick + 1

	trainer.describe(fmt.Sprintf(
		"train %s %s precursor=%d holding=%d ticks",
		excursion.Scope, excursion.Role, precursorSpan, holdingSpan,
	))
	errnie.Info(fmt.Sprintf(
		"[trainer] begin %s %s a=%d b=%d c=%d (precursor %d ticks, holding %d ticks)",
		excursion.Scope, excursion.Role, aTick, bTick, cTick, precursorSpan, holdingSpan,
	))

	ticks, missing, loadErr := trainer.loadTicksCounted(epoch, tickIndex, aTick, cTick)

	if loadErr != nil {
		return errnie.Error(loadErr)
	}

	if len(ticks) == 0 {
		errnie.Info(fmt.Sprintf(
			"[trainer] %s: no ticks in [%d,%d] (a=%d d=%d missing=%d index=%d)",
			tag, aTick, cTick, aTick, cTick, missing, len(tickIndex),
		))
		return nil
	}

	symbol := excursion.Scope

	precursorPath, holdingPath, precursorTicks, holdingTicks, replayErr := trainer.replayTape(
		tag, symbol, ticks, aTick, bTick, cTick,
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

	// One trade: buy at B, sell at C. The precursor path is where that
	// trade is entered and the holding path is where it is exited, so
	// both actions follow the same round trip. A→B is the pattern that
	// leads into the entry, not a second trade.
	trades := excursion.Role == "up" &&
		trainer.clearsFriction(label, ticks, bPrice, cPrice)

	wrote, err := trainer.publishExcursion(
		epoch, label, precursorPath, holdingPath,
		trades, trades, netReturn,
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
publishExcursion writes the fragment paths of one trade. When that up
round trip (B→C) clears friction, the precursor path is enter.bin and the
holding path is exit.bin. Anything else is left unmatched. An exit is
never written unless the same excursion also writes its enter.
*/
func (trainer *Trainer) publishExcursion(
	epoch int64,
	label, precursorPath, holdingPath string,
	precursorClears, holdingClears bool,
	gain float64,
) (string, error) {
	type file struct {
		path   string
		action string
	}

	var files []file

	enter := precursorClears && precursorPath != ""

	if enter {
		files = append(files, file{precursorPath, "enter"})
	}

	if enter && holdingClears && holdingPath != "" {
		files = append(files, file{holdingPath, "exit"})
	}

	if len(files) == 0 {
		return "no file", nil
	}

	names := make([]string, 0, len(files))

	for _, file := range files {
		if err := trainer.writePolicy(epoch, label, file.path, file.action, gain); err != nil {
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

func (trainer *Trainer) describe(msg string) {
	if trainer != nil && trainer.OnDescribe != nil {
		trainer.OnDescribe(msg)
	}
}

func excursionMetrics(value *data.Value) (map[string]float64, error) {
	if value == nil {
		return nil, errnie.Err(errnie.Validation, "[trainer] no excursion", nil)
	}

	if value.Origin != data.DetectionsOrigin {
		return nil, errnie.Err(
			errnie.Validation,
			"[trainer] train reads only committed detections, not "+value.Origin,
			nil,
		)
	}

	metrics := data.Metrics(value)

	for _, key := range []string{"a_tick", "b_tick", "c_tick", "b_price", "c_price"} {
		if _, ok := metrics[key]; !ok {
			return nil, errnie.Err(
				errnie.UnprocessableContent,
				"[trainer] excursion "+value.Prefix+" has no "+key,
				nil,
			)
		}
	}

	return metrics, nil
}

/*
clearsFriction is true when a low→high move on symbol clears round-trip fees.
*/
func (trainer *Trainer) clearsFriction(
	symbol string,
	ticks []*data.Value,
	low, high *decimal.Decimal,
) bool {
	if trainer == nil || trainer.price == nil || low == nil || high == nil ||
		low.Sign() <= 0 || high.Cmp(low) <= 0 {
		return false
	}

	pnl, _, err := trainer.priceRoundTrip(symbol, ticks, low, high)

	return err == nil && pnl != nil && pnl.Sign() > 0
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

func (trainer *Trainer) loadTicksCounted(
	epoch int64,
	index map[uint64]string,
	fromTick, toTick uint64,
) ([]*data.Value, int, error) {
	vals := make([]*data.Value, 0, toTick-fromTick+1)
	missing := 0
	fetchKeys := make([]string, 0)
	fetchTicks := make([]uint64, 0)

	for tick := fromTick; tick <= toTick; tick++ {
		if cached, ok := trainer.cachedTick(epoch, tick); ok {
			vals = append(vals, cached)
			continue
		}

		key, ok := index[tick]

		if !ok {
			missing++
			continue
		}

		fetchKeys = append(fetchKeys, key)
		fetchTicks = append(fetchTicks, tick)
	}

	if len(fetchKeys) > 0 {
		trainer.describe(fmt.Sprintf("loading %d workspace ticks", len(fetchKeys)))
	}

	at := 0

	for val, err := range store.ParallelValues(trainer.client, fetchKeys) {
		if err != nil {
			return nil, missing, err
		}

		tick := fetchTicks[at]
		at++
		val.Tick = tick
		trainer.storeCachedTick(epoch, tick, val)
		vals = append(vals, val)
	}

	slices.SortFunc(vals, func(first, second *data.Value) int {
		return cmp.Compare(first.Tick, second.Tick)
	})

	return vals, missing, nil
}

func (trainer *Trainer) cachedTick(epoch int64, tick uint64) (*data.Value, bool) {
	trainer.tickCacheMu.Lock()
	defer trainer.tickCacheMu.Unlock()

	if trainer.tickCache == nil {
		return nil, false
	}

	byTick, ok := trainer.tickCache[epoch]

	if !ok {
		return nil, false
	}

	val, ok := byTick[tick]

	return val, ok
}

func (trainer *Trainer) storeCachedTick(epoch int64, tick uint64, val *data.Value) {
	if val == nil {
		return
	}

	trainer.tickCacheMu.Lock()
	defer trainer.tickCacheMu.Unlock()

	if trainer.tickCache == nil {
		trainer.tickCache = make(map[int64]map[uint64]*data.Value)
	}

	if trainer.tickCache[epoch] == nil {
		trainer.tickCache[epoch] = make(map[uint64]*data.Value)
	}

	trainer.tickCache[epoch][tick] = val
}

/*
replayTape runs two isolated fragments: precursor A→B and holding B→C. Each
has its own grid and Sequitur grammar. Every tick in the window is observed
on that fragment's grid. Regions at or above the midpoint of the brightest
and dimmest lit region become the chord.
*/
func (trainer *Trainer) replayTape(
	label, symbol string,
	ticks []*data.Value,
	aTick, bTick, cTick uint64,
) (precursorPath, holdingPath string, precursorTicks, holdingTicks int, err error) {
	ctx := context.Background()

	if trainer != nil && trainer.System != nil {
		ctx = trainer.Context()
	}

	precursorPath, precursorTicks, err = trainer.replayFragment(ctx, label, symbol, "precursor", ticks, aTick, bTick)

	if err != nil {
		return "", "", precursorTicks, holdingTicks, err
	}

	holdingPath, holdingTicks, err = trainer.replayFragment(ctx, label, symbol, "holding", ticks, bTick, cTick)

	return precursorPath, holdingPath, precursorTicks, holdingTicks, err
}

func (trainer *Trainer) replayFragment(
	ctx context.Context,
	label, symbol, phase string,
	ticks []*data.Value,
	fromTick, toTick uint64,
) (path string, count int, err error) {
	grid := nmstore.NewGrid(ctx)
	fragment := newFragment(grid)
	span := toTick - fromTick + 1

	for _, val := range ticks {
		if val == nil || val.Tick < fromTick || val.Tick > toTick {
			continue
		}

		count++

		if count == 1 || count%1000 == 0 || count == int(span) {
			trainer.describe(fmt.Sprintf("%s %s tick %d/%d", label, phase, count, span))

			if count%1000 == 0 || count == int(span) {
				errnie.Info(fmt.Sprintf("[trainer] %s %s at tick %d/%d", label, phase, count, span))
			}
		}

		regions, observeErr := fragment.observe(val)

		if observeErr != nil {
			return "", count, errnie.Error(observeErr)
		}

		fragment.consume(symbol, val, regions)
	}

	if gridErr := grid.Error(); gridErr != nil {
		return "", count, errnie.Error(gridErr)
	}

	return fragment.lastPhrase, count, nil
}

/*
fragment is one A→B or B→C window: grid and grammar private to that window.
*/
type fragment struct {
	grid       *nmstore.Grid
	grammar    *Grammar
	lastSym    string
	lastPhrase string
	sentence   []string
	regime     regime
}

func newFragment(grid *nmstore.Grid) *fragment {
	return &fragment{
		grid:    grid,
		grammar: NewGrammar(),
	}
}

func (fragment *fragment) observe(val *data.Value) ([]*nmstore.Region, error) {
	return fragment.grid.Observe(val)
}

/*
consume appends the chord of regions at or above the brightness midpoint.
The fragment's phrase is the terminal expansion of a rule Sequitur completed.
A chord that completes no rule leaves the phrase unchanged.
*/
func (fragment *fragment) consume(label string, val *data.Value, regions []*nmstore.Region) {
	if fragment == nil {
		return
	}

	if fragment.regime.advance(fragment.grid.Energy(label)) {
		fragment.sentence = nil
		fragment.lastSym = ""
		fragment.lastPhrase = ""
	}

	if val == nil || len(regions) == 0 {
		return
	}

	active := make([]*nmstore.Region, 0, len(regions))

	for _, region := range regions {
		if region != nil && region.Brightness > 0 {
			active = append(active, region)
		}
	}

	if len(active) == 0 {
		return
	}

	midB := (active[0].Brightness + active[len(active)-1].Brightness) / 2.0
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
		fragment.sentence = append(fragment.sentence, token)

		if rule := fragment.grammar.Append(token); rule != nil {
			deepest = rule
		}
	}

	if deepest == nil {
		return
	}

	phrase := deepest.TerminalNames(fragment.grammar)

	if len(phrase) < 2 || !phraseInSentence(phrase, fragment.sentence) {
		return
	}

	fragment.lastPhrase = strings.Join(phrase, "/")
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

	if action != "enter" && action != "exit" {
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
