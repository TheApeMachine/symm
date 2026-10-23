package strategy

import (
	"context"
	"io"
	"strconv"
	"strings"
	"sync"

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
	Enter(symbol string, path string, virtual bool) error
	Exit(symbol string, path string, virtual bool) error
	Virtual(symbol string) *broker.Position
	Positions(open bool) []*broker.Position
	Profit(symbol string, virtual bool) (bool, bool)
	ClosedVirtuals() []*broker.Position
	Drop(symbol string)
	Quote(symbol string) (qty, entry, fee *decimal.Decimal, err error)
	Realize(symbol string, qty, entry, fee *decimal.Decimal) (float64, bool)
	Occupied(symbol string) bool
	Open(symbol string) *broker.Position
	ExitOccupied(symbol, path string) error
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
	closing   bool
}

// coolDown blocks a second sample of the sentence that just closed until
// a regime token arrives that was not part of that entry path.
type coolDown struct {
	tokens map[string]struct{}
}

type tickReading struct {
	maturity     float64
	surprise     float64
	frictionPass bool
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
	excursions      map[string]*excursion
	cool            map[string]*coolDown
}

func NewModel(
	ctx context.Context, desk Desk, client *store.Client,
	hub io.Writer, cfg *Config,
) *Model {
	if cfg == nil {
		cfg = LoadConfig()
	}

	model := &Model{
		System:       runtime.NewSystem(ctx, "model"),
		cfg:          cfg,
		desk:         desk,
		store:        client,
		hub:          hub,
		tree:         iradix.New[*data.Value](),
		explored:     iradix.New[*data.Value](),
		sequencers:   &sync.Map{},
		paths:        &sync.Map{},
		seqPaths:     &sync.Map{},
		settled:      &sync.Map{},
		ingress:      &sync.Map{},
		tickReadings: &sync.Map{},
		excursions:   map[string]*excursion{},
		cool:         map[string]*coolDown{},
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

func (model *Model) NoteTick(
	symbol string,
	regions []*nmstore.Region,
	grid *nmstore.Grid,
) {
	if symbol == "" || grid == nil {
		return
	}

	reading := tickReading{frictionPass: true}

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

	var surpriseSum float64
	lit := 0

	for _, region := range regions {
		if region == nil {
			continue
		}

		lit++
		surpriseSum += region.Brightness
	}

	if lit > 0 {
		reading.surprise = 1 - surpriseSum/float64(lit)
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

func (model *Model) TrackSequitur(symbol string, token tokenizer.Token) {
	if len(token.Symbols) == 0 {
		return
	}

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
	before := sequencer.Grammar().RuleCount()
	token := sequencer.Analyze(regions)
	model.ingress.Store(symbol, sequencer.Grammar().RuleCount() > before)

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
		path.String(), model.ingested(symbol), "noop",
	)
}

/*
Evaluate processes one regime token for one symbol and returns the
model's action, or nil when no action is taken.

The token is one trie edge. In order:
 1. A token from the sentence just sampled is ignored until a different
    regime shows the sentence has ended. That separator is not a path step.
 2. An open ledger excursion closes when an exit-side regime arrives and
    the bid book can take the size.
 3. The longest stored prefix of the path decides. A confirmed enter may
    open a venue order. A confirmed exit closes the open lot.
 4. Otherwise an enter-side path of at least minPathLength opens a ledger
    excursion, which does not take the symbol's position slot.
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
			symbol, sequencer, token, pathKey, model.ingested(symbol), reported,
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

	if ex != nil && (ex.closing || side == "exit") {
		if side == "exit" {
			ex.exitPath = pathKey
			ex.closing = true
		}

		model.mu.Unlock()
		model.finishExcursion(symbol, ex)
		return nil
	}

	if ex != nil {
		model.mu.Unlock()
		return nil
	}

	model.mu.Unlock()

	if len(path) >= model.cfg.MinPathLength {
		action, _ := model.evaluateTree(pathKey)
		if action != "" {
			reported = action
		}

		switch action {
		case "enter":
			if model.desk != nil && !model.desk.Occupied(symbol) &&
				model.venueSlotsAvailable() {
				if err := model.desk.Enter(symbol, pathKey, false); err != nil {
					model.Error(err)
				}
			}

			val := data.NewValue("training", "enter", symbol)
			val.Append(pathKey, "path")
			return val
		case "exit":
			if model.desk != nil && model.desk.Occupied(symbol) {
				model.finishVenue(symbol, pathKey)
			}

			val := data.NewValue("training", "exit", symbol)
			val.Append(pathKey, "path")
			return val
		}
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
		model.Error(err)
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

	return reading.frictionPass
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

	if position.Return == nil {
		model.Error(errnie.Err(
			errnie.UnprocessableContent,
			"[model] closed position "+position.Symbol+" has no return",
			nil,
		))
		return
	}

	ret := *position.Return / 100

	if _, found := model.tree.Get([]byte(position.EntryPath)); found {
		model.accountVenue(position.EntryPath, position.Symbol, ret)
		return
	}

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
		return err
	}

	// The virtual lot is what the learning book and the longtail edge read.
	// Confirmation still has to pass before a venue order is sent.
	if err := model.desk.Enter(symbol, pathKey, true); err != nil {
		return err
	}

	if model.desk.Virtual(symbol) == nil {
		return errnie.Err(
			errnie.UnprocessableContent,
			"[model] virtual was not opened for "+symbol,
			nil,
		)
	}

	model.mu.Lock()
	model.excursions[symbol] = &excursion{
		entryPath: pathKey,
		qty:       qty,
		entry:     entry,
		fee:       fee,
	}
	model.mu.Unlock()

	return nil
}

func (model *Model) finishExcursion(symbol string, ex *excursion) {
	if ex == nil || model.desk == nil {
		return
	}

	ret, ok := model.desk.Realize(symbol, ex.qty, ex.entry, ex.fee)
	if !ok {
		model.Error(errnie.Err(
			errnie.UnprocessableContent,
			"[model] book cannot take "+symbol,
			nil,
		))
		return
	}

	virtual := model.desk.Virtual(symbol)
	exitPath := ex.exitPath
	if exitPath == "" {
		exitPath = ex.entryPath
	}

	if virtual != nil {
		if err := model.desk.Exit(symbol, exitPath, true); err != nil {
			model.Error(err)
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

	ret, ok := model.desk.Realize(symbol, position.Qty, position.EntryPrice, position.EntryFee)
	if !ok {
		model.Error(errnie.Err(
			errnie.UnprocessableContent,
			"[model] book cannot take "+symbol,
			nil,
		))
		return
	}

	if err := model.desk.ExitOccupied(symbol, pathKey); err != nil {
		model.Error(err)
		return
	}

	model.accountVenue(position.EntryPath, position.Symbol, ret)

	model.mu.Lock()
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

func (model *Model) ingested(symbol string) bool {
	flag, ok := model.ingress.Load(symbol)
	if !ok {
		return false
	}

	created, _ := flag.(bool)

	return created
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
// to maxPathLength, and stores it. Caller must hold model.mu.
func (model *Model) appendPath(symbol string, token tokenizer.Token) tokenizer.Tokens {
	untyped, ok := model.paths.Load(symbol)
	var path tokenizer.Tokens
	if ok {
		path = untyped.(tokenizer.Tokens)
	}
	path = append(path, token)

	if model.cfg != nil && len(path) > model.cfg.MaxPathLength {
		path = path[len(path)-model.cfg.MaxPathLength:]
	}

	model.paths.Store(symbol, path)
	return path
}

// resetPath clears the symbol's accumulated path. Caller must hold model.mu.
func (model *Model) resetPath(symbol string) {
	model.paths.Store(symbol, tokenizer.Tokens{})
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
		for key, _, ok := iterator.Next(); ok; key, _, ok = iterator.Next() {
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
	created bool,
	action string,
) {
	if !hubPublishes(model.hub) || sequencer == nil || sequencer.Grammar() == nil {
		return
	}

	grammar := sequencer.Grammar()
	kind := "append"
	if created {
		kind = "create"
	} else if grammar.RuleCount() > 1 {
		kind = "reuse"
	}

	productions := grammar.Rules()
	rules := make([]map[string]any, 0, len(productions))
	for i, symbols := range productions {
		if symbols == nil {
			symbols = []string{}
		}
		terminals := make([]string, 0, len(symbols))
		depth := 1
		for _, sym := range symbols {
			if len(sym) > 1 && sym[0] == 'M' {
				depth = 2
				continue
			}
			terminals = append(terminals, sym)
		}
		rules = append(rules, map[string]any{
			"id":        i + 1,
			"name":      tokenizer.RuleRef(i + 1),
			"count":     1,
			"depth":     depth,
			"length":    len(symbols),
			"symbols":   symbols,
			"terminals": terminals,
		})
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
	pathDrift := seqPath != "" && pathKey != "" && seqPath != pathKey
	gatePassed := model.gateAllows(symbol, token)
	var gateReading map[string]any

	if untyped, ok := model.tickReadings.Load(symbol); ok {
		if reading, ok := untyped.(tickReading); ok {
			gateReading = map[string]any{
				"surprise": reading.surprise,
				"z":        nil,
				"warm":     reading.maturity >= model.cfg.MinGridMaturity,
				"passed":   gatePassed,
			}

			if reading.maturity < model.cfg.MinGridMaturity {
				gateReading["cold"] = "history"
			}
		}
	}

	if gateReading == nil {
		gateReading = map[string]any{
			"surprise": 1 - token.Brightness,
			"z":        nil,
			"warm":     true,
			"passed":   gatePassed,
		}
	}

	frame := map[string]any{
		"symbol":    symbol,
		"epoch":     "0",
		"tick":      int(data.Tick.Load()),
		"timestamp": strconv.FormatUint(data.Tick.Load(), 10),
		"active":    active,
		"midpoint":  token.Brightness,
		"fired":     fired,
		"held":      len(fired),
		"gate":      gateReading,
		"blocked":   !gatePassed,
		"measurement": map[string]any{
			"canonical":    true,
			"sequiturPath": seqPath,
			"pathDrift":    pathDrift,
			"frictionPass": gatePassed,
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

	model.recordFrameMetrics(symbol, gatePassed, pathDrift, stored)
}

func (model *Model) recordFrameMetrics(
	symbol string,
	gatePassed, pathDrift, policyMatch bool,
) {
	if model.metrics == nil {
		return
	}

	delta := TickMetrics{Symbols: 1, CanonicalTicks: 1, GateTotal: 1, FrictionTotal: 1, PathCompared: 1, PolicyChecks: 1}

	if gatePassed {
		delta.GatePass++
		delta.FrictionPass++
	}

	if pathDrift {
		delta.PathDrift++
	}

	if policyMatch {
		delta.PolicyMatches++
	}

	if untyped, ok := model.tickReadings.Load(symbol); ok {
		if reading, ok := untyped.(tickReading); ok {
			delta.MeanMaturity = reading.maturity
			delta.MeanSurprise = reading.surprise
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

	delta.ExploredEnter = model.countExploredLocked("enter")
	delta.ConfirmedEnter = model.countPolicyLocked("enter")
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
