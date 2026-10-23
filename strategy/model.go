package strategy

import (
	"context"
	"io"
	"math"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmstore "github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/strategy/tokenizer"
)

// Desk is the trading surface the Model acts through.
// *broker.Desk satisfies this interface.
type Desk interface {
	Has(symbol string, virtual bool) bool
	Enter(symbol string, path string, virtual bool, tick uint64) error
	Exit(symbol string, path string, virtual bool, tick uint64, triggers ...string) error
	SaleReturn(symbol string, qty, entry, fee *decimal.Decimal) (float64, bool)
	Virtual(symbol string) *broker.Position
	Positions(open bool) []*broker.Position
	Profit(symbol string, virtual bool) (bool, bool)
	ClosedVirtuals() []*broker.Position
	Drop(symbol string)
	Friction(symbol string) (float64, bool)
	OpenMark(symbol string) (float64, bool)
	Quote(symbol string) (qty, entry, fee *decimal.Decimal, err error)
	Realize(symbol string, qty, entry, fee *decimal.Decimal) (float64, bool)
	Occupied(symbol string) bool
	Open(symbol string) *broker.Position
	ExitOccupied(symbol, path string, tick uint64) error
}

/*
pathScore is the persisted ledger for one path and one role. Sum and
Count are the in-sample fee-net returns (a fraction, from the book walk).
Frozen means that score is closed and only confirmation closes remain.
Retired means the mean did not clear friction; the path is not entered
again, including after a restart.
*/
type pathScore struct {
	Sum     float64 `json:"sum"`
	Count   int     `json:"count"`
	Confirm int     `json:"confirm"`
	Frozen  bool    `json:"frozen"`
	Retired bool    `json:"retired"`
}

// excursion is one open ledger sample. It does not occupy the desk slot.
type excursion struct {
	entryPath string
	exitPath  string
	qty       *decimal.Decimal
	entry     *decimal.Decimal
	fee       *decimal.Decimal
	// anchor is the fee-net book return at the open print. The hurdle is
	// measured from this reading, so the spread already in the fill is not
	// itself an exit.
	anchor   float64
	anchored bool
	cause    string
}

// coolDown blocks a second sample of the sentence that just closed until
// a regime token arrives that was not part of that entry path.
type coolDown struct {
	tokens map[string]struct{}
}

type tickReading struct {
	maturity       float64
	surprise       float64
	hasSurprise    bool
	z              float64
	hasZ           bool
	frictionPass   bool
	frictionHurdle float64
	ignitionPass   bool
}

// surpriseTrack is one symbol's running energy-error distribution.
// z is the new error against the errors already seen, and it stays
// absent until two of them exist so a spread is defined.
type surpriseTrack struct {
	n    int
	mean float64
	m2   float64
}

// regimeNote is the regime token emitted on a tick, kept only so that
// tick's chord can be compared with the terminals that closed it.
type regimeNote struct {
	tick    uint64
	symbols []string
}

type Model struct {
	*runtime.System `json:"-"`
	mu              sync.Mutex
	cfg             *Config
	desk            Desk
	store           *store.Client
	hub             io.Writer
	metrics         *metricsAccumulator
	tree            *iradix.Tree[*data.Value]
	explored        *iradix.Tree[*data.Value]
	sequencers      *sync.Map
	paths           *sync.Map
	seqPaths        *sync.Map
	settled         *sync.Map
	ingress         *sync.Map
	tickReadings    *sync.Map
	lastRegime      sync.Map
	surpriseTracks  sync.Map
	excursions      map[string]*excursion
	pathAnchorTick  map[string]uint64
	// tape is the workspace value's own tick for the step in progress.
	// Path anchors, entries, and exits stamp this, not the ingress counter.
	tape        atomic.Uint64
	venueAnchor map[string]float64
	cool        map[string]*coolDown
}

func NewModel(
	ctx context.Context, desk Desk, client *store.Client,
	hub io.Writer, cfg *Config,
) *Model {
	if cfg == nil {
		cfg = LoadConfig()
	}

	model := &Model{
		System:         runtime.NewSystem(ctx, "model"),
		cfg:            cfg,
		desk:           desk,
		store:          client,
		hub:            hub,
		tree:           iradix.New[*data.Value](),
		explored:       iradix.New[*data.Value](),
		sequencers:     &sync.Map{},
		paths:          &sync.Map{},
		seqPaths:       &sync.Map{},
		settled:        &sync.Map{},
		ingress:        &sync.Map{},
		tickReadings:   &sync.Map{},
		excursions:     map[string]*excursion{},
		pathAnchorTick: map[string]uint64{},
		venueAnchor:    map[string]float64{},
		cool:           map[string]*coolDown{},
	}

	policyPrefix := strings.TrimSuffix(cfg.PolicyPrefix, "/")

	if client != nil {
		for prefix := range client.ListKeys(policyPrefix) {
			// Only load enter/exit keys, not noop.
			if !strings.Contains(prefix, "/enter") && !strings.Contains(prefix, "/exit") {
				continue
			}
			value := client.Download(ctx, prefix)
			if value == nil {
				continue
			}

			var path string
			value.Lookup(&path, "path")
			if path == "" {
				continue
			}

			errnie.Debug("[model.NewModel]", "prefix", prefix, "path", path)

			model.tree, _, _ = model.tree.Insert([]byte(path), value)
		}

		for prefix := range client.ListKeys("policy/explored") {
			value := client.Download(ctx, prefix)
			if value == nil {
				continue
			}

			role := value.Role
			if role != "enter" && role != "exit" {
				model.Error(errnie.Err(
					errnie.Validation,
					"[model] explored object "+prefix+" has no enter or exit role",
					nil,
				))
				continue
			}

			var path string
			var score pathScore
			value.Lookup(&path, "path")
			value.Lookup(&score, "score")

			if path == "" || (score.Count == 0 && !score.Retired && !score.Frozen) {
				model.Error(errnie.Err(
					errnie.Validation,
					"[model] explored object "+prefix+" has no path score",
					nil,
				))
				continue
			}

			model.explored, _, _ = model.explored.Insert(
				[]byte(exploredKey(role, path)), value,
			)
		}
	}

	return model
}

