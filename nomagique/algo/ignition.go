package algo

import (
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/types"
)

type ignitionMap = types.Map[string, types.Value[float64]]
type ignitionCollection = types.Map[string, ignitionMap]
type ignitionState = types.Pair[string, ignitionCollection]

const (
	ignitionCapacity = "capacity"
	ignitionVolume   = "volume"
	ignitionLast     = "last"
	ignitionBid      = "bid"
	ignitionAsk      = "ask"
	ignitionUnixSec  = "unix_sec"
	ignitionUnixNsec = "unix_nsec"

	ignitionInitialized = "window/initialized"
	ignitionClassified  = "window/classified"
	ignitionBars        = "window/bars"
	ignitionHaveTime    = "window/have_time"
	ignitionLastSec     = "window/last_sec"
	ignitionLastNsec    = "window/last_nsec"
	ignitionBarOpenSec  = "window/bar_open_sec"
	ignitionBarOpenNsec = "window/bar_open_nsec"
	ignitionBarVolume   = "window/bar_volume"
	ignitionPrevClose   = "window/previous_close"
	ignitionLastRVOL    = "window/previous_rvol"
)

/*
Ignition is a keyed, causal volume-clock composition. All observation, retained
history, calibration, and output state lives in the map carried by initial and
next; the algorithm has no domain-specific state fields.
*/
type Ignition struct {
	initial types.Input[ignitionState]
	next    types.Input[ignitionState]
}

var _ types.IO[ignitionState] = (*Ignition)(nil)

/*
NewIgnition creates a keyed ignition collection. The pair key selects the
active stream; the collection map carries every stream's observation, history,
calibration, and output map.
*/
func NewIgnition(initial types.Input[ignitionState]) *Ignition {
	return &Ignition{
		initial: initial,
		next:    types.NewInput[ignitionState](),
	}
}

/*
Write stages an observation collection and overlays the pair-selected stream
map on its last committed map. Other keyed streams remain untouched.
*/
func (ignition *Ignition) Write(input types.IO[ignitionState]) {
	if input == nil {
		ignition.reject(ignitionError("input is nil"))
		return
	}
	if input.Error() != "" {
		ignition.reject(errnie.Error(errnie.Err(
			errnie.NotFound,
			input.Error(),
			nil,
		)))
		return
	}

	incoming := input.Project().Read()
	candidate := incoming.Value.Clone()
	committed, hasCommitted := ignition.committed()

	if hasCommitted {
		candidate = committed.Value.Clone()
		incomingMap, found := incoming.Value.Get(incoming.Key)
		if !found {
			ignition.reject(ignitionError("active stream is missing from collection"))
			return
		}

		merged := incomingMap.Clone()
		if prior, hasPrior := candidate.Get(incoming.Key); hasPrior {
			merged = prior.Clone()
			for key, value := range incomingMap.All() {
				merged.Put(key, value)
			}
		}
		candidate.Put(incoming.Key, merged)
	}

	ignition.next = types.NewInput(types.NewValue(types.NewPair(incoming.Key, candidate)))
}

/*
Read advances the staged observation transactionally. Validation or primitive
failure leaves initial untouched and exposes the failure through next.
*/
func (ignition *Ignition) Read() types.IO[ignitionState] {
	if ignition.next.Error() != "" {
		return ignition.next
	}

	staged := ignition.next.Project()
	if !staged.Ready {
		ignition.reject(ignitionError("input has not been written"))
		return ignition.next
	}

	pair := staged.Read()
	if pair.Key == "" {
		ignition.reject(ignitionError("missing stream key"))
		return ignition.next
	}

	stream, found := pair.Value.Get(pair.Key)
	if !found {
		ignition.reject(ignitionError("active stream is missing from collection"))
		return ignition.next
	}
	mapping := stream.Clone()
	capacity, volume, last, bid, ask, sec, nsec, hasTime, err :=
		ignitionObservation(mapping)
	if err != nil {
		ignition.reject(err)
		return ignition.next
	}

	spread := ask - bid
	if !ignitionFlag(mapping, ignitionInitialized) {
		if err := ignition.initialize(mapping, capacity, volume, last, spread, sec, nsec, hasTime); err != nil {
			ignition.reject(err)
			return ignition.next
		}
	} else if err := ignition.advance(mapping, capacity, volume, last, spread, sec, nsec, hasTime); err != nil {
		ignition.reject(err)
		return ignition.next
	}

	ignition.compose(mapping, spread)
	pair.Value.Put(pair.Key, mapping)
	ignition.commit(pair)
	return ignition.next
}

func (ignition *Ignition) Project() types.Value[ignitionState] {
	return ignition.next.Project()
}

