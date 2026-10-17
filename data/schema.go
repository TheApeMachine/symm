package data

import (
	"encoding/binary"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/theapemachine/errnie"
)

/*
The storage schema: every key and layer the commands exchange, in one place.

	| writer  | key (Prefix + %019d tick .bin)                 | reader              |
	|---------|------------------------------------------------|---------------------|
	| collect | workspace/tick/aggregate/<epoch>/<tick>.bin    | detect, train, audit |
	| detect  | detector/<class>/<symbol>/<epoch>/<c_tick>.bin | train, audit        |
	| detect  | detector/watermark/all/<epoch>/<last tick>.bin | train               |
	| train   | policy/<action>/all/<policy epoch>/<seq>.bin   | run (CognitionTree), audit, hindsight |
	| train   | resonance/state/<symbol>/<policy epoch>/<last tick>.bin | train (next), run (CognitionTree) |
	| paper   | paper/<action>/<symbol>/<epoch>/<seq>.bin      | audit (realization) |

run (`make run`) never writes storage: it reads policies through a
store.Reader and appends its paper records to the local file
paper.records_path; `symm paper` uploads that file under paper/.

Layers of a stored tick (one Kraken trade frame; collect, cmd/booter.go):

	| index                                    | shape                                   |
	|------------------------------------------|-----------------------------------------|
	| spot:trade/<type>                        | the Kraken trade frame JSON, exactly once |
	| book/<symbol> or book_error/<symbol>     | spotbook.Book JSON, or why it is not valid |
	| price/<symbol>                           | broker.PriceLayer JSON                  |
	| instrument/<symbol>                      | Kraken instrument JSON                  |
	| balance, positions                       | the desk's wallet and positions JSON    |
	| signal/<source>/<label...>/<tag...>/symbol:<symbol> | float64 vector, one per label |
	| "" (empty)                               | a reserved slot no signal claimed       |

An excursion Value carries one float64 vector labelled ExcursionKeys; a
policy Value one labelled PolicyKeys plus a trailing PolicyPathTag tag, and
its gate layer (PolicyGate).

Hub-only Values (streamed to the dashboard, never stored):

	| writer             | Origin/Role/Scope           | layer                          |
	|--------------------|-----------------------------|--------------------------------|
	| run (strategy.Sequitur) | resonance/reading/<symbol> | each symbol's chord-gate coder reading (logic/resonance.Solver.Step), every tick |
	| run (strategy.Sequitur) | sequitur/grammar/<symbol> | LayerGrammar: strategy.SequiturFrame JSON |
	| run (strategy.Training) | paper/<action>/<symbol>   | PaperKeys vector + tags (also appended to paper.records_path) |
	| run (hindsight.Service) | hindsight/<role>/<symbol> | see HindsightOrigin: one client's replay of a stored run |
	| run (strategy.PublishTree) | cognition/tree/all     | LayerCognition: strategy.CognitionFrame JSON, the hub's retained frame |
	| run (strategy.Sequitur) | cognition/match/<symbol>  | LayerCognition: strategy.CognitionMatchFrame JSON, per appended chord |
	| run (strategy.Training) | position/<event>/<symbol> | LayerPosition: strategy.PositionFrame JSON, per paper round-trip event and per tick mark of an open position |
*/

/*
CognitionOrigin identifies the CognitionTree make run loaded (Role
CognitionTreeRole, Scope PolicyScope; sent once and retained by the hub for
every client that connects later) and each symbol's live match against it
(Role CognitionMatchRole, Scope the symbol). Both carry one LayerCognition
JSON layer.
*/
const (
	CognitionOrigin    = "cognition"
	CognitionTreeRole  = "tree"
	CognitionMatchRole = "match"
	LayerCognition     = "cognition"
)

/*
PositionOrigin identifies paper trading's live round trips on the hub: Role
the event (or "mark"), Scope the symbol, Epoch the policy epoch traded, one
LayerPosition JSON layer.
*/
const (
	PositionOrigin = "position"
	LayerPosition  = "position"
)

/*
SequiturOrigin and SequiturRole identify a live grammar frame on the hub;
its Scope is the symbol and its one layer is LayerGrammar.
*/
const (
	SequiturOrigin = "sequitur"
	SequiturRole   = "grammar"
	LayerGrammar   = "grammar"
)

const (
	TickOrigin = "workspace"
	TickRole   = "tick"
	TickScope  = "aggregate"
)

/*
TickPrefix is Prefix for stored ticks, narrowed to one epoch when given.
*/
func TickPrefix(epoch ...uint64) string {
	return Prefix(TickOrigin, TickRole, TickScope, epoch...)
}