func (model *Model) BindMetrics(acc *metricsAccumulator) {
	model.metrics = acc
}

func (model *Model) policyPrefix() string {
	return strings.TrimSuffix(model.cfg.PolicyPrefix, "/")
}

/*
noteTape binds the workspace value's tick for this step. Anchors, entries,
and exits read it so a consumer running behind ingress does not collapse
several chords onto one counter value.
*/
func (model *Model) noteTape(tick uint64) {
	if model == nil {
		return
	}

	model.tape.Store(tick)
}

func (model *Model) requireTape() (uint64, error) {
	tick := model.tape.Load()
	if tick == 0 {
		return 0, errnie.Err(errnie.Validation, "[model] tape tick is unset", nil)
	}

	return tick, nil
}

func (model *Model) NoteTick(
	symbol string,
	regions []*nmstore.Region,
	grid *nmstore.Grid,
) {
	if symbol == "" || grid == nil {
		return
	}

	reading := tickReading{frictionPass: true, ignitionPass: !model.cfg.RequireIgnitionRegime}

	if model.cfg.RequireFrictionGate {
		reading.frictionPass = false

		for _, region := range regions {
			if region == nil || region.Key != 12 {
				continue
			}

			if region.Brightness >= model.cfg.FrictionMinBrightness {
				reading.frictionPass = true
				break
			}
		}
	}

	for _, region := range regions {
		if region == nil {
			continue
		}

		if region.Key == 12 {
			reading.frictionHurdle = region.Brightness
		}

		if region.Key == 7 && region.Brightness >= model.cfg.IgnitionMinBrightness {
			reading.ignitionPass = true
		}
	}

	if surprise, ok := grid.SymbolSurprise(symbol); ok {
		reading.surprise = surprise
		reading.hasSurprise = true
		reading.z, reading.hasZ = model.scoreSurprise(symbol, surprise)
	}

	for _, bySymbol := range grid.Priors {
		signal, ok := bySymbol[symbol]
		if !ok || signal == nil {
			continue
		}

		maturity := grid.Maturity(&signal.Signal)
		if maturity > reading.maturity {
			reading.maturity = maturity
		}
	}

	model.tickReadings.Store(symbol, reading)
}

func (model *Model) scoreSurprise(symbol string, surprise float64) (float64, bool) {
	var track *surpriseTrack

	if existing, ok := model.surpriseTracks.Load(symbol); ok {
		track = existing.(*surpriseTrack)
	} else {
		track = &surpriseTrack{}
		model.surpriseTracks.Store(symbol, track)
	}

	var z float64
	has := false

	if track.n >= 2 {
		has = true
		variance := track.m2 / float64(track.n-1)
		if variance > 0 {
			z = (surprise - track.mean) / math.Sqrt(variance)
		}
	}

	track.n++
	delta := surprise - track.mean
	track.mean += delta / float64(track.n)
	track.m2 += delta * (surprise - track.mean)

	return z, has
}

func (model *Model) TrackSequitur(symbol string, token tokenizer.Token) {
	if len(token.Symbols) == 0 {
		model.lastRegime.Delete(symbol)
		return
	}

	copied := append([]string(nil), token.Symbols...)
	model.lastRegime.Store(symbol, regimeNote{
		tick:    model.tape.Load(),
		symbols: copied,
	})

	model.mu.Lock()
	defer model.mu.Unlock()

	untyped, ok := model.seqPaths.Load(symbol)
	var path tokenizer.Tokens
	if ok {
		path = untyped.(tokenizer.Tokens)
	}

	path = append(path, token)

	if len(path) > model.cfg.MaxPathLength {
		path = path[len(path)-model.cfg.MaxPathLength:]
	}

	model.seqPaths.Store(symbol, path)
}

func (model *Model) sequiturPathKey(symbol string) string {
	untyped, ok := model.seqPaths.Load(symbol)
	if !ok {
		return ""
	}

	return untyped.(tokenizer.Tokens).String()
}

/*
Ingest feeds one tick's lit regions for a symbol into that symbol's
grammar. The returned token is one regime: the terminals of a rule the
grammar just formed. A zero token means the chord has not repeated yet,
so it is not a path step.
*/
func (model *Model) Ingest(symbol string, regions []tokenizer.Token) tokenizer.Token {
	if symbol == "" || len(regions) == 0 {
		return tokenizer.Token{}
	}

	untyped, _ := model.sequencers.LoadOrStore(symbol, tokenizer.NewSequencer())
	sequencer := untyped.(*tokenizer.Sequencer)
	token := sequencer.Analyze(regions)
	model.ingress.Store(symbol, sequencer.Grammar().LastKind())

	return token
}

/*
Touch publishes the symbol's grammar on a tick that did not close a
regime, so the dashboard still sees the chord.
*/
func (model *Model) Touch(symbol string) {
	untyped, ok := model.sequencers.Load(symbol)
	if !ok {
		return
	}

	model.mu.Lock()
	path := model.currentPath(symbol)
	model.mu.Unlock()

	model.publishMatch(symbol, path, "noop")
	model.publishGrammar(
		symbol, untyped.(*tokenizer.Sequencer), tokenizer.Token{},
		path.String(), model.grammarKind(symbol), "noop",
	)
}

