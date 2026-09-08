package strategy

import (
	"context"
	"encoding/binary"
	"math/rand/v2"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
	"github.com/theapemachine/symm/nomagique/learning/associative/model"
	"github.com/theapemachine/symm/nomagique/learning/associative/prior"
	"golang.org/x/sync/errgroup"
)

/*
Experience is one completed historical decision, already graded net of the
episode's own friction. The issuing worker owns the context tokens and their
grid identities; the consolidated model only ever observes them through the
single drainer below, never through a racing worker.
*/
type Experience struct {
	Symbol    string
	Context   []uint64
	Action    Action
	Outcome   float64
	Authority float64
}

/*
Rehearsal owns the offline replay workers. It samples historical episodes in
random order, grades each decision at the episode's known end, and streams the
resulting experience to the consolidated model through one drainer. The workers
never touch the live grid, the live book, or a live wallet.
*/
type Rehearsal struct {
	ctx          context.Context
	workers      int
	catalog      *tables.Catalog
	run          hindsight.RunID
	policy       hindsight.DiscoveryPolicy
	price        *broker.Price
	consolidated *model.Model[string, Action]
	cognition    *cognition.Engine
	experiences  chan Experience
	observations map[string][]hindsight.Observation
	Episodes     uint64
	Decisions    uint64
	err          error
	mutex        sync.Mutex
}

/* NewRehearsal creates offline workers with the shared policy model. */
func NewRehearsal(
	workers int,
	catalog *tables.Catalog,
	run hindsight.RunID,
	policy hindsight.DiscoveryPolicy,
	price *broker.Price,
	consolidated *model.Model[string, Action],
	engine *cognition.Engine,
) *Rehearsal {
	if err := errnie.Require(map[string]any{
		"workers": workers,
		"engine":  engine,
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

	return &Rehearsal{
		workers:      workers,
		catalog:      catalog,
		run:          run,
		policy:       policy,
		price:        price,
		consolidated: consolidated,
		cognition:    engine,
		experiences:  make(chan Experience, workers*64),
	}
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
Run replays the historical record already present in the archive. A missing or
empty archive is a valid first-run state, not a failure: the live forward loop
remains the only source of experience until captures exist.
*/
func (rehearsal *Rehearsal) Run(ctx context.Context) error {
	observations, err := hindsight.ReadObservations(
		ctx, rehearsal.catalog, rehearsal.run,
	)

	if err != nil {
		rehearsal.fail(err)
		return rehearsal.Error()
	}

	if len(observations) == 0 {
		return nil
	}

	rehearsal.observations = groupObservations(observations)
	episodes := rehearsal.episodes()

	if len(episodes) == 0 {
		return nil
	}

	rehearsal.Episodes = uint64(len(episodes))
	rehearsal.experiences = make(chan Experience, rehearsal.workers*64)

	// Clone before the drainer starts writing to the consolidated model, so
	// every worker trains a stable snapshot while the shared policy advances.
	learned := make([]*model.Model[string, Action], rehearsal.workers)

	for index := range learned {
		learned[index] = rehearsal.consolidated.Clone()
	}

	var drain sync.WaitGroup
	drain.Add(1)

	go func() {
		defer drain.Done()

		for experience := range rehearsal.experiences {
			if err := rehearsal.consolidated.Observe(
				experience.Symbol,
				experience.Context,
				experience.Action,
				experience.Outcome,
				experience.Authority,
			); err != nil {
				rehearsal.fail(err)
			}
		}
	}()

	group, ctx := errgroup.WithContext(rehearsal.ctx)

	for _, workerModel := range learned {
		group.Go(func() error {
			rehearsal.replayWorker(ctx, episodes, workerModel)
			return nil
		})
	}

	if rehearsal.err = group.Wait(); rehearsal.err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[rehearsal] failed worker",
			rehearsal.err,
		))
	}

	close(rehearsal.experiences)
	drain.Wait()

	return rehearsal.Error()
}

/*
replayWorker runs one full shuffled pass over the current episode pool. The
learner's retry loop calls Run again after the persistence interval, so each
pass re-reads the archive and discovers episodes that arrived since the last
pass instead of pinning the workers to a stale snapshot.
*/
func (rehearsal *Rehearsal) replayWorker(
	ctx context.Context,
	episodes []hindsight.Episode,
	learned *model.Model[string, Action],
) {
	order := shuffledEpisodes(episodes)

	for _, episode := range order {
		if ctx.Err() != nil {
			return
		}
		rehearsal.replayEpisode(ctx, learned, episode)
	}
}

func shuffledEpisodes(episodes []hindsight.Episode) []hindsight.Episode {
	shuffled := make([]hindsight.Episode, len(episodes))
	copy(shuffled, episodes)
	rand.Shuffle(len(shuffled), func(left, right int) {
		shuffled[left], shuffled[right] = shuffled[right], shuffled[left]
	})
	return shuffled
}