/*
Tick layer heads.
*/
const (
	LayerTrade      = "spot:trade"
	LayerBook       = "book"
	LayerBookError  = "book_error"
	LayerPrice      = "price"
	LayerInstrument = "instrument"
	LayerBalance    = "balance"
	LayerPositions  = "positions"
)

/*
TradeIndex is the index of the tick's trade frame of Kraken message type.
*/
func TradeIndex(messageType string) string {
	return LayerTrade + "/" + messageType
}

/*
SymbolIndex is the index of a per-symbol tick layer (LayerBook,
LayerBookError, LayerPrice, LayerInstrument).
*/
func SymbolIndex(layer, symbol string) string {
	return layer + "/" + symbol
}

/*
CheckTickLayers fails a tick carrying a layer outside the schema, such as a
signal layer in a retired index format: a reader skipping it would read the
tick as having no signals.
*/
func CheckTickLayers(value *Value) error {
	heads := []string{LayerTrade + "/", LayerBook + "/", LayerBookError + "/", LayerPrice + "/", LayerInstrument + "/"}

	for idx, index := range value.Index {
		if idx >= len(value.Layers) {
			break
		}

		switch {
		case index == "" && len(value.Layers[idx]) == 0:
		case index == LayerBalance, index == LayerPositions, IsSignalIndex(index):
		case slices.ContainsFunc(heads, func(head string) bool { return strings.HasPrefix(index, head) }):
		default:
			return errnie.Err(errnie.UnprocessableContent, fmt.Sprintf(
				"data: %s carries layer %d with index %q outside the tick schema (a retired format? collect a fresh epoch)",
				value.Prefix, idx, index,
			), nil)
		}
	}

	return nil
}

/*
Signal layer index:

	signal/<source>/<label...>/<tag...>/symbol:<symbol>

The source is the signal that wrote the layer; labels name the float64s in
order, and segments past them are "key:value" tags. The symbol tag is last
because a symbol carries a "/" itself (BTC/USD): everything after "/symbol:"
is the symbol. The index never embeds the Value's Prefix, so a layer reads the
same on the stored tick and on any Value it is copied to.
*/
const (
	signalNamespace = "signal"
	symbolTag       = "symbol:"
)

/*
SignalIndex is the layer index AppendSignalVector writes for source and
symbol with keys.
*/
func SignalIndex(source, symbol string, keys ...string) string {
	parts := append([]string{signalNamespace, source}, keys...)
	return strings.Join(append(parts, symbolTag+symbol), "/")
}

/*
AppendSignalVector is AppendVector for one signal source and one symbol of a
multi-symbol tick, indexed by SignalIndex.
*/
func AppendSignalVector[T any](value *Value, source, symbol string, vector []T, keys ...string) *Value {
	if value == nil {
		return nil
	}

	value.claim(vectorBytes(vector), SignalIndex(source, symbol, keys...))

	return value
}

/*
IsSignalIndex reports whether index is in the signal layer namespace.
*/
func IsSignalIndex(index string) bool {
	return strings.HasPrefix(index, signalNamespace+"/")
}

/*
Excursions: detect writes one Value per excursion, Origin ExcursionOrigin,
Role its class, Scope its symbol, Tick its C tick.
*/
const ExcursionOrigin = "detector"

var ExcursionClasses = []string{"up", "down", "chop", "flat"}

/*
ExcursionKeys label an excursion's vector, in order. B and C are the
ignition and exhaustion trades; A and D bound the precursor and aftermath
windows by half the move on either side (A clamped to the symbol's first
trade in the epoch). b_ask and c_bid are the touch the tick recorded at B and C.

entry and exit are the executable points: the last trades at or before B and
C pulled back by detector.execution_lead_fraction of the precursor (A..B)
length in ticks, with the touch their ticks recorded. Train learns and
prices entry/exit; B and C stay for audit and hindsight.
*/
var ExcursionKeys = []string{
	"a_idx", "a_tick", "b_idx", "b_tick", "c_idx", "c_tick", "d_idx", "d_tick",
	"b_price", "c_price", "b_ask", "c_bid", "b_at", "c_at",
	"entry_idx", "entry_tick", "exit_idx", "exit_tick",
	"entry_ask", "exit_bid", "entry_at", "exit_at",
}

/*
ExcursionPrefixes are the keys of every excursion of symbol in epoch, one
prefix per class.
*/
func ExcursionPrefixes(symbol string, epoch uint64) []string {
	prefixes := make([]string, 0, len(ExcursionClasses))

	for _, class := range ExcursionClasses {
		prefixes = append(prefixes, Prefix(ExcursionOrigin, class, symbol, epoch))
	}

	return prefixes
}