/*
Evaluate processes one regime token for one symbol and returns the
model's action, or nil when no action is taken.

The token is one trie edge. In order:
 1. A token from the sentence just sampled is ignored until a different
    regime shows the sentence has ended. That separator is not a path step.
 2. An open ledger excursion is left to develop. It closes later, from the
    fee-net book return, when that return has moved one friction hurdle
    from the open print: up through a profit, or down by the same amount.
 3. The longest stored prefix of the path decides. A confirmed enter may
    open a venue order. A confirmed exit closes the open lot.
 4. Otherwise an enter-side path of at least minPathLength opens a ledger
    excursion, which does not take the symbol's position slot.

Precursor (A→B) and holding (B→C) are separate trie walks: opening a
sample or a venue lot resets the symbol path so exit keys are only the
holding fragment, never enter plus exit on one terminal prefix.
*/
func (model *Model) Evaluate(symbol string, token tokenizer.Token) *data.Value {
	if len(token.Symbols) == 0 {
		return nil
	}

	untyped, _ := model.sequencers.LoadOrStore(symbol, tokenizer.NewSequencer())
	sequencer := untyped.(*tokenizer.Sequencer)

	reported := "noop"
	var path tokenizer.Tokens
	var pathKey string

	defer func() {
		model.publishMatch(symbol, path, reported)
		model.publishGrammar(
			symbol, sequencer, token, pathKey, model.grammarKind(symbol), reported,
		)
	}()

	model.mu.Lock()

	if cool := model.cool[symbol]; cool != nil {
		if _, known := cool.tokens[token.String()]; known {
			path = model.currentPath(symbol)
			pathKey = path.String()
			model.mu.Unlock()
			return nil
		}

		delete(model.cool, symbol)
		path = model.currentPath(symbol)
		pathKey = path.String()
		model.mu.Unlock()
		return nil
	}

	path = model.appendPath(symbol, token)
	pathKey = path.String()
	ex := model.excursions[symbol]
	side := tokenSide(token)

	if ex != nil {
		// Tokens after the open are the holding fragment. The precursor
		// stays on entryPath. This walk is its own path from the root.
		ex.exitPath = pathKey
		model.mu.Unlock()
		return nil
	}

	model.mu.Unlock()

	action, _ := model.evaluateTree(pathKey)
	if action != "" {
		reported = action
	}

	if action == "exit" {
		if model.desk != nil && model.desk.Occupied(symbol) {
			model.finishVenue(symbol, pathKey)
		}

		// A matched exit clears the token sequence whether or not a lot
		// closed. Leaving the prefix in place makes the next tokens keep
		// matching exit until max_path_length slides it off.
		model.mu.Lock()
		model.resetPath(symbol)
		model.mu.Unlock()

		val := data.NewValue("training", "exit", symbol)
		val.Append(pathKey, "path")
		return val
	}

	if len(path) >= model.cfg.MinPathLength && action == "enter" {
		if model.desk != nil && !model.desk.Occupied(symbol) &&
			model.venueSlotsAvailable() {
			tick, err := model.requireTape()
			if err != nil {
				model.Error(err)
			} else if err := model.desk.Enter(symbol, pathKey, false, tick); err != nil {
				model.Error(err)
			} else {
				model.mu.Lock()
				model.resetPath(symbol)
				model.mu.Unlock()
				model.anchorVenue(symbol)
			}
		}

		val := data.NewValue("training", "enter", symbol)
		val.Append(pathKey, "path")
		return val
	}

	if side != "enter" || len(path) < model.cfg.MinPathLength {
		if side == "" {
			model.mu.Lock()
			if len(model.currentPath(symbol)) > 0 {
				model.resetPath(symbol)
			}
			model.mu.Unlock()
		}

		return nil
	}

	if model.desk != nil && model.desk.Occupied(symbol) {
		return nil
	}

	if !model.gateAllows(symbol, token) {
		return nil
	}

	model.mu.Lock()
	retired := model.retired(pathKey, "enter")
	model.mu.Unlock()

	if retired {
		return nil
	}

	if err := model.openExcursion(symbol, pathKey); err != nil {
		if !errnie.IsNotFound(err) {
			model.Error(err)
		}
	}

	return nil
}

func (model *Model) gateAllows(symbol string, token tokenizer.Token) bool {
	if model.cfg == nil {
		return true
	}

	if token.Brightness < model.cfg.MinTokenBrightness {
		return false
	}

	untyped, ok := model.tickReadings.Load(symbol)
	if !ok {
		return true
	}

	reading, ok := untyped.(tickReading)
	if !ok {
		return true
	}

	if reading.maturity < model.cfg.MinGridMaturity {
		return false
	}

	if model.cfg.RequireIgnitionRegime && !reading.ignitionPass {
		return false
	}

	return reading.frictionPass
}

/*
reviewExcursions closes an open ledger sample when its fee-net book return
has moved one friction hurdle from the open print. Up, that move has to
come out as a profit. Down, the same hurdle is the stop. A print inside
the band leaves the position open.
*/
func (model *Model) reviewExcursions() {
	if model.desk == nil {
		return
	}

	model.mu.Lock()
	symbols := make([]string, 0, len(model.excursions))
	pending := make([]*excursion, 0, len(model.excursions))

	for symbol, ex := range model.excursions {
		if ex == nil {
			continue
		}

		symbols = append(symbols, symbol)
		pending = append(pending, ex)
	}

	model.mu.Unlock()

	for index, symbol := range symbols {
		model.reviewExcursion(symbol, pending[index])
	}

	model.reviewVenues()
}

func (model *Model) reviewExcursion(symbol string, ex *excursion) {
	if ex == nil || model.desk == nil {
		return
	}

	hurdle, ok := model.desk.Friction(symbol)
	if !ok || hurdle <= 0 {
		model.Error(errnie.Err(
			errnie.NotFound,
			"[model] friction hurdle unavailable for "+symbol,
			nil,
		))
		return
	}

	ret, ok := model.desk.SaleReturn(symbol, ex.qty, ex.entry, ex.fee)
	if !ok {
		return
	}

	if !ex.anchored {
		ex.anchor = ret
		ex.anchored = true
		return
	}

	if ret-ex.anchor >= hurdle && ret > 0 {
		ex.cause = "cleared friction"
		model.finishExcursion(symbol, ex, ret)
		return
	}

	if ex.anchor-ret >= hurdle {
		ex.cause = "friction dip"
		model.finishExcursion(symbol, ex, ret)
	}
}

func (model *Model) anchorVenue(symbol string) {
	if model.desk == nil {
		return
	}

	position := model.desk.Open(symbol)
	if position == nil || position.Virtual || position.Qty == nil || position.EntryPrice == nil || position.EntryFee == nil {
		return
	}

	ret, ok := model.desk.OpenMark(symbol)
	if !ok {
		return
	}

	model.mu.Lock()
	model.venueAnchor[symbol] = ret
	model.mu.Unlock()
}

func (model *Model) reviewVenues() {
	if model.desk == nil {
		return
	}

	for _, position := range model.desk.Positions(true) {
		if position == nil || position.Virtual || position.Symbol == "" {
			continue
		}

		model.reviewVenue(position)
	}
}

