package strategy

import (
	"context"
	"fmt"
	"iter"
	"math/rand/v2"
	"sort"
	"sync"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
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
/*
tapeKey names one instrument's captured tape inside one recorded run.

Runs are kept apart deliberately. Capture sequences restart and the receive
clock jumps between them, so two runs are not one longer tape and must never
be concatenated into one: an episode discovered across that seam would be
geometry that never happened.
*/
type tapeKey struct {
	run    hindsight.RunID
	symbol string
}

type Rehearsal struct {
	workers      int
	cursors      []*replayCursor
	Workload     *runtime.Workload[*types.Envelope]
	prepared     map[string]bool
	catalog      *tables.Catalog
	run          hindsight.RunID
	budget       int
	policy       hindsight.DiscoveryPolicy
	price        *broker.Price
	learner      *Learner
	observations map[tapeKey][]hindsight.Observation
	sequences    map[hindsight.RunID]int64
	loaded       int
	inputs       map[hindsight.EnvelopeRef]hindsight.RehearsalInput
	witnessed    map[tables.EnvelopeRefRow]bool
	progress     wire.LearningRehearsalT
	err          error
	mutex        sync.Mutex
}

/*
defaultObservationBudget bounds resident captured tape when no budget is
declared. It is a memory bound on the archive, not a statement about how much
history is worth practising on.
*/
const defaultObservationBudget = 500_000

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

	budget := viper.GetInt("hindsight.rehearsal.observation_budget")

	// An undeclared budget must not silently mean "read nothing beyond the
	// live run", which is the condition this budget exists to lift.
	if budget <= 0 {
		budget = defaultObservationBudget
	}

	rehearsal := &Rehearsal{
		observations: make(map[tapeKey][]hindsight.Observation),
		sequences:    make(map[hindsight.RunID]int64),
		prepared:     make(map[string]bool), workers: workers, budget: budget,
		catalog: catalog, run: run, policy: policy, price: price, learner: learner,
		progress: wire.LearningRehearsalT{Workers: int32(workers), Budget: uint64(budget), Status: "waiting for archive"},
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

	for _, err := range rehearsal.read(ctx) {
		if err != nil {
			rehearsal.fail(err)
			return rehearsal.Error()
		}

		if err := rehearsal.distribute(); err != nil {
			rehearsal.fail(err)
			return rehearsal.Error()
		}
	}
	return rehearsal.Error()
}

// distribute makes each fully read tape available while older runs are still
// loading. Workers consume immutable fragments through their pending queues.
func (rehearsal *Rehearsal) distribute() error {
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

	var prepared []fragment
	for _, entry := range pool {
		episode := entry.found.episode
		identity := fmt.Sprint(entry.found.tape, episode.Kind, episode.References)

		if rehearsal.prepared[identity] {
			continue
		}
		tape, err := rehearsal.prepare(entry.found.tape, episode)

		if err != nil {
			return errnie.Error(err)
		}
		tape.opportunity = opportunities[entry.class]
		rehearsal.prepared[identity] = true
		prepared = append(prepared, tape)
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

/*
poolShare is how many tapes each present class contributes to a pass. A class
holding fewer than the share contributes everything it has.

Stillness is abundant and movement is rare, so this number decides what the
workers actually practise. Sizing it by the smallest present class lets one
scarce class discard every other class's tapes; sizing it by the largest hands
the whole curriculum to stillness. The median present class does neither: a
scarce class is taken in full rather than throwing the rest away, and an
abundant one is capped at what the ordinary classes can match.
*/
func poolShare(classes [5][]selected) int {
	present := make([]int, 0, len(classes))

	for _, episodes := range classes {
		if len(episodes) > 0 {
			present = append(present, len(episodes))
		}
	}

	if len(present) == 0 {
		return 0
	}
	sort.Ints(present)

	return present[len(present)/2]
}

// selected is one pool episode with the class it was counted under.
type selected struct {
	found discovered
	class int
}

func shuffledEpisodes(episodes []selected) []selected {
	shuffled := make([]selected, len(episodes))
	copy(shuffled, episodes)
	rand.Shuffle(len(shuffled), func(left, right int) {
		shuffled[left], shuffled[right] = shuffled[right], shuffled[left]
	})
	return shuffled
}

func (rehearsal *Rehearsal) episodes() []discovered {
	episodes := make([]discovered, 0, len(rehearsal.observations))

	for key, observations := range rehearsal.observations {
		// Inserted precursor decision coordinates carry an as-of quote. They are
		// not additional market observations for episode discovery.
		symbolObservations := make([]hindsight.Observation, 0, len(observations))
		for _, observation := range observations {
			if observation.Kind != "precursor" {
				symbolObservations = append(symbolObservations, observation)
			}
		}
		for _, episode := range rehearsal.quiet(key.symbol, symbolObservations) {
			episodes = append(episodes, discovered{tape: key, episode: episode})
		}
		discovery := hindsight.DiscoverEpisodes(
			key.symbol, symbolObservations, rehearsal.policy,
		)

		for _, episode := range discovery.Episodes {
			if (episode.Kind == hindsight.EpisodeUpwardExcursion ||
				episode.Kind == hindsight.EpisodeDownwardExcursion) && episode.Confirmed {
				episodes = append(episodes, discovered{tape: key, episode: episode})
			}
		}
	}

	sort.SliceStable(episodes, func(left, right int) bool {
		if episodes[left].episode.FromSequence != episodes[right].episode.FromSequence {
			return episodes[left].episode.FromSequence < episodes[right].episode.FromSequence
		}

		return episodes[left].episode.Kind < episodes[right].episode.Kind
	})

	return episodes
}

// discovered is one episode with the tape it was discovered on.
type discovered struct {
	tape    tapeKey
	episode hindsight.Episode
}

/*
fragment is one mini tape together with the geometry a worker is judged
against. entry is the observation the selected excursion ignited at and exit
the observation it reached its extremum at — B and C on the inspection view,
with everything before entry serving as the entry precursor and everything
between entry and exit as the exit precursor.

A tape carrying no ignition at all holds -1 in both, and on such a tape the
only correct behaviour is to keep waiting. The indices address the observation
slice directly; they are retrospective coordinates on the record and never a
statement that SYMM should have traded there.
*/
type fragment struct {
	observations []hindsight.Observation
	opportunity  string
	entry, exit  int
}

/*
opportunities names what the record says each kind of tape did, in the same
words the episode pool is counted in, so a lane and the pool legend beside it
cannot describe the same tape differently. These are descriptions of what
happened, never a judgement of the call a worker makes on them.
*/
var opportunities = [5]string{
	"rise clears costs",
	"rise eaten by costs",
	"price falls",
	"exit liquidity unavailable",
	"no price development",
}

/* ignites reports that this tape contains a moment to be recognised. */
func (tape fragment) ignites() bool {
	return tape.entry >= 0 && tape.exit > tape.entry
}

/*
slice returns a complete mini tape with lead-in and post-extremum observations.
Its extent comes from the captured leg duration. The observation grid is rebuilt
from the original captured signal measurements.
*/
func (rehearsal *Rehearsal) slice(key tapeKey, episode hindsight.Episode) (fragment, bool) {
	observations := rehearsal.observations[key]

	if len(observations) == 0 {
		return fragment{}, false
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
		return fragment{}, false
	}

	// The completed leg's own duration supplies the lead-in and tail. Keep
	// the full intervening tape, including quiet periods and failed rebounds.
	span := observations[endpointIndex].ReceivedAt.Sub(observations[anchorIndex].ReceivedAt)

	if span <= 0 {
		return fragment{}, false
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
		return fragment{}, false
	}
	tape := fragment{
		observations: observations[from : through+1],
		entry:        anchorIndex - from,
		exit:         endpointIndex - from,
	}

	// An unchanged-price span has endpoints but no ignition: its references
	// bound the span rather than name a moment inside it.
	if episode.Kind == hindsight.EpisodeQuiet {
		tape.entry, tape.exit = -1, -1
	}

	return tape, true
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

/*
read admits captured tape from the whole archive, not only the run being
recorded right now.

A confirmed excursion needs a qualifying move and then a retracement away from
its extremum, which a freshly started process has not had time to produce. The
runs already on disk are where the movement is, so practice reads them too. The
live run is always read; completed runs are taken newest first until the
observation budget is spent, so an archive larger than memory degrades to the
most recent tape rather than failing.
*/
func (rehearsal *Rehearsal) read(ctx context.Context) iter.Seq2[hindsight.RunID, error] {
	return func(yield func(hindsight.RunID, error) bool) {
		err := rehearsal.readRun(ctx, rehearsal.run)

		if !yield(rehearsal.run, err) || err != nil {
			return
		}
		runs, err := rehearsal.catalog.Runs(ctx)

		if err != nil {
			yield("", errnie.Error(err))
			return
		}
		sort.SliceStable(runs, func(left, right int) bool {
			return runs[left].StartedAt.After(runs[right].StartedAt)
		})

		for _, row := range runs {
			run := hindsight.RunID(row.ID)

			if run == rehearsal.run {
				continue
			}

			// Admit whole recorded runs; a budget boundary cannot masquerade
			// as an observed episode endpoint. Publish between those runs.
			if rehearsal.loaded >= rehearsal.budget {
				return
			}

			err := rehearsal.readRun(ctx, run)

			if !yield(run, err) || err != nil {
				return
			}
		}
	}
}

/* readRun appends one run's committed suffix and joins its precursor inputs. */
func (rehearsal *Rehearsal) readRun(ctx context.Context, run hindsight.RunID) error {
	if len(rehearsal.prepared) == 0 {
		rehearsal.mutex.Lock()
		rehearsal.progress.Status = "reading archive"
		rehearsal.mutex.Unlock()
	}
	observations, through, err := hindsight.ReadObservations(
		ctx, rehearsal.catalog, run, rehearsal.sequences[run],
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
		key := tapeKey{run: run, symbol: observation.Symbol}
		rehearsal.observations[key] = append(rehearsal.observations[key], observation)
		rehearsal.loaded++
	}
	rehearsal.sequences[run] = through
	rehearsal.mutex.Lock()
	rehearsal.progress.Runs = int32(len(rehearsal.sequences))
	rehearsal.progress.Observations = uint64(rehearsal.loaded)
	rehearsal.progress.Budget = uint64(rehearsal.budget)
	rehearsal.mutex.Unlock()
	return rehearsal.readInputs(ctx, run)
}

/*
balance takes an equally sized random subset of each available class. No episode
is repeated within a worker's pass. Missing classes stay absent; counts expose
that limitation. Reversal descriptors are excluded to avoid counting the same
legs again under a second label.
*/
func (rehearsal *Rehearsal) balance(episodes []discovered) ([]selected, error) {
	var classes [5][]selected
	ungraded := uint64(0)

	for _, found := range episodes {
		episode := found.episode
		tape, ok := rehearsal.slice(found.tape, episode)
		if !ok {
			ungraded++
			continue
		}
		anchorIndex := indexOfReference(tape, episode, hindsight.ReferenceAnchor)
		endpointRole := hindsight.ReferencePeak

		if episode.Kind == hindsight.EpisodeDownwardExcursion {
			endpointRole = hindsight.ReferenceTrough
		}
		endpointIndex := indexOfReference(tape, episode, endpointRole)

		if anchorIndex < 0 || endpointIndex < 0 {
			ungraded++
			continue
		}
		entryQuote, exitQuote := tape.observations[anchorIndex], tape.observations[endpointIndex]
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

		classes[class] = append(classes[class], selected{found: found, class: class})
	}

	perClass := poolShare(classes)
	var balanced []selected

	for _, episodes := range classes {
		if len(episodes) > 0 {
			balanced = append(balanced, shuffledEpisodes(episodes)[:min(perClass, len(episodes))]...)
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

/*
quiet adds observed unchanged-price spans closed by a subsequent change.

A span qualifies on the same terms the policy applies to a move: it must be at
least MinRegimeSpan observations long, so a coordinate that simply has not been
requoted for two ticks is not evidence of stillness. Without that bar the
midpoint's own quantisation manufactures an episode between almost every pair
of observations — which is exactly what the policy's floor exists to prevent —
and stillness then wins the pool on volume alone.
*/
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
			if last-first >= rehearsal.policy.MinRegimeSpan {
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
// The tape and its own geometry are the ring value.
func (rehearsal *Rehearsal) prepare(key tapeKey, episode hindsight.Episode) (fragment, error) {
	tape, complete := rehearsal.slice(key, episode)

	if !complete {
		return fragment{}, errnie.Error(errnie.Err(errnie.Validation, "rehearsal: tape requires lead-in and tail", nil))
	}
	captured := append([]hindsight.Observation(nil), tape.observations...)
	for index := range captured {
		reference := hindsight.EnvelopeRef{Origin: captured[index].Capture, Ordinal: captured[index].Ordinal}
		captured[index].Measurements = rehearsal.inputs[reference].Measurements
	}
	tape.observations = captured
	return tape, nil
}

// indexOfReference locates one of an episode's retrospective coordinates
// inside a prepared tape by capture identity, never by timestamp proximity.
func indexOfReference(
	tape fragment, episode hindsight.Episode, role hindsight.ReferenceRole,
) int {
	reference, ok := episode.Reference(role)

	if !ok {
		return -1
	}

	for index, observation := range tape.observations {
		if observation.Capture == reference.Capture && observation.Ordinal == reference.Ordinal {
			return index
		}
	}

	return -1
}