/*
ExcursionFields reads every ExcursionKeys field of an excursion Value; a
missing one is an error.
*/
func ExcursionFields(value *Value) (map[string]float64, error) {
	if value == nil || value.Origin != ExcursionOrigin {
		return nil, errnie.Err(errnie.Validation, "data: not an excursion Value", nil)
	}

	metrics := Metrics(value)
	fields := make(map[string]float64, len(ExcursionKeys))

	for _, key := range ExcursionKeys {
		field, ok := metrics[key]

		if !ok {
			return nil, errnie.Err(errnie.UnprocessableContent, "data: excursion "+value.Prefix+" has no "+key, nil)
		}

		fields[key] = field
	}

	return fields, nil
}

/*
DetectorWatermark is what one detect run of an epoch labelled: collect may
still be writing the epoch, so train replays only the ticks detect saw and
checks it sees exactly them. detect writes it as one Value, Origin
ExcursionOrigin, Role DetectorWatermarkRole, Scope PolicyScope, Epoch the
tick epoch and Tick its last tick, carrying one vector labelled
DetectorWatermarkKeys. detect replaces it on every run; a run that fails
leaves none, so train refuses the epoch until detect completes.

	| key        | meaning                                        |
	|------------|------------------------------------------------|
	| first_tick | the epoch's first stored tick detect read      |
	| last_tick  | the last stored tick detect read (the watermark) |
	| ticks      | how many stored ticks detect read              |
	| symbols    | how many symbols detect labelled               |
*/
type DetectorWatermark struct {
	Epoch     uint64
	FirstTick uint64
	LastTick  uint64
	Ticks     int
	Symbols   int
}

const DetectorWatermarkRole = "watermark"

var DetectorWatermarkKeys = []string{"first_tick", "last_tick", "ticks", "symbols"}

/*
DetectorWatermarkPrefix is the key prefix of epoch's watermark.
*/
func DetectorWatermarkPrefix(epoch uint64) string {
	return Prefix(ExcursionOrigin, DetectorWatermarkRole, PolicyScope, epoch)
}

func (watermark DetectorWatermark) String() string {
	return fmt.Sprintf(
		"detect watermark of epoch %d: ticks %d..%d (%d ticks), %d symbols",
		watermark.Epoch, watermark.FirstTick, watermark.LastTick, watermark.Ticks, watermark.Symbols,
	)
}

/*
Value is the stored form of watermark.
*/
func (watermark DetectorWatermark) Value() *Value {
	value := &Value{
		Origin: ExcursionOrigin, Role: DetectorWatermarkRole, Scope: PolicyScope,
		Epoch: watermark.Epoch, Tick: watermark.LastTick,
	}
	value.FormatPrefix()

	return AppendVector(value, []float64{
		float64(watermark.FirstTick), float64(watermark.LastTick), float64(watermark.Ticks), float64(watermark.Symbols),
	}, DetectorWatermarkKeys...)
}

/*
ReadDetectorWatermark reads a stored watermark; a foreign Value, a missing
field, or a last tick that disagrees with the key is an error.
*/
func ReadDetectorWatermark(value *Value) (DetectorWatermark, error) {
	if value == nil || value.Origin != ExcursionOrigin || value.Role != DetectorWatermarkRole {
		return DetectorWatermark{}, errnie.Err(errnie.Validation, "data: not a detector watermark Value", nil)
	}

	metrics := Metrics(value)
	fields := make([]float64, len(DetectorWatermarkKeys))

	for idx, key := range DetectorWatermarkKeys {
		field, ok := metrics[key]

		if !ok {
			return DetectorWatermark{}, errnie.Err(errnie.UnprocessableContent, "data: watermark "+value.Prefix+" has no "+key, nil)
		}

		fields[idx] = field
	}

	watermark := DetectorWatermark{
		Epoch: value.Epoch, FirstTick: uint64(fields[0]), LastTick: uint64(fields[1]),
		Ticks: int(fields[2]), Symbols: int(fields[3]),
	}

	if watermark.LastTick != value.Tick || watermark.FirstTick > watermark.LastTick || watermark.Ticks <= 0 {
		return DetectorWatermark{}, errnie.Err(errnie.UnprocessableContent, fmt.Sprintf(
			"data: watermark %s is inconsistent: %s, key tick %d", value.Prefix, watermark, value.Tick,
		), nil)
	}

	return watermark, nil
}

/*
Policies: train publishes one Value per learned grammar path, Origin
PolicyOrigin, Role its action, Scope PolicyScope (pooled across symbols),
Tick a per-epoch sequence.
*/
const (
	PolicyOrigin  = "policy"
	PolicyScope   = "all"
	PolicyPathTag = "path:"
)

var PolicyActions = []string{"enter", "exit", "noop"}

/*
PolicyKeys label a policy's vector, in order; PolicyPathTag+<path> follows.
*/
var PolicyKeys = []string{"gain", "hold_seconds", "visits", "mean_reward", "confidence"}