func (rehearsal *Rehearsal) replayEpisode(
	ctx context.Context,
	learned *model.Model[string, Action],
	episode hindsight.Episode,
) {
	if ctx.Err() != nil {
		return
	}

	observations, anchor, _, ok := rehearsal.slice(episode)

	if !ok {
		return
	}

	space := grid.NewSpace()
	development := &Development{Symbol: episode.Symbol}

	for _, observation := range observations {
		if ctx.Err() != nil {
			return
		}

		measurement := measurementFromObservation(observation)
		at := observation.At()

		history := development.Context(
			at, []*data.Measurement[float64]{measurement},
		)

		if err := space.Step(
			[]*data.Measurement[float64]{measurement},
		); err != nil {
			rehearsal.fail(err)
			return
		}

		if err := development.Advance(at, space); err != nil {
			rehearsal.fail(err)
			return
		}

		// Decide at the episode anchor: the precursor has formed, the known
		// outcome has not begun. Everything after this point is the outcome
		// window and is never shown to the agent.
		if observation.Capture.Sequence != anchor.Capture.Sequence ||
			observation.Ordinal != anchor.Ordinal {
			continue
		}

		regions, _, err := space.Regions(episode.Symbol)

		if err != nil {
			rehearsal.fail(err)
			return
		}

		context := append(append(
			[]uint64(nil), history...,
		),
			regionConditions(regions)...,
		)

		action, selected, err := learned.Select(
			episode.Symbol,
			context,
			[]Action{{Kind: "enter"}, {Kind: "wait"}},
			true,
		)

		if err != nil {
			rehearsal.fail(err)
			return
		}

		outcome, authority := rehearsal.grade(
			episode, action, selected, anchor,
		)

		rehearsal.cognition.Observe(
			contextBytes(context), actionBytes(action),
		)

		rehearsal.experiences <- Experience{
			Symbol:    episode.Symbol,
			Context:   context,
			Action:    action,
			Outcome:   outcome,
			Authority: authority,
		}

		atomic.AddUint64(&rehearsal.Decisions, 1)
	}
}

func (rehearsal *Rehearsal) episodes() []hindsight.Episode {
	episodes := make([]hindsight.Episode, 0, len(rehearsal.observations))

	for symbol, symbolObservations := range rehearsal.observations {
		discovery := hindsight.DiscoverEpisodes(
			symbol, symbolObservations, rehearsal.policy,
		)

		for _, episode := range discovery.Episodes {
			if episode.IsPriceGeometry() && episode.Confirmed {
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
slice returns the warmup plus decision observation span for one episode. The
warmup length is the episode selector's own horizon, so baselines reach the same
maturity they would have live before the decision is taken.
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

	start := anchorIndex - rehearsal.policy.ExcursionHorizon

	if start < 0 {
		start = 0
	}

	return observations[start : endpointIndex+1], observations[anchorIndex], observations[endpointIndex], true
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

func groupObservations(
	observations []hindsight.Observation,
) map[string][]hindsight.Observation {
	grouped := make(map[string][]hindsight.Observation)

	for _, observation := range observations {
		if observation.Symbol == "" || observation.Domain != "spot" {
			continue
		}

		grouped[observation.Symbol] = append(
			grouped[observation.Symbol], observation,
		)
	}

	for symbol := range grouped {
		sort.SliceStable(grouped[symbol], func(left, right int) bool {
			if grouped[symbol][left].Capture.Sequence != grouped[symbol][right].Capture.Sequence {
				return grouped[symbol][left].Capture.Sequence < grouped[symbol][right].Capture.Sequence
			}

			return grouped[symbol][left].Ordinal < grouped[symbol][right].Ordinal
		})
	}

	return grouped
}

func regionConditions(regions []grid.Region) []uint64 {
	conditions := make([]uint64, 0, len(regions))

	for _, region := range regions {
		conditions = append(conditions, region.Condition)
	}

	return conditions
}

func (rehearsal *Rehearsal) grade(
	episode hindsight.Episode,
	action Action,
	selected prior.Reading,
	anchor hindsight.Observation,
) (float64, float64) {
	if !episode.HasObservedExcursion {
		return 0, selected.Authority
	}

	excursion := episode.ObservedExcursion
	value := -excursion

	if action.Kind == "enter" {
		value = excursion - rehearsal.friction(episode, anchor)
	}

	return value, selected.Authority
}

func (rehearsal *Rehearsal) friction(
	episode hindsight.Episode, anchor hindsight.Observation,
) float64 {
	spread, defined := anchor.SpreadFraction()

	if !defined {
		spread = 0
	}

	feeFraction := 0.0

	if rehearsal.price != nil {
		if fee := rehearsal.price.FeeIfAvailable(episode.Symbol); fee != nil {
			feeFraction = 2 * fee.Fee.Float64() / 100
		}
	}

	return feeFraction + spread
}

func contextBytes(context []uint64) []byte {
	encoded := make([]byte, len(context)*8)

	for index, token := range context {
		binary.LittleEndian.PutUint64(encoded[index*8:], token)
	}

	return encoded
}

func actionBytes(action Action) []byte {
	return []byte(action.Kind)
}