func (model *Model) reviewVenue(position *broker.Position) {
	symbol := position.Symbol
	hurdle, ok := model.desk.Friction(symbol)
	if !ok || hurdle <= 0 {
		model.Error(errnie.Err(
			errnie.NotFound,
			"[model] friction hurdle unavailable for "+symbol,
			nil,
		))
		return
	}

	if position.Qty == nil || position.EntryPrice == nil || position.EntryFee == nil {
		return
	}

	ret, ok := model.desk.SaleReturn(symbol, position.Qty, position.EntryPrice, position.EntryFee)
	if !ok {
		return
	}

	model.mu.Lock()
	anchor, seen := model.venueAnchor[symbol]
	if !seen {
		model.venueAnchor[symbol] = ret
		model.mu.Unlock()
		return
	}
	model.mu.Unlock()

	if ret-anchor < hurdle && anchor-ret < hurdle {
		return
	}

	if ret-anchor >= hurdle && ret <= 0 {
		return
	}

	tick, err := model.requireTape()
	if err != nil {
		model.Error(err)
		return
	}

	if err := model.desk.ExitOccupied(symbol, position.EntryPath, tick); err != nil {
		model.Error(err)
		return
	}

	model.accountVenue(position.EntryPath, symbol, ret)

	model.mu.Lock()
	delete(model.venueAnchor, symbol)
	model.cool[symbol] = &coolDown{tokens: tokenSet(position.EntryPath)}
	model.resetPath(symbol)
	model.mu.Unlock()
}

func (model *Model) venueSlotsAvailable() bool {
	if model.desk == nil || model.cfg == nil {
		return true
	}

	limit := model.cfg.MaxVenuePositions
	if limit <= 0 {
		return true
	}

	open := 0

	for _, position := range model.desk.Positions(true) {
		if position != nil && !position.Virtual {
			open++
		}
	}

	return open < limit
}

/*
evaluateTree returns the action on the longest stored prefix of pathKey.
A shorter enter does not veto a longer exit. No stored prefix yields ("", 0).
*/
func (model *Model) evaluateTree(pathKey string) (string, int) {
	segments := strings.Split(pathKey, "/")

	var action string
	var matchDepth int

	for i := 1; i <= len(segments); i++ {
		prefix := strings.Join(segments[:i], "/")
		val, ok := model.tree.Get([]byte(prefix))
		if !ok || val == nil {
			continue
		}

		if val.Role != "enter" && val.Role != "exit" {
			continue
		}

		action = val.Role
		matchDepth = i
	}

	if action == "" {
		return "", 0
	}

	return action, matchDepth
}

/*
Settle scores a closed lot that the desk already priced. Return on the
position is percent. A path in the current policy is demoted from venue
losses. Any other path is a ledger sample. Idempotent per position ID.
*/
func (model *Model) Settle(position *broker.Position) {
	if position == nil || position.EntryPath == "" {
		return
	}

	if _, done := model.settled.LoadOrStore(position.ID, true); done {
		return
	}

	model.mu.Lock()
	delete(model.excursions, position.Symbol)
	model.mu.Unlock()

	if position.Return == nil {
		model.Error(errnie.Err(
			errnie.UnprocessableContent,
			"[model] closed position "+position.Symbol+" has no return",
			nil,
		))
		return
	}

	ret := position.Return.Div(decimal.NewFromFloat64(100)).Float64()

	if _, found := model.tree.Get([]byte(position.EntryPath)); found {
		model.accountVenue(position.EntryPath, position.Symbol, ret)
		model.mu.Lock()
		delete(model.venueAnchor, position.Symbol)
		model.mu.Unlock()
		return
	}

	model.mu.Lock()
	model.cool[position.Symbol] = &coolDown{tokens: tokenSet(position.EntryPath)}
	model.resetPath(position.Symbol)
	model.mu.Unlock()

	model.noteSample(position.EntryPath, position.Symbol, "enter", ret)

	if position.ExitPath != "" {
		model.noteSample(position.ExitPath, position.Symbol, "exit", ret)
	}
}

/*
Train inserts a value into the current policy tree and persists it.
The value must carry a "path" layer with the token path string.
*/
func (model *Model) Train(value *data.Value) {
	if value == nil {
		return
	}

	var path string
	value.Lookup(&path, "path")
	if path == "" {
		return
	}

	role := value.Role
	if role != "enter" && role != "exit" {
		model.Error(errnie.Err(
			errnie.Validation,
			"[model] policy "+path+" has no enter or exit role",
			nil,
		))
		return
	}

	model.writePolicy(value, path, role)
}

/*
promote writes a confirmed path into the current policy.
*/
func (model *Model) promote(pathKey, symbol, role string) {
	val := data.NewValue("training", role, symbol)
	val.Append(pathKey, "path")
	val.Append(0, "losses")
	model.Train(val)
}

func (model *Model) writePolicy(value *data.Value, pathKey, role string) {
	model.mu.Lock()
	model.tree, _, _ = model.tree.Insert([]byte(pathKey), value)
	model.mu.Unlock()
	model.PublishPolicy()

	if model.store == nil {
		return
	}

	storeKey := model.policyPrefix() + "/" + pathKey + "/" + role + ".bin"
	value.Prefix = storeKey

	if _, err := model.store.WriteAtPrefix(storeKey, value.Bytes()); err != nil {
		model.Error(err)
	}
}

func (model *Model) deletePolicy(pathKey, role string) {
	if model.store == nil {
		return
	}

	if err := model.store.DeletePrefix(model.policyPrefix() + "/" + pathKey + "/" + role + ".bin"); err != nil {
		model.Error(err)
	}
}