/*
PolicyGate is the chord gate a policy epoch was trained under: train mints a
fresh policy epoch per tick epoch it trains on, so a gated and an ungated
training of the same ticks coexist. Every policy Value of the epoch carries
it as a second layer, PolicyGateKeys labelled and tagged PolicyGateLayer,
PolicyWarmupTag+<warm-up rule> and PolicyTicksTag+<tick epoch> (a tag: an
epoch exceeds what a float64 holds).

	| key              | meaning                                                 |
	|------------------|---------------------------------------------------------|
	| enabled          | 1 when resonance.gate was on, 0 when off                |
	| surprise_z       | resonance.surprise_z: the causal z a chord had to reach  |
	| warmup_fast_rate | the fast error average's rate of the settled warm-up    |
	| warmup_slow_rate | the slow error average's rate of the settled warm-up    |
	| learning_rate    | resonance.learning_rate: the solver's pace              |

Policies published before the settled warm-up carry the legacy layout
(PolicyLegacyGateKeys, warmup_ticks instead of the two rates, no warm-up
tag); they read back as the WarmupTicksRule warm-up, which no current gate
is Same as.
*/
type PolicyGate struct {
	Epoch        uint64
	TickEpoch    uint64
	Enabled      bool
	SurpriseZ    float64
	Warmup       PolicyWarmup
	LearningRate float64
}

/*
PolicyWarmup is the rule that decided when a symbol's gate opened.
WarmupSettled: once the coder's error stopped falling, its average at
FastRate no longer below its average at SlowRate. WarmupTicksRule (legacy,
read only): after Ticks solver steps.
*/
type PolicyWarmup struct {
	Rule     string
	FastRate float64
	SlowRate float64
	Ticks    int
}

const (
	WarmupSettled   = "settled"
	WarmupTicksRule = "ticks"
)

func (warmup PolicyWarmup) String() string {
	if warmup.Rule == WarmupTicksRule {
		return fmt.Sprintf("after %d ticks (legacy)", warmup.Ticks)
	}

	return fmt.Sprintf("%s: error average at %g no longer below its average at %g", warmup.Rule, warmup.FastRate, warmup.SlowRate)
}

const (
	PolicyGateLayer = "gate"
	PolicyWarmupTag = "warmup:"
	PolicyTicksTag  = "ticks:"
)

var (
	PolicyGateKeys       = []string{"enabled", "surprise_z", "warmup_fast_rate", "warmup_slow_rate", "learning_rate"}
	PolicyLegacyGateKeys = []string{"enabled", "surprise_z", "warmup_ticks", "learning_rate"}
)

/*
Same reports whether gate and other were trained under the same gate
settings; which epochs they belong to does not matter.
*/
func (gate PolicyGate) Same(other PolicyGate) bool {
	return gate.Enabled == other.Enabled && gate.SurpriseZ == other.SurpriseZ &&
		gate.Warmup == other.Warmup && gate.LearningRate == other.LearningRate
}

func (gate PolicyGate) String() string {
	settings := fmt.Sprintf(
		"gate enabled=%t surprise_z=%g warm-up %s learning_rate=%g",
		gate.Enabled, gate.SurpriseZ, gate.Warmup, gate.LearningRate,
	)

	// A configuration (no epochs: what a run is set up for) is only its
	// settings; a stored gate also names where it was trained.
	if gate.Epoch == 0 && gate.TickEpoch == 0 {
		return settings
	}

	return fmt.Sprintf("%s (policy epoch %d, ticks %d)", settings, gate.Epoch, gate.TickEpoch)
}

/*
AppendPolicyGate writes gate as value's gate layer. Only the settled warm-up
is written; the legacy ticks rule is read, never published.
*/
func AppendPolicyGate(value *Value, gate PolicyGate) error {
	if gate.Warmup.Rule != WarmupSettled {
		return errnie.Err(errnie.Validation, fmt.Sprintf(
			"data: policy %s gate warm-up %q is not %q", value.Prefix, gate.Warmup.Rule, WarmupSettled,
		), nil)
	}

	enabled := 0.0

	if gate.Enabled {
		enabled = 1
	}

	AppendVector(value, []float64{enabled, gate.SurpriseZ, gate.Warmup.FastRate, gate.Warmup.SlowRate, gate.LearningRate},
		append(slices.Clone(PolicyGateKeys),
			PolicyGateLayer, PolicyWarmupTag+gate.Warmup.Rule, PolicyTicksTag+strconv.FormatUint(gate.TickEpoch, 10),
		)...,
	)

	return nil
}

