package strategy

import (
	"context"
	"fmt"
	"math/rand/v2"
	"sort"
	"sync"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/runtime"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
)

/*
Rehearsal owns the offline replay workers. It discovers completed episodes and appends them to each worker's ring.
The mounted workload advances workers one historical observation or grade per event. The workers
never touch the live grid, the live book, or a live wallet.
*/
type Rehearsal struct {
	workers      int
	cursors      []*replayCursor
	Workload     *runtime.Workload[*types.Envelope]
	prepared     map[string]bool
	catalog      *tables.Catalog
	run          hindsight.RunID
	policy       hindsight.DiscoveryPolicy
	price        *broker.Price
	learner      *Learner
	observations map[string][]hindsight.Observation
	lastSequence int64
	inputs       map[hindsight.EnvelopeRef]hindsight.RehearsalInput
	witnessed    map[tables.EnvelopeRefRow]bool
	progress     wire.LearningRehearsalT
	err          error
	mutex        sync.Mutex
}

/* NewRehearsal creates workers that retain their own historical evidence. */
func NewRehearsal(
	ctx context.Context,
	workers int,
	catalog *tables.Catalog,
	run hindsight.RunID,
	policy hindsight.DiscoveryPolicy,
	price *broker.Price,
	learner *Learner,
) *Rehearsal {
	if err := errnie.Require(map[string]any{
		"workers": workers,
		"learner": learner,
		"price":   price,
		"catalog": catalog,
		"policy":  policy,
		"run":     run,
	}); err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation, "learning: rehearsal", err,
		))

		return nil
	}

	rehearsal := &Rehearsal{
		observations: make(map[string][]hindsight.Observation),
		prepared:     make(map[string]bool), workers: workers,
		catalog: catalog, run: run, policy: policy, price: price, learner: learner,
		progress: wire.LearningRehearsalT{Workers: int32(workers), Status: "waiting for archive"},
	}
	nodes := make([]runtime.Node[*types.Envelope], workers)
	for index := range nodes {
		cursor := &replayCursor{ctx: ctx, rehearsal: rehearsal, learned: cognition.NewEngine(cognition.DefaultConfig())}
		rehearsal.cursors = append(rehearsal.cursors, cursor)
		nodes[index] = cursor
	}
	rehearsal.Workload = runtime.NewWorkload(ctx, "learners", [][]runtime.Node[*types.Envelope]{nodes})
	return rehearsal
}

/* Error exposes the rehearsal failure through the runtime node protocol. */
func (rehearsal *Rehearsal) Error() error {
	rehearsal.mutex.Lock()
	defer rehearsal.mutex.Unlock()
	return rehearsal.err
}

func (rehearsal *Rehearsal) fail(err error) {
	rehearsal.mutex.Lock()
	defer rehearsal.mutex.Unlock()
	rehearsal.err = errnie.Error(err)
}

/*
Run refreshes the durable fragment pool; the mounted workload owns replay. An empty
archive is a valid first-run state: practice waits for captured precursors while
the live stages continue recording observations and evaluating the policy.
*/
func (rehearsal *Rehearsal) Run(ctx context.Context) error {
	rehearsal.mutex.Lock()
	rehearsal.progress.Status = "reading archive"
	rehearsal.mutex.Unlock()

	if err := rehearsal.read(ctx); err != nil {
		rehearsal.fail(err)
		return rehearsal.Error()
	}

	if len(rehearsal.inputs) == 0 {
		rehearsal.mutex.Lock()
		rehearsal.progress.Status = "waiting for captured precursors"
		rehearsal.mutex.Unlock()
		return nil
	}

	episodes := rehearsal.episodes()
	pool, err := rehearsal.balance(episodes)

	if err != nil {
		rehearsal.fail(err)
		return rehearsal.Error()
	}

	if len(pool) == 0 {
		return nil
	}

	var prepared [][]hindsight.Observation
	for _, episode := range pool {
		identity := fmt.Sprint(episode.Symbol, episode.Kind, episode.References)

		if rehearsal.prepared[identity] {
			continue
		}
		fragment, err := rehearsal.prepare(episode)

		if err != nil {
			return errnie.Error(err)
		}
		rehearsal.prepared[identity] = true
		prepared = append(prepared, fragment)
	}
	for _, cursor := range rehearsal.cursors {
		cursor.mutex.Lock()
		for _, selected := range rand.Perm(len(prepared)) {
			cursor.pending = append(cursor.pending, prepared[selected])
		}
		cursor.mutex.Unlock()
	}
	return rehearsal.Error()
}

func shuffledEpisodes(episodes []hindsight.Episode) []hindsight.Episode {
	shuffled := make([]hindsight.Episode, len(episodes))
	copy(shuffled, episodes)
	rand.Shuffle(len(shuffled), func(left, right int) {
		shuffled[left], shuffled[right] = shuffled[right], shuffled[left]
	})
	return shuffled
}