/*
noteSample records one fee-net return (a fraction) for a path and role.
The mean freezes after freezeAfter separate excursions. Confirmation
closes are held out of that mean. A non-positive mean retires the path.
*/
func (model *Model) noteSample(pathKey, symbol, role string, ret float64) {
	if pathKey == "" || (role != "enter" && role != "exit") {
		model.Error(errnie.Err(
			errnie.Validation,
			"[model] sample for "+symbol+" needs an enter or exit path",
			nil,
		))
		return
	}

	key := exploredKey(role, pathKey)

	model.mu.Lock()
	score := pathScore{}

	if val, found := model.explored.Get([]byte(key)); found && val != nil {
		val.Lookup(&score, "score")
	}

	if score.Retired {
		model.mu.Unlock()
		return
	}

	promoteNow := false

	if score.Frozen {
		if ret > 0 {
			score.Confirm++
			promoteNow = score.Confirm >= model.cfg.ConfirmAfter
		} else {
			score.Frozen = false
			score.Confirm = 0
			score.Sum += ret
			score.Count++
			model.freezeOrRetire(&score)
		}
	} else {
		score.Sum += ret
		score.Count++
		model.freezeOrRetire(&score)
	}

	if promoteNow {
		model.explored, _, _ = model.explored.Delete([]byte(key))
		model.mu.Unlock()
		model.promote(pathKey, symbol, role)
		model.deleteExploredObject(role, pathKey)
		return
	}

	if score.Retired {
		model.mu.Unlock()
		model.storeScore(key, pathKey, symbol, role, score)
		return
	}

	model.mu.Unlock()
	model.storeScore(key, pathKey, symbol, role, score)
}

func (model *Model) freezeOrRetire(score *pathScore) {
	if score == nil || model.cfg == nil || score.Count < model.cfg.FreezeAfter {
		return
	}

	prior := model.cfg.PriorSamples
	mean := (score.Sum + prior*model.cfg.MinMeanReturn) / (float64(score.Count) + prior)

	if mean > model.cfg.MinMeanReturn {
		score.Frozen = true
		score.Confirm = 0
		return
	}

	score.Retired = true
	score.Frozen = false
}

func (model *Model) storeScore(key, pathKey, symbol, role string, score pathScore) {
	val := data.NewValue("training", role, symbol)
	val.Append(pathKey, "path")
	val.Append(score, "score")

	model.mu.Lock()
	model.explored, _, _ = model.explored.Insert([]byte(key), val)
	model.mu.Unlock()
	model.PublishPolicy()

	if model.store == nil {
		return
	}

	storeKey := "policy/explored/" + role + "/" + pathKey + ".bin"
	val.Prefix = storeKey

	if _, err := model.store.WriteAtPrefix(storeKey, val.Bytes()); err != nil {
		model.Error(err)
	}
}

func (model *Model) deleteExploredObject(role, pathKey string) {
	if model.store == nil {
		return
	}

	if err := model.store.DeletePrefix("policy/explored/" + role + "/" + pathKey + ".bin"); err != nil {
		model.Error(err)
	}
}

func (model *Model) openExcursion(symbol, pathKey string) error {
	if model.desk == nil {
		return errnie.Err(errnie.Validation, "[model] desk unavailable", nil)
	}

	model.mu.Lock()
	blocked := model.excursions[symbol] != nil || model.retired(pathKey, "enter")
	model.mu.Unlock()

	if blocked {
		return nil
	}

	qty, entry, fee, err := model.desk.Quote(symbol)
	if err != nil {
		if errnie.IsNotFound(err) {
			return nil
		}
		return err
	}

	// The virtual lot is what the learning book and the longtail edge read.
	// Confirmation still has to pass before a venue order is sent.
	model.mu.Lock()
	markA := model.pathAnchorTick[symbol]
	model.mu.Unlock()

	tick, err := model.requireTape()
	if err != nil {
		return err
	}

	if err := model.desk.Enter(symbol, pathKey, true, tick); err != nil {
		if errnie.IsNotFound(err) {
			return nil
		}
		return err
	}

	virtual := model.desk.Virtual(symbol)
	if virtual == nil {
		return nil
	}

	if markA > 0 && markA <= virtual.EntryTick {
		virtual.MarkATick = markA
	}

	model.mu.Lock()
	model.excursions[symbol] = &excursion{
		entryPath: pathKey,
		qty:       qty,
		entry:     entry,
		fee:       fee,
	}
	if ret, ok := model.desk.OpenMark(symbol); ok {
		model.excursions[symbol].anchor = ret
		model.excursions[symbol].anchored = true
	}

	// B, the ignition token, is the last step of the precursor and the
	// first step of the holding path. The holding path then grows on its
	// own from the root.
	precursor := model.currentPath(symbol)
	var ignition tokenizer.Token
	if len(precursor) > 0 {
		ignition = precursor[len(precursor)-1]
	}

	model.resetPath(symbol)

	if len(ignition.Symbols) > 0 {
		held := model.appendPath(symbol, ignition)
		model.excursions[symbol].exitPath = held.String()
	}

	model.mu.Unlock()

	return nil
}

func (model *Model) finishExcursion(symbol string, ex *excursion, ret float64) {
	if ex == nil || model.desk == nil {
		return
	}

	virtual := model.desk.Virtual(symbol)
	exitPath := ex.exitPath

	if virtual != nil {
		trigger := ex.cause
		if trigger == "" {
			trigger = "book"
		}

		tick, err := model.requireTape()
		if err != nil {
			model.Error(err)
			return
		}

		if err := model.desk.Exit(symbol, exitPath, true, tick, trigger); err != nil {
			return
		}

		if virtual.ID != "" {
			model.settled.LoadOrStore(virtual.ID, true)
		}
	}

	model.mu.Lock()
	delete(model.excursions, symbol)
	model.cool[symbol] = &coolDown{tokens: tokenSet(ex.entryPath)}
	model.resetPath(symbol)
	model.mu.Unlock()

	model.noteSample(ex.entryPath, symbol, "enter", ret)

	if ex.exitPath != "" && ex.exitPath != ex.entryPath {
		model.noteSample(ex.exitPath, symbol, "exit", ret)
	}
}