/*
ReadPolicyGate reads the gate layer of a policy Value, in the current or the
legacy layout; a policy without one (published before the gate existed) or
with a malformed one is an error.
*/
func ReadPolicyGate(value *Value) (PolicyGate, error) {
	current := value.Prefix + "/" + strings.Join(PolicyGateKeys, "/") + "/" + PolicyGateLayer + "/" + PolicyWarmupTag
	legacy := value.Prefix + "/" + strings.Join(PolicyLegacyGateKeys, "/") + "/" + PolicyGateLayer + "/" + PolicyTicksTag

	for index, vector := range Indexed[[]float64](value, current, legacy) {
		var gate PolicyGate
		var ticks string

		switch {
		case strings.HasPrefix(index, current):
			rule, rest, cut := strings.Cut(strings.TrimPrefix(index, current), "/")
			epochTag, tagged := strings.CutPrefix(rest, PolicyTicksTag)
			ticks = epochTag

			if !cut || !tagged || rule != WarmupSettled {
				return PolicyGate{}, errnie.Err(errnie.UnprocessableContent, "data: policy "+value.Prefix+" gate tag "+index+" names no known warm-up and tick epoch", nil)
			}

			if len(vector) != len(PolicyGateKeys) {
				return PolicyGate{}, gateWidth(value, vector, PolicyGateKeys)
			}

			gate.Warmup = PolicyWarmup{Rule: rule, FastRate: vector[2], SlowRate: vector[3]}
			gate.LearningRate = vector[4]
		case strings.HasPrefix(index, legacy):
			ticks = strings.TrimPrefix(index, legacy)

			if len(vector) != len(PolicyLegacyGateKeys) {
				return PolicyGate{}, gateWidth(value, vector, PolicyLegacyGateKeys)
			}

			gate.Warmup = PolicyWarmup{Rule: WarmupTicksRule, Ticks: int(vector[2])}
			gate.LearningRate = vector[3]
		default:
			continue
		}

		epoch, err := strconv.ParseUint(ticks, 10, 64)

		if err != nil || epoch == 0 {
			return PolicyGate{}, errnie.Err(errnie.UnprocessableContent, "data: policy "+value.Prefix+" gate tag "+index+" names no tick epoch", err)
		}

		if vector[0] != 0 && vector[0] != 1 {
			return PolicyGate{}, errnie.Err(errnie.UnprocessableContent, fmt.Sprintf(
				"data: policy %s gate enabled is %g, not 0 or 1", value.Prefix, vector[0],
			), nil)
		}

		gate.Epoch, gate.TickEpoch, gate.Enabled, gate.SurpriseZ = value.Epoch, epoch, vector[0] == 1, vector[1]
		return gate, nil
	}

	return PolicyGate{}, errnie.Err(errnie.UnprocessableContent, "data: policy "+value.Prefix+" carries no gate tag: it was trained before the gate, retrain it", nil)
}

func gateWidth(value *Value, vector []float64, keys []string) error {
	return errnie.Err(errnie.UnprocessableContent, fmt.Sprintf(
		"data: policy %s gate layer holds %d values for %d keys", value.Prefix, len(vector), len(keys),
	), nil)
}

/*
PolicyPrefixes are the keys of every policy of epoch, or of every epoch when
epoch is omitted, one prefix per action.
*/
func PolicyPrefixes(epoch ...uint64) []string {
	prefixes := make([]string, 0, len(PolicyActions))

	for _, action := range PolicyActions {
		prefixes = append(prefixes, Prefix(PolicyOrigin, action, PolicyScope, epoch...))
	}

	return prefixes
}

/*
AppendPolicy writes a policy's vector (PolicyKeys order) and its path tag.
*/
func AppendPolicy(value *Value, path string, vector []float64) error {
	if len(vector) != len(PolicyKeys) {
		return errnie.Err(errnie.Validation, fmt.Sprintf(
			"data: policy %s has %d values for %d keys", value.Prefix, len(vector), len(PolicyKeys),
		), nil)
	}

	AppendVector(value, vector, append(slices.Clone(PolicyKeys), PolicyPathTag+path)...)
	return nil
}

/*
PolicyPath reads the grammar path a policy Value was published for.
*/
func PolicyPath(value *Value) (string, error) {
	head := value.Prefix + "/"

	for _, index := range value.Index {
		rest, ok := strings.CutPrefix(index, head)

		if !ok {
			continue
		}

		if _, path, found := strings.Cut(rest, "/"+PolicyPathTag); found && path != "" {
			return path, nil
		}
	}

	return "", errnie.Err(errnie.UnprocessableContent, "data: policy "+value.Prefix+" has no "+PolicyPathTag+" tag", nil)
}