func (rehearsal *Rehearsal) episodes() []hindsight.Episode {
	episodes := make([]hindsight.Episode, 0, len(rehearsal.observations))

	for symbol, observations := range rehearsal.observations {
		// Inserted precursor decision coordinates carry an as-of quote. They are
		// not additional market observations for episode discovery.
		symbolObservations := make([]hindsight.Observation, 0, len(observations))
		for _, observation := range observations {
			if observation.Kind != "precursor" {
				symbolObservations = append(symbolObservations, observation)
			}
		}
		episodes = append(episodes, rehearsal.quiet(symbol, symbolObservations)...)
		discovery := hindsight.DiscoverEpisodes(
			symbol, symbolObservations, rehearsal.policy,
		)

		for _, episode := range discovery.Episodes {
			if (episode.Kind == hindsight.EpisodeUpwardExcursion ||
				episode.Kind == hindsight.EpisodeDownwardExcursion) && episode.Confirmed {
				episodes = append(episodes, episode)
			}
		}
	}

	sort.SliceStable(episodes, func(left, right int) bool {
		if episodes[left].FromSequence != episodes[right].FromSequence {
			return episodes[left].FromSequence < episodes[right].FromSequence
		}

		return episodes[left].Kind < episodes[right].Kind
	})

	return episodes
}

/*
slice returns a complete mini tape with lead-in and post-extremum observations.
Its extent comes from the captured leg duration. The observation grid is rebuilt
from the original captured signal measurements.
*/
func (rehearsal *Rehearsal) slice(
	episode hindsight.Episode,
) (
	[]hindsight.Observation,
	hindsight.Observation,
	hindsight.Observation,
	bool,
) {
	observations := rehearsal.observations[episode.Symbol]

	if len(observations) == 0 {
		return nil, hindsight.Observation{}, hindsight.Observation{}, false
	}

	anchorIndex := indexOfObservation(
		observations, episode, hindsight.ReferenceAnchor,
	)

	endpointRole := hindsight.ReferencePeak

	if episode.Kind == hindsight.EpisodeDownwardExcursion {
		endpointRole = hindsight.ReferenceTrough
	}

	endpointIndex := indexOfObservation(observations, episode, endpointRole)

	if anchorIndex < 0 || endpointIndex < anchorIndex {
		return nil, hindsight.Observation{}, hindsight.Observation{}, false
	}

	// The completed leg's own duration supplies the lead-in and tail. Keep
	// the full intervening tape, including quiet periods and failed rebounds.
	span := observations[endpointIndex].ReceivedAt.Sub(observations[anchorIndex].ReceivedAt)

	if span <= 0 {
		return nil, hindsight.Observation{}, hindsight.Observation{}, false
	}
	from, through := anchorIndex, endpointIndex
	start := observations[anchorIndex].ReceivedAt.Add(-span)
	end := observations[endpointIndex].ReceivedAt.Add(span)

	for from > 0 && observations[from].ReceivedAt.After(start) {
		from--
	}

	for through+1 < len(observations) && observations[through].ReceivedAt.Before(end) {
		through++
	}

	if from == anchorIndex || through == endpointIndex {
		return nil, hindsight.Observation{}, hindsight.Observation{}, false
	}
	return observations[from : through+1], observations[anchorIndex], observations[endpointIndex], true
}

func indexOfObservation(
	observations []hindsight.Observation,
	episode hindsight.Episode,
	role hindsight.ReferenceRole,
) int {
	reference, ok := episode.Reference(role)

	if !ok {
		return -1
	}

	for index, observation := range observations {
		if observation.Capture == reference.Capture &&
			observation.Ordinal == reference.Ordinal {
			return index
		}
	}

	return -1
}

func (rehearsal *Rehearsal) read(ctx context.Context) error {
	observations, through, err := hindsight.ReadObservations(
		ctx, rehearsal.catalog, rehearsal.run, rehearsal.lastSequence,
	)

	if err != nil {
		return errnie.Error(err)
	}

	// The catalog owns capture ordering; decoding preserves each frame's
	// ordinal order. Appending a committed suffix needs no history sort/copy.
	for _, observation := range observations {
		if observation.Symbol == "" || observation.Domain != "spot" {
			continue
		}
		rehearsal.observations[observation.Symbol] = append(
			rehearsal.observations[observation.Symbol], observation,
		)
	}
	rehearsal.lastSequence = through
	return rehearsal.readInputs(ctx)
}