func (model *Model) finishVenue(symbol, pathKey string) {
	if model.desk == nil {
		return
	}

	position := model.desk.Open(symbol)
	if position == nil {
		model.Error(errnie.Err(
			errnie.NotFound, "[model] no open position for "+symbol, nil,
		))
		return
	}

	if position.Qty == nil || position.EntryPrice == nil || position.EntryFee == nil {
		model.Error(errnie.Err(
			errnie.UnprocessableContent,
			"[model] open position "+symbol+" has no priced entry",
			nil,
		))
		return
	}

	ret, ok := model.desk.SaleReturn(symbol, position.Qty, position.EntryPrice, position.EntryFee)
	if !ok {
		return
	}

	tick, err := model.requireTape()
	if err != nil {
		model.Error(err)
		return
	}

	if err := model.desk.ExitOccupied(symbol, pathKey, tick); err != nil {
		model.Error(err)
		return
	}

	model.accountVenue(position.EntryPath, position.Symbol, ret)

	model.mu.Lock()
	delete(model.venueAnchor, symbol)
	model.cool[symbol] = &coolDown{tokens: tokenSet(position.EntryPath)}
	model.resetPath(symbol)
	model.mu.Unlock()
}

func (model *Model) accountVenue(pathKey, symbol string, ret float64) {
	if pathKey == "" {
		return
	}

	model.mu.Lock()
	val, found := model.tree.Get([]byte(pathKey))

	if !found || val == nil {
		model.mu.Unlock()
		return
	}

	role := val.Role

	if ret > 0 {
		updated := data.NewValue("training", role, symbol)
		updated.Append(pathKey, "path")
		updated.Append(0, "losses")
		model.tree, _, _ = model.tree.Insert([]byte(pathKey), updated)
		model.mu.Unlock()
		model.PublishPolicy()
		model.writePolicy(updated, pathKey, role)
		return
	}

	var losses int
	val.Lookup(&losses, "losses")
	losses++

	if losses >= model.cfg.DemotionLosses {
		model.tree, _, _ = model.tree.Delete([]byte(pathKey))
		model.mu.Unlock()
		model.PublishPolicy()
		model.deletePolicy(pathKey, role)
		model.storeScore(exploredKey(role, pathKey), pathKey, symbol, role, pathScore{
			Retired: true,
			Count:   losses,
			Sum:     ret,
		})
		return
	}

	updated := data.NewValue("training", role, symbol)
	updated.Append(pathKey, "path")
	updated.Append(losses, "losses")
	model.tree, _, _ = model.tree.Insert([]byte(pathKey), updated)
	model.mu.Unlock()
	model.PublishPolicy()
	model.writePolicy(updated, pathKey, role)
}

// retired reports whether the path's role was retired. Caller holds model.mu.
func (model *Model) retired(pathKey, role string) bool {
	val, found := model.explored.Get([]byte(exploredKey(role, pathKey)))
	if !found || val == nil {
		return false
	}

	var score pathScore
	val.Lookup(&score, "score")

	return score.Retired
}

func (model *Model) grammarKind(symbol string) string {
	flag, ok := model.ingress.Load(symbol)
	if !ok {
		return "append"
	}

	kind, _ := flag.(string)
	if kind != "create" && kind != "reuse" {
		return "append"
	}

	return kind
}

func exploredKey(role, pathKey string) string {
	return role + "\x00" + pathKey
}

func exploredPublishKey(stored string) (string, bool) {
	role, path, ok := strings.Cut(stored, "\x00")
	if !ok || path == "" || (role != "enter" && role != "exit") {
		return "", false
	}

	return "policy/explored/" + role + "/" + path + ".bin", true
}

func tokenSet(pathKey string) map[string]struct{} {
	set := make(map[string]struct{})

	for _, part := range strings.Split(pathKey, "/") {
		if part == "" {
			continue
		}

		set[part] = struct{}{}
	}

	return set
}

func tokenSide(token tokenizer.Token) string {
	enter, exit := 0, 0

	for _, sym := range token.Symbols {
		key, ok := regionIndex(sym)
		if !ok {
			continue
		}

		switch nmstore.RegionSide(uint8(key)) {
		case "enter":
			enter++
		case "exit":
			exit++
		}
	}

	if exit > enter && exit > 0 {
		return "exit"
	}

	if enter > exit && enter > 0 {
		return "enter"
	}

	return ""
}

func regionIndex(symbol string) (int, bool) {
	if len(symbol) != 3 || symbol[0] != 'R' {
		return 0, false
	}

	key, err := strconv.Atoi(symbol[1:])
	if err != nil || key < 0 || key > 255 {
		return 0, false
	}

	return key, true
}

// appendPath adds a token to the symbol's accumulated path, trimming
// to maxPathLength, and stores it. Consecutive identical tokens are
// skipped so the trie learns transitions, not dwell length. Caller
// must hold model.mu.
func (model *Model) appendPath(symbol string, token tokenizer.Token) tokenizer.Tokens {
	label := token.String()
	if label == "" {
		untyped, ok := model.paths.Load(symbol)
		if ok {
			return untyped.(tokenizer.Tokens)
		}
		return nil
	}

	untyped, ok := model.paths.Load(symbol)
	var path tokenizer.Tokens
	if ok {
		path = untyped.(tokenizer.Tokens)
	}

	if len(path) > 0 && path[len(path)-1].String() == label {
		return path
	}

	path = append(path, token)

	if model.cfg != nil && len(path) > model.cfg.MaxPathLength {
		path = path[len(path)-model.cfg.MaxPathLength:]
	}

	if len(path) == 1 {
		if tick := model.tape.Load(); tick > 0 {
			if _, ok := model.pathAnchorTick[symbol]; !ok {
				model.pathAnchorTick[symbol] = tick
			}
		}
	}

	model.paths.Store(symbol, path)
	return path
}

// resetPath clears the symbol's accumulated path. Caller must hold model.mu.
func (model *Model) resetPath(symbol string) {
	model.paths.Store(symbol, tokenizer.Tokens{})
	delete(model.pathAnchorTick, symbol)
}

// currentPath is the symbol's accumulated regime path. Caller holds model.mu.
func (model *Model) currentPath(symbol string) tokenizer.Tokens {
	untyped, ok := model.paths.Load(symbol)
	if !ok {
		return nil
	}

	return untyped.(tokenizer.Tokens)
}

/*
PublishPolicy sends the policy trie to the dashboard. Confirmed paths are
policy/current/<token path>/<action>.bin. Explored paths, including ones
that have not been confirmed, are policy/explored/<action>/<token path>.bin.
The radix view lays both out as one trie.
*/
func hubPublishes(hub io.Writer) bool {
	if hub == nil {
		return false
	}
	type active interface {
		Active() bool
	}
	if h, ok := hub.(active); ok {
		return h.Active()
	}
	return true
}