/*
Coder state: one Value per symbol the chord gate's solver learned, written
by train under the policy epoch it publishes (Epoch), Tick the last tick it
trained on, so a later train continues the newest state trained under the
same gate settings and make run the state of the policy epoch it trades.
Origin CoderOrigin, Role CoderStateRole, Scope the symbol. Layers:

	| layer            | content                                                  |
	|------------------|----------------------------------------------------------|
	| CoderStateKeys   | float64 vector, below                                    |
	| CoderStateLayer  | the solver's symbol state (logic/resonance.Solver.SaveSymbol) |
	| PolicyGateLayer  | the gate it was trained under (as on that epoch's policies) |

	| key            | meaning                                                    |
	|----------------|------------------------------------------------------------|
	| pending_dropped| predictions issued and not yet resolved when it was saved; dropped, never resolved across the gap |
	| settle_steps   | the gate warm-up's error count (strategy settling)         |
	| settle_fast    | its fast error average                                     |
	| settle_slow    | its slow error average                                     |
	| warm           | 1 once the symbol warmed up, else 0                        |
	| surprise_count | warm solver steps folded into the surprise moments          |
	| surprise_mean  | their mean surprise                                        |
	| surprise_m2    | their sum of squared deviations (Welford)                  |
*/
const (
	CoderOrigin     = "resonance"
	CoderStateRole  = "state"
	CoderStateLayer = "coder"
)

var CoderStateKeys = []string{
	"pending_dropped", "settle_steps", "settle_fast", "settle_slow", "warm",
	"surprise_count", "surprise_mean", "surprise_m2",
}

/*
CoderStatesPrefix is the prefix of every stored coder state.
*/
func CoderStatesPrefix() string { return CoderOrigin + "/" + CoderStateRole + "/" }

/*
CoderStateEpoch is the policy epoch of a stored coder state key (its
second-to-last segment: a symbol holds "/").
*/
func CoderStateEpoch(key string) (uint64, error) {
	segments := strings.Split(key, "/")

	if !strings.HasPrefix(key, CoderStatesPrefix()) || len(segments) < 5 || len(segments[len(segments)-2]) != 19 {
		return 0, errnie.Err(errnie.UnprocessableContent, "data: coder state key "+key+" has no %019d policy epoch", nil)
	}

	epoch, err := strconv.ParseUint(segments[len(segments)-2], 10, 64)

	if err != nil {
		return 0, errnie.Err(errnie.UnprocessableContent, "data: coder state key "+key+" has no %019d policy epoch", err)
	}

	return epoch, nil
}

/*
CoderState is one stored coder state's content.
*/
type CoderState struct {
	Symbol string
	Gate   PolicyGate
	Fields map[string]float64
	Blob   []byte
}

/*
AppendCoderState writes vector (CoderStateKeys order), the solver state
blob and the gate onto value.
*/
func AppendCoderState(value *Value, vector []float64, blob []byte, gate PolicyGate) error {
	if len(vector) != len(CoderStateKeys) {
		return errnie.Err(errnie.Validation, fmt.Sprintf(
			"data: coder state %s has %d values for %d keys", value.Prefix, len(vector), len(CoderStateKeys),
		), nil)
	}

	if len(blob) == 0 {
		return errnie.Err(errnie.Validation, "data: coder state "+value.Prefix+" has no solver state", nil)
	}

	AppendVector(value, vector, CoderStateKeys...)
	// A raw layer: AppendJSON copies the bytes and indexes them; it does
	// not parse them.
	value.AppendJSON(blob, value.Prefix+"/"+CoderStateLayer)
	return AppendPolicyGate(value, gate)
}

/*
ReadCoderState reads a stored coder state; one missing a layer, or whose
gate does not read, is an error.
*/
func ReadCoderState(value *Value) (CoderState, error) {
	state := CoderState{Symbol: value.Scope}

	if value.Origin != CoderOrigin || value.Role != CoderStateRole || value.Scope == "" {
		return state, errnie.Err(errnie.UnprocessableContent, "data: "+value.Prefix+" is not a coder state", nil)
	}

	vectorIndex := value.Prefix + "/" + strings.Join(CoderStateKeys, "/")
	blobIndex := value.Prefix + "/" + CoderStateLayer

	for index, layer := range value.Layers[:value.visible()] {
		if index >= len(value.Index) {
			break
		}

		switch value.Index[index] {
		case vectorIndex:
			vector, err := floats(layer, len(CoderStateKeys))

			if err != nil {
				return state, errnie.Err(errnie.UnprocessableContent, "data: coder state "+value.Prefix, err)
			}

			state.Fields = make(map[string]float64, len(CoderStateKeys))

			for key, name := range CoderStateKeys {
				state.Fields[name] = vector[key]
			}
		case blobIndex:
			state.Blob = layer
		}
	}

	if state.Fields == nil || len(state.Blob) == 0 {
		return state, errnie.Err(errnie.UnprocessableContent, "data: coder state "+value.Prefix+" lacks its "+
			strings.Join(CoderStateKeys, "/")+" vector or its "+CoderStateLayer+" layer", nil)
	}

	gate, err := ReadPolicyGate(value)

	if err != nil {
		return state, err
	}

	state.Gate = gate
	return state, nil
}