/*
balance takes an equally sized random subset of each available class. No episode
is repeated within a worker's pass. Missing classes stay absent; counts expose
that limitation. Reversal descriptors are excluded to avoid counting the same
legs again under a second label.
*/
func (rehearsal *Rehearsal) balance(episodes []hindsight.Episode) ([]hindsight.Episode, error) {
	var classes [5][]hindsight.Episode
	ungraded := uint64(0)

	for _, episode := range episodes {
		observations, anchor, endpoint, ok := rehearsal.slice(episode)
		if !ok {
			ungraded++
			continue
		}
		anchorIndex, endpointIndex := -1, -1

		for index, observation := range observations {
			if observation.Capture == anchor.Capture && observation.Ordinal == anchor.Ordinal {
				anchorIndex = index
			}

			if observation.Capture == endpoint.Capture && observation.Ordinal == endpoint.Ordinal {
				endpointIndex = index
			}
		}
		entryQuote, exitQuote := observations[anchorIndex], observations[endpointIndex]
		executable := entryQuote.HasAsk && exitQuote.HasBid && entryQuote.AskQty > 0 && exitQuote.BidQty >= entryQuote.AskQty
		value := 0.0

		if executable {
			if rehearsal.price.FeeIfAvailable(episode.Symbol) == nil {
				return nil, errnie.Error(errnie.Err(errnie.Validation, "rehearsal: recorded symbol requires fees", nil))
			}
			cost := rehearsal.price.WithFee(episode.Symbol, decimal.NewFromFloat64(entryQuote.Ask), broker.BUY)
			proceeds := rehearsal.price.WithFee(episode.Symbol, decimal.NewFromFloat64(exitQuote.Bid), broker.SELL)
			value = proceeds.Sub(cost).Div(cost).Float64()
		}
		class := 0

		switch {
		case episode.Kind == hindsight.EpisodeQuiet:
			class = 4
		case !executable:
			class = 3
		case episode.ObservedExcursion < 0:
			class = 2
		case value <= 0:
			class = 1
		}

		classes[class] = append(classes[class], episode)
	}

	perClass := len(episodes)

	for _, episodes := range classes {
		if len(episodes) > 0 {
			perClass = min(perClass, len(episodes))
		}
	}

	var balanced []hindsight.Episode

	for _, episodes := range classes {
		if len(episodes) > 0 {
			balanced = append(balanced, shuffledEpisodes(episodes)[:perClass]...)
		}
	}

	rehearsal.mutex.Lock()
	defer rehearsal.mutex.Unlock()
	rehearsal.progress.Episodes = uint64(len(episodes))
	rehearsal.progress.Profitable = uint64(len(classes[0]))
	rehearsal.progress.Subfriction = uint64(len(classes[1]))
	rehearsal.progress.Declining = uint64(len(classes[2]))
	rehearsal.progress.Illiquid = uint64(len(classes[3]))
	rehearsal.progress.Quiet = uint64(len(classes[4]))
	rehearsal.progress.Ungraded = ungraded
	rehearsal.progress.PerWorker = uint64(len(balanced))
	rehearsal.progress.Status = "replaying"

	if len(balanced) == 0 {
		rehearsal.progress.Status = "waiting for gradeable episodes"
	}

	return balanced, nil
}

// quiet adds observed unchanged-price spans closed by a subsequent change.
// Two endpoints are needed to measure a span; this is not a market threshold.
func (rehearsal *Rehearsal) quiet(symbol string, observations []hindsight.Observation) []hindsight.Episode {
	var episodes []hindsight.Episode
	first, last := -1, -1
	var previous float64

	for index, observation := range observations {
		value, defined := observation.Value(rehearsal.policy.Coordinate)

		if !defined {
			continue
		}

		if first >= 0 && value != previous {
			if last > first {
				anchor, endpoint := observations[first], observations[last]
				episodes = append(episodes, hindsight.Episode{Symbol: symbol, Kind: hindsight.EpisodeQuiet, Confirmed: true,
					HasObservedExcursion: true, FromSequence: anchor.Capture.Sequence, ToSequence: endpoint.Capture.Sequence,
					FromAt: anchor.At(), ToAt: endpoint.At(), References: []hindsight.ReferencePoint{
						{Role: hindsight.ReferenceAnchor, Capture: anchor.Capture, Ordinal: anchor.Ordinal},
						{Role: hindsight.ReferencePeak, Capture: endpoint.Capture, Ordinal: endpoint.Ordinal},
					}})
			}
			first = -1
		}

		if first < 0 {
			first = index
		}
		last, previous = index, value
	}
	return episodes
}

// prepare joins numerical witnesses onto the existing captured observations.
// The slice itself is the ring value; no fragment or practice wrapper exists.
func (rehearsal *Rehearsal) prepare(episode hindsight.Episode) ([]hindsight.Observation, error) {
	observations, _, _, complete := rehearsal.slice(episode)

	if !complete {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "rehearsal: tape requires lead-in and tail", nil))
	}
	captured := append([]hindsight.Observation(nil), observations...)
	for index := range captured {
		reference := hindsight.EnvelopeRef{Origin: captured[index].Capture, Ordinal: captured[index].Ordinal}
		captured[index].Measurements = rehearsal.inputs[reference].Measurements
	}
	return captured, nil
}