func (model *Model) countExploredLocked(role string) int {
	if model.explored == nil {
		return 0
	}

	count := 0
	prefix := role + "\x00"

	iterator := model.explored.Root().Iterator()
	for key, _, ok := iterator.Next(); ok; key, _, ok = iterator.Next() {
		if strings.HasPrefix(string(key), prefix) {
			count++
		}
	}

	return count
}

func (model *Model) countPolicyLocked(role string) int {
	if model.tree == nil {
		return 0
	}

	count := 0
	iterator := model.tree.Root().Iterator()

	for _, val, ok := iterator.Next(); ok; _, val, ok = iterator.Next() {
		if val != nil && val.Role == role {
			count++
		}
	}

	return count
}

func (model *Model) PublishPolicy() {
	if !hubPublishes(model.hub) {
		return
	}

	model.mu.Lock()
	tree := model.tree
	explored := model.explored
	model.mu.Unlock()

	keys := make([]string, 0)

	if tree != nil {
		iterator := tree.Root().Iterator()
		for key, val, ok := iterator.Next(); ok; key, val, ok = iterator.Next() {
			path := string(key)
			if path == "" {
				continue
			}

			prefix := model.policyPrefix()

			if strings.Contains(path, ".bin") {
				if !strings.HasPrefix(path, prefix+"/") {
					path = prefix + "/" + path
				}
				keys = append(keys, path)
				continue
			}

			role := "enter"
			if val != nil && (val.Role == "enter" || val.Role == "exit") {
				role = val.Role
			}
			keys = append(keys, prefix+"/"+path+"/"+role+".bin")
		}
	}

	if explored != nil {
		iterator := explored.Root().Iterator()
		for key, val, ok := iterator.Next(); ok; key, val, ok = iterator.Next() {
			if val != nil {
				var score pathScore
				val.Lookup(&score, "score")
				if score.Retired {
					continue
				}
			}

			published, valid := exploredPublishKey(string(key))
			if !valid {
				continue
			}
			keys = append(keys, published)
		}
	}

	value := data.NewValue("cognition", "keys", "policy")
	value.Append(map[string]any{
		"epoch": "0",
		"keys":  keys,
	}, "cognition")
	io.Copy(model.hub, value)
}

/*
publishGrammar sends one sequitur frame for the symbol: the grammar after
this token, whether the append created or reused a rule, and how the path
met the current policy. The longtail surface reads these frames.
*/
func (model *Model) publishGrammar(
	symbol string,
	sequencer *tokenizer.Sequencer,
	token tokenizer.Token,
	pathKey string,
	kind string,
	action string,
) {
	if !hubPublishes(model.hub) || sequencer == nil || sequencer.Grammar() == nil {
		return
	}

	if kind != "create" && kind != "reuse" {
		kind = "append"
	}

	grammar := sequencer.Grammar()
	confirmed, explored := model.storedPaths()
	views := grammar.RuleViews()
	rules := make([]map[string]any, 0, len(views))
	for _, view := range views {
		symbols := view.Symbols
		if symbols == nil {
			symbols = []string{}
		}
		terminals := view.Terminals
		if terminals == nil {
			terminals = []string{}
		}
		row := map[string]any{
			"id":        view.ID,
			"name":      tokenizer.RuleRef(view.ID),
			"count":     view.Count,
			"depth":     view.Depth,
			"length":    len(symbols),
			"symbols":   symbols,
			"terminals": terminals,
		}
		if ruleAction := actionFor(terminals, confirmed, explored); ruleAction != "" {
			row["action"] = ruleAction
		}
		rules = append(rules, row)
	}

	start := grammar.RootSequence()
	if start == nil {
		start = []string{}
	}

	fired := token.Symbols
	if fired == nil {
		fired = []string{}
	}

	active := make([]map[string]any, 0, len(fired))
	for _, sym := range fired {
		if len(sym) != 3 || sym[0] != 'R' {
			continue
		}
		key, err := strconv.Atoi(sym[1:])
		if err != nil {
			continue
		}
		active = append(active, map[string]any{
			"key":        key,
			"brightness": token.Brightness,
		})
	}

	treeAction, depth := model.evaluateTree(pathKey)
	stored := treeAction != ""
	match := ""
	if stored {
		match = pathKey
	}
	outcome := ""
	if action == "enter" || action == "exit" {
		outcome = action
	}

	seqPath := model.sequiturPathKey(symbol)
	pathDrift, pathCompared := model.regimeDrift(symbol, token)
	gatePassed := model.gateAllows(symbol, token)
	frictionKnown := false
	frictionPassed := false
	gateReading := map[string]any{
		"surprise": nil,
		"z":        nil,
		"warm":     false,
		"passed":   gatePassed,
		"cold":     "no reading",
	}

	if untyped, ok := model.tickReadings.Load(symbol); ok {
		if reading, ok := untyped.(tickReading); ok {
			if model.cfg != nil && model.cfg.RequireFrictionGate {
				frictionKnown = true
				frictionPassed = reading.frictionPass
			}
			warm := model.cfg != nil && reading.maturity >= model.cfg.MinGridMaturity
			gateReading["warm"] = warm
			if warm {
				delete(gateReading, "cold")
			} else {
				gateReading["cold"] = "history"
			}
			if reading.hasSurprise {
				gateReading["surprise"] = reading.surprise
			}
			if reading.hasZ {
				gateReading["z"] = reading.z
			}
		}
	}

	frame := map[string]any{
		"symbol":    symbol,
		"epoch":     "0",
		"tick":      int(model.tape.Load()),
		"timestamp": strconv.FormatUint(model.tape.Load(), 10),
		"active":    active,
		"midpoint":  token.Brightness,
		"fired":     fired,
		"held":      len(fired),
		"gate":      gateReading,
		"blocked":   !gatePassed,
		"measurement": map[string]any{
			"canonical":       true,
			"sequiturPath":    seqPath,
			"pathDrift":       pathDrift,
			"pathCompared":    pathCompared,
			"frictionPass":    frictionPassed,
			"frictionChecked": frictionKnown,
		},
		"events": []map[string]any{{
			"kind":   kind,
			"token":  token.String(),
			"rule":   0,
			"into":   0,
			"from":   0,
			"digram": []string{"", ""},
		}},
		"completed": "",
		"path":      pathKey,
		"policy": map[string]any{
			"epoch":   "0",
			"paths":   0,
			"trading": model.desk != nil,
			"stored":  stored,
			"match":   match,
			"depth":   depth,
			"action":  action,
			"outcome": outcome,
		},
		"rules":          rules,
		"start":          start,
		"rawLength":      grammar.RootLength(),
		"grammarSymbols": grammar.Symbols(),
		"violations":     []string{},
	}

	value := data.NewValue("sequitur", "grammar", symbol)
	value.Append(frame, "grammar")
	io.Copy(model.hub, value)

	model.recordFrameMetrics(symbol, gatePassed, frictionKnown, frictionPassed, pathDrift, pathCompared, stored)
}