/*
floats reads a float64 vector layer of exactly n values.
*/
func floats(layer []byte, n int) ([]float64, error) {
	if len(layer) != n*8 {
		return nil, fmt.Errorf("layer holds %d bytes, not %d float64s", len(layer), n)
	}

	out := make([]float64, n)

	for index := range out {
		out[index] = math.Float64frombits(binary.LittleEndian.Uint64(layer[index*8:]))
	}

	return out, nil
}

/*
Paper records: one Value per paper decision that traded a round trip (its
entry and its exit decision each get one), Origin PaperOrigin, Role the
decision's action, Scope the symbol, Epoch the run's epoch and Tick a
per-run sequence. The vector is labelled PaperKeys, followed by the tags
PolicyPathTag+<matched stored path> and PaperPolicyEpochTag+<policy epoch>.

	| key             | meaning                                                   |
	|-----------------|-----------------------------------------------------------|
	| depth           | tokens in the matched stored path (prefix match depth)    |
	| intended_price  | the touch when this decision was taken (ask enter, bid exit) |
	| fill_price      | this decision's simulated average fill price              |
	| decided_at      | unix ns the decision was taken                            |
	| filled_at       | unix ns of its last fill (venue timestamp)                |
	| latency_seconds | filled_at - decided_at                                    |
	| slippage        | adverse fill vs intended, as a fraction of intended       |
	| qty             | base quantity of the round trip                           |
	| entry_price     | average entry fill price                                  |
	| exit_price      | average exit fill price                                   |
	| entered_at      | unix ns of the entry's last fill                          |
	| exited_at       | unix ns of the exit's last fill                           |
	| fees            | entry + exit fees in the quote currency                   |
	| pnl             | exit proceeds - entry cost, after fees (slippage is in the fills) |
	| net_return      | pnl / (entry_price*qty + entry fee): the trainer's gain unit |
*/
const (
	PaperOrigin         = "paper"
	PaperPolicyEpochTag = "policy_epoch:"
)

var PaperActions = []string{"enter", "exit"}

var PaperKeys = []string{
	"depth", "intended_price", "fill_price", "decided_at", "filled_at",
	"latency_seconds", "slippage", "qty", "entry_price", "exit_price",
	"entered_at", "exited_at", "fees", "pnl", "net_return",
}

/*
PaperPrefixes are the keys of every paper record of symbol, one prefix per
action, narrowed to one run epoch when given.
*/
func PaperPrefixes(symbol string, epoch ...uint64) []string {
	prefixes := make([]string, 0, len(PaperActions))

	for _, action := range PaperActions {
		prefixes = append(prefixes, Prefix(PaperOrigin, action, symbol, epoch...))
	}

	return prefixes
}

/*
PaperActionPrefixes are the keys of every paper record of every symbol and
run, one prefix per action.
*/
func PaperActionPrefixes() []string {
	prefixes := make([]string, 0, len(PaperActions))

	for _, action := range PaperActions {
		prefixes = append(prefixes, PaperOrigin+"/"+action+"/")
	}

	return prefixes
}

/*
AppendPaper writes a paper record's vector (PaperKeys order) and its tags.
*/
func AppendPaper(value *Value, path string, policyEpoch uint64, vector []float64) error {
	if len(vector) != len(PaperKeys) {
		return errnie.Err(errnie.Validation, fmt.Sprintf(
			"data: paper record %s has %d values for %d keys", value.Prefix, len(vector), len(PaperKeys),
		), nil)
	}

	if path == "" {
		return errnie.Err(errnie.Validation, "data: paper record "+value.Prefix+" has no matched path", nil)
	}

	AppendVector(value, vector, append(
		slices.Clone(PaperKeys), PolicyPathTag+path, fmt.Sprintf("%s%d", PaperPolicyEpochTag, policyEpoch),
	)...)

	return nil
}

/*
PaperRecord is a paper Value read back: its fields, the matched stored path
and the policy epoch that path came from.
*/
type PaperRecord struct {
	Action      string
	Symbol      string
	Path        string
	PolicyEpoch uint64
	Fields      map[string]float64
}