func (ignition *Ignition) Error() string {
	return ignition.next.Error()
}

func (ignition *Ignition) Close() error {
	if ignition.initial != nil {
		if err := ignition.initial.Close(); err != nil {
			return err
		}
	}
	if ignition.next != nil {
		if err := ignition.next.Close(); err != nil {
			return err
		}
	}

	ignition.initial = types.NewInput[ignitionState]()
	ignition.next = types.NewInput[ignitionState]()
	return nil
}

func (ignition *Ignition) commit(state ignitionState) {
	ignition.initial = types.NewInput(types.NewValue(state))
	ignition.next = types.NewInput(types.NewValue(state))
}

func (ignition *Ignition) reject(err error) {
	state, found := ignition.committed()
	if !found {
		state = ignitionState{}
	}
	ignition.next = types.NewErrorInput(state, err)
}

func (ignition *Ignition) committed() (ignitionState, bool) {
	if ignition.initial == nil {
		return ignitionState{}, false
	}
	projected := ignition.initial.Project()
	if !projected.Ready {
		return ignitionState{}, false
	}
	if projected.Err != nil {
		return projected.Zero, true
	}
	return projected.Read(), true
}

func ignitionObservation(
	mapping ignitionMap,
) (
	capacity float64,
	volume float64,
	last float64,
	bid float64,
	ask float64,
	sec float64,
	nsec float64,
	hasTime bool,
	err error,
) {
	var found bool
	capacity, found = ignitionLookup(mapping, ignitionCapacity)
	if !found || capacity <= 0 || capacity != math.Trunc(capacity) || !ignitionFinite(capacity) {
		err = ignitionError("positive integer capacity required")
		return
	}

	for key, target := range map[string]*float64{
		ignitionVolume: &volume,
		ignitionLast:   &last,
		ignitionBid:    &bid,
		ignitionAsk:    &ask,
	} {
		value, present := ignitionLookup(mapping, key)
		if !present || value <= 0 || !ignitionFinite(value) {
			err = ignitionError("volume, last, bid, and ask must be finite and positive")
			return
		}
		*target = value
	}
	if ask <= bid {
		err = ignitionError("ask must be above bid")
		return
	}

	sec, hasSec := ignitionLookup(mapping, ignitionUnixSec)
	nsec, hasNsec := ignitionLookup(mapping, ignitionUnixNsec)
	if hasSec != hasNsec {
		err = ignitionError("timestamp requires unix_sec and unix_nsec")
		return
	}
	if hasSec {
		if !ignitionFinite(sec, nsec) || nsec < 0 || nsec >= 1e9 {
			err = ignitionError("timestamp coordinates must be finite and normalized")
			return
		}
		hasTime = sec != 0 || nsec != 0
	}
	return
}

func ignitionParams(
	pairs ...types.Pair[string, types.Value[float64]],
) ignitionMap {
	mapping := types.NewMap[string, types.Value[float64]]()
	for _, pair := range pairs {
		mapping.Put(pair.Key, pair.Value)
	}
	return mapping
}

func ignitionParam(
	key string,
	value float64,
) types.Pair[string, types.Value[float64]] {
	return types.NewPair(key, types.NewValue(value))
}

func ignitionLookup(mapping ignitionMap, key string) (float64, bool) {
	value, found := mapping.Get(key)
	if !found {
		return 0, false
	}
	return value.Read(), true
}

func ignitionNumber(mapping ignitionMap, key string) float64 {
	value, _ := ignitionLookup(mapping, key)
	return value
}

func ignitionPut(mapping ignitionMap, key string, value float64) {
	mapping.Put(key, types.NewValue(value))
}

func ignitionFlag(mapping ignitionMap, key string) bool {
	return ignitionNumber(mapping, key) != 0
}

func ignitionBool(value bool) float64 {
	if value {
		return 1
	}
	return 0
}

func ignitionFinite(values ...float64) bool {
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
	}
	return true
}

func ignitionBefore(sec float64, nsec float64, otherSec float64, otherNsec float64) bool {
	return sec < otherSec || (sec == otherSec && nsec < otherNsec)
}

func ignitionAfter(sec float64, nsec float64, otherSec float64, otherNsec float64) bool {
	return sec > otherSec || (sec == otherSec && nsec > otherNsec)
}

func positiveOnly(value float64) bool {
	return value > 0 && ignitionFinite(value)
}

func nonNegative(value float64) bool {
	return value >= 0 && ignitionFinite(value)
}

func ignitionError(message string) error {
	return errnie.Error(errnie.Err(
		errnie.Validation,
		"ignition: "+message,
		nil,
	))
}
