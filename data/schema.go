package data

import (
	"fmt"
	"slices"
	"strings"

	"github.com/theapemachine/errnie"
)

/*
The storage schema: every key and layer the commands exchange, in one place.

	| writer  | key (Prefix + %019d tick .bin)                 | reader              |
	|---------|------------------------------------------------|---------------------|
	| collect | workspace/tick/aggregate/<epoch>/<tick>.bin    | detect, train, audit |
	| detect  | detector/<class>/<symbol>/<epoch>/<c_tick>.bin | train, audit        |
	| train   | policy/<action>/all/<epoch>/<seq>.bin          | live Training, audit |

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
policy Value one labelled PolicyKeys plus a trailing PolicyPathTag tag.

Hub-only Values (streamed to the dashboard, never stored):

	| writer             | Origin/Role/Scope           | layer                          |
	|--------------------|-----------------------------|--------------------------------|
	| run (strategy.Sequitur) | sequitur/grammar/<symbol> | LayerGrammar: strategy.SequiturFrame JSON |
*/

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
windows by half the move on either side. b_ask and c_bid are the touch the
tick recorded at B and C: the prices a long entry and exit cross.
*/
var ExcursionKeys = []string{
	"a_idx", "a_tick", "b_idx", "b_tick", "c_idx", "c_tick", "d_idx", "d_tick",
	"b_price", "c_price", "b_ask", "c_bid", "b_at", "c_at",
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