/*
PaperFields reads a paper Value; a missing key or tag is an error.
*/
func PaperFields(value *Value) (PaperRecord, error) {
	if value == nil || value.Origin != PaperOrigin {
		return PaperRecord{}, errnie.Err(errnie.Validation, "data: not a paper Value", nil)
	}

	record := PaperRecord{Action: value.Role, Symbol: value.Scope, Fields: make(map[string]float64, len(PaperKeys))}
	metrics := Metrics(value)

	for _, key := range PaperKeys {
		field, ok := metrics[key]

		if !ok {
			return PaperRecord{}, errnie.Err(errnie.UnprocessableContent, "data: paper record "+value.Prefix+" has no "+key, nil)
		}

		record.Fields[key] = field
	}

	head := value.Prefix + "/"

	for _, index := range value.Index {
		rest, ok := strings.CutPrefix(index, head)

		if !ok {
			continue
		}

		for _, tag := range strings.Split(rest, "/") {
			if epoch, found := strings.CutPrefix(tag, PaperPolicyEpochTag); found {
				parsed, err := strconv.ParseUint(epoch, 10, 64)

				if err != nil {
					return PaperRecord{}, errnie.Err(errnie.UnprocessableContent, "data: paper record "+value.Prefix+" policy epoch "+epoch, err)
				}

				record.PolicyEpoch = parsed
			}
		}

		if _, path, found := strings.Cut(rest, "/"+PolicyPathTag); found {
			path, _, _ = strings.Cut(path, "/"+PaperPolicyEpochTag)
			record.Path = path
		}
	}

	if record.Path == "" || record.PolicyEpoch == 0 {
		return PaperRecord{}, errnie.Err(errnie.UnprocessableContent, "data: paper record "+value.Prefix+" has no path or policy epoch tag", nil)
	}

	return record, nil
}

/*
Hindsight frames replay one stored run to the one dashboard client that sent
the hub's "hindsight" command; they are never stored. Origin HindsightOrigin,
Epoch the run (the collect epoch), Scope the symbol ("" for the run index).

	| Role                       | layers                                                 |
	|----------------------------|--------------------------------------------------------|
	| HindsightTick              | HindsightTickKeys: the symbol at one stored tick (Tick, Timestamp) |
	| ExcursionOrigin            | a stored excursion, verbatim: its index names its key   |
	| PolicyOrigin               | a policy published for the run, verbatim                |
	| PaperOrigin                | a paper record traded under the run's policies, verbatim |
	| HindsightSymbol            | HindsightSymbolKeys: one symbol the run traded (Scope "", symbol in the key) |
	| HindsightDone              | HindsightDoneKeys: the stream is complete               |
	| HindsightError             | LayerHindsightError: the error text; the stream ended on it |

A tick's bid or ask is NaN where its price layer holds no such side.

The hub's "detections" command (the Historical runs tab) streams what detect
wrote, in the same frames but under Origin DetectionsOrigin, so neither
stream takes the other's frames: with no run, one HindsightDetectionRun per epoch
detect labelled (its watermark, Epoch the tick epoch), newest first; with a
run, every excursion of it (ExcursionOrigin, verbatim); with a run, a symbol,
a class and a C tick, that excursion's trades from its A tick to its D tick,
one HindsightTrades frame per stored tick (Tick, Timestamp the tick's), its
layers DetectionTradeLayers, each one value per trade of the symbol at that
tick. A done frame closes each complete stream.

	| Role                       | layers                                                 |
	|----------------------------|--------------------------------------------------------|
	| HindsightDetectionRun      | DetectionRunKeys: the watermark and the class counts   |
	| HindsightTrades            | DetectionTradeLayers: seq (the symbol's 1-based trade index in the epoch, detect's a_idx..d_idx), price, at (trade time, ns) |
*/
const (
	HindsightOrigin     = "hindsight"
	HindsightTick       = "tick"
	HindsightSymbol     = "symbol"
	HindsightDone       = "done"
	HindsightError      = "error"
	LayerHindsightError = "error"

	// DetectionsOrigin is the origin of the "detections" command's frames
	// (the roles below and the Hindsight ones they share), never stored.
	DetectionsOrigin      = "detections"
	HindsightDetectionRun = "detection_run"
	HindsightTrades       = "trades"
)

/*
DetectionRunKeys are DetectorWatermarkKeys, then the epoch's stored
excursions per ExcursionClasses.
*/
var DetectionRunKeys = append(slices.Clone(DetectorWatermarkKeys), ExcursionClasses...)

/*
DetectionTradeLayers are a HindsightTrades frame's layers, each one vector
indexed <prefix>/<layer>.
*/
var DetectionTradeLayers = []string{"seq", "price", "at"}

var HindsightTickKeys = []string{"bid", "ask", "trade_price", "trade_qty", "trades"}

/*
HindsightSymbolKeys: touched counts ticks whose price layer held both sides;
top_excursion is the largest up or down excursion move (c_price-b_price)/b_price,
signed, 0 with none.
*/
var HindsightSymbolKeys = []string{
	"ticks", "touched", "trades", "first_tick", "last_tick", "first_at", "last_at",
	"low", "high", "up", "down", "top_excursion",
}

var HindsightDoneKeys = []string{"ticks", "symbols", "excursions", "policies", "papers"}