func (model *Model) regimeDrift(symbol string, chord tokenizer.Token) (bool, bool) {
	untyped, ok := model.lastRegime.Load(symbol)
	if !ok || len(chord.Symbols) == 0 {
		return false, false
	}

	note, ok := untyped.(regimeNote)
	if !ok || note.tick != model.tape.Load() || len(note.symbols) == 0 {
		return false, false
	}

	have := make(map[string]struct{}, len(chord.Symbols))
	for _, sym := range chord.Symbols {
		have[sym] = struct{}{}
	}

	for _, sym := range note.symbols {
		if _, ok := have[sym]; !ok {
			return true, true
		}
	}

	return false, true
}

type storedPath struct {
	parts []string
	role  string
}

func (model *Model) storedPaths() (confirmed, explored []storedPath) {
	if model == nil {
		return nil, nil
	}

	model.mu.Lock()
	tree := model.tree
	exploredTree := model.explored
	model.mu.Unlock()

	if tree != nil {
		iterator := tree.Root().Iterator()
		for key, val, ok := iterator.Next(); ok; key, val, ok = iterator.Next() {
			if val == nil || (val.Role != "enter" && val.Role != "exit") {
				continue
			}
			confirmed = append(confirmed, storedPath{
				parts: splitPath(string(key)),
				role:  val.Role,
			})
		}
	}

	if exploredTree != nil {
		iterator := exploredTree.Root().Iterator()
		for key, val, ok := iterator.Next(); ok; key, val, ok = iterator.Next() {
			role, path, cut := strings.Cut(string(key), "\x00")
			if !cut || path == "" || (role != "enter" && role != "exit") {
				continue
			}
			if val != nil {
				var score pathScore
				val.Lookup(&score, "score")
				if score.Retired {
					continue
				}
			}
			explored = append(explored, storedPath{
				parts: splitPath(path),
				role:  role,
			})
		}
	}

	return confirmed, explored
}

func splitPath(path string) []string {
	var parts []string

	for _, segment := range strings.Split(path, "/") {
		if segment == "" {
			continue
		}
		if strings.HasPrefix(segment, "[") && strings.HasSuffix(segment, "]") && len(segment) > 2 {
			parts = append(parts, strings.Split(segment[1:len(segment)-1], "-")...)
			continue
		}
		parts = append(parts, segment)
	}

	return parts
}

func actionFor(terminals []string, confirmed, explored []storedPath) string {
	for _, path := range confirmed {
		if covers(path.parts, terminals) {
			return path.role
		}
	}

	for _, path := range explored {
		if covers(path.parts, terminals) {
			return path.role
		}
	}

	return ""
}

func covers(path, rule []string) bool {
	if len(rule) == 0 || len(rule) > len(path) {
		return false
	}

	for i := 0; i+len(rule) <= len(path); i++ {
		matched := true
		for j := range rule {
			if path[i+j] != rule[j] {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}

	return false
}

func (model *Model) StampStock() {
	if model == nil || model.metrics == nil {
		return
	}

	model.mu.Lock()
	explored := model.countExploredLocked("enter")
	confirmed := model.countPolicyLocked("enter")
	model.mu.Unlock()

	model.metrics.setStock(explored, confirmed)
}

func (model *Model) recordFrameMetrics(
	symbol string,
	gatePassed, frictionKnown, frictionPassed, pathDrift, pathCompared, policyMatch bool,
) {
	if model.metrics == nil {
		return
	}

	delta := TickMetrics{Symbols: 1, CanonicalTicks: 1, GateTotal: 1, PolicyChecks: 1}

	if gatePassed {
		delta.GatePass++
	}

	if frictionKnown {
		delta.FrictionTotal = 1
		if frictionPassed {
			delta.FrictionPass++
		}
	}

	if pathCompared {
		delta.PathCompared = 1
		if pathDrift {
			delta.PathDrift++
		}
	}

	if policyMatch {
		delta.PolicyMatches++
	}

	if untyped, ok := model.tickReadings.Load(symbol); ok {
		if reading, ok := untyped.(tickReading); ok {
			delta.MeanMaturity = reading.maturity
			if reading.hasSurprise {
				delta.MeanSurprise = reading.surprise
				delta.SurpriseSamples = 1
			}
		}
	}

	model.mu.Lock()
	if model.excursions[symbol] != nil {
		delta.VirtualExcursion++
	}

	if model.desk != nil {
		if position := model.desk.Open(symbol); position != nil && !position.Virtual {
			delta.VenueOpen++
		}
	}
	model.mu.Unlock()

	model.metrics.add(delta)
}

/*
publishMatch sends the symbol's current token path to the dashboard as a
cognition/match frame so the position card's token ribbon and strength
gauge render live. The shape matches the frontend's CognitionMatch type.
*/
func (model *Model) publishMatch(symbol string, path tokenizer.Tokens, action string) {
	if !hubPublishes(model.hub) {
		return
	}

	match := map[string]any{
		"symbol": symbol,
		"tick":   data.Tick.Load(),
		"tokens": path.StringList(),
		"stored": false,
		"match":  path.String(),
		"depth":  len(path),
		"action": action,
	}

	value := data.NewValue("cognition", "match", symbol)
	value.Append(match, "cognition")

	io.Copy(model.hub, value)
}
