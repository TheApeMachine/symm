package strategy

import (
	"context"
	"math/rand/v2"
	"sort"
	"sync"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning/associative"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
	"github.com/theapemachine/symm/nomagique/learning/associative/model"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
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
	Columns   [][2]string
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
	workers      int
	models       []*model.Model[string, Action]
	columns      [][][2]string
	catalog      *tables.Catalog
	run          hindsight.RunID
	policy       hindsight.DiscoveryPolicy
	price        *broker.Price
	population   *associative.Population[Action]
	experiences  chan Experience
	observations map[string][]hindsight.Observation
	progress     wire.LearningRehearsalT
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
	population *associative.Population[Action],
) *Rehearsal {
	if err := errnie.Require(map[string]any{
		"workers":    workers,
		"population": population,
		"price":      price,
		"catalog":    catalog,
		"policy":     policy,
		"run":        run,
	}); err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation, "learning: rehearsal", err,
		))

		return nil
	}

	models := make([]*model.Model[string, Action], workers)

	for index := range models {
		models[index] = model.New[string, Action]()
		models[index].Ordered = true
	}

	return &Rehearsal{
		models:     models,
		columns:    make([][][2]string, workers),
		workers:    workers,
		catalog:    catalog,
		run:        run,
		policy:     policy,
		price:      price,
		population: population,
		progress:   wire.LearningRehearsalT{Workers: int32(workers), Status: "waiting for archive"},
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
	rehearsal.mutex.Lock()
	rehearsal.progress.Status = "reading archive"
	rehearsal.mutex.Unlock()

	observations, err := hindsight.ReadObservations(
		ctx, rehearsal.catalog, rehearsal.run,
	)

	if err != nil {
		rehearsal.fail(err)
		return rehearsal.Error()
	}

	if len(observations) == 0 {
		rehearsal.mutex.Lock()
		rehearsal.progress.Status = "waiting for archive"
		rehearsal.mutex.Unlock()
		return nil
	}

	rehearsal.observations = groupObservations(observations)
	episodes := rehearsal.episodes()
	pool := rehearsal.balance(episodes)

	if len(pool) == 0 {
		return nil
	}

	// One in-flight completed fact per worker; backpressure bounds replay memory.
	rehearsal.experiences = make(chan Experience, rehearsal.workers)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Workers retain only their own historical evidence and local dictionary.
	// Copying the live trie per worker stalls live processing and multiplies
	// its memory footprint without adding historical observations.
	var drain sync.WaitGroup
	drain.Add(1)

	go func() {
		defer drain.Done()

		for experience := range rehearsal.experiences {
			if err := rehearsal.population.Learn(
				experience.Symbol,
				experience.Columns,
				experience.Context,
				experience.Action,
				experience.Outcome,
				experience.Authority,
			); err != nil {
				rehearsal.fail(err)
				cancel()
				return
			}

			rehearsal.mutex.Lock()
			rehearsal.progress.Trained++
			rehearsal.mutex.Unlock()
		}
	}()

	group, workerContext := errgroup.WithContext(ctx)

	for index, workerModel := range rehearsal.models {
		group.Go(func() error {
			return rehearsal.replayWorker(workerContext, pool, workerModel, &rehearsal.columns[index])
		})
	}

	err = group.Wait()
	close(rehearsal.experiences)
	drain.Wait()

	if err != nil {
		rehearsal.fail(err)
	}

	rehearsal.mutex.Lock()
	rehearsal.progress.Passes++
	rehearsal.progress.Status = "pass complete"

	if ctx.Err() != nil {
		rehearsal.progress.Status = "stopped"
	}
	rehearsal.mutex.Unlock()

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
	columns *[][2]string,
) error {
	order := shuffledEpisodes(episodes)

	for _, episode := range order {
		if ctx.Err() != nil {
			return nil
		}

		if err := rehearsal.replayEpisode(ctx, learned, episode, columns); err != nil {
			return errnie.Error(err)
		}
	}

	return nil
}

func shuffledEpisodes(episodes []hindsight.Episode) []hindsight.Episode {
	shuffled := make([]hindsight.Episode, len(episodes))
	copy(shuffled, episodes)
	rand.Shuffle(len(shuffled), func(left, right int) {
		shuffled[left], shuffled[right] = shuffled[right], shuffled[left]
	})
	return shuffled
}

// replayEpisode selects from prefix observations only. The endpoint quote is
// used solely to grade a completed entry/wait exercise, never as a selection input.
func (rehearsal *Rehearsal) replayEpisode(
	ctx context.Context,
	learned *model.Model[string, Action],
	episode hindsight.Episode,
	columns *[][2]string,
) error {
	observations, anchor, endpoint, ok := rehearsal.slice(episode)

	if !ok || ctx.Err() != nil {
		return nil
	}

	space := grid.NewSpace()
	for _, identity := range *columns {
		space.Column(identity[0], identity[1])
	}
	defer func() { *columns = space.Columns }()
	development := &Development{Symbol: episode.Symbol}

	for _, observation := range observations {
		if ctx.Err() != nil {
			return nil
		}

		measurement := measurementFromObservation(observation)
		// The available causal lead-in is the observation horizon, not the
		// selector's retrospective outcome duration.
		measurement.From = observations[0].At()
		history := development.Context(observation.At(), []*data.Measurement[float64]{measurement})

		if err := space.Step([]*data.Measurement[float64]{measurement}); err != nil {
			return errnie.Error(err)
		}

		if err := development.Advance(observation.At(), space); err != nil {
			return errnie.Error(err)
		}

		if observation.Capture != anchor.Capture || observation.Ordinal != anchor.Ordinal {
			continue
		}

		conditions := append([]uint64{FlatPositionContext}, history...)
		conditions = append(conditions, regionConditions(development.Regions)...)
		action, _, err := learned.Select(
			episode.Symbol, conditions, []Action{{Kind: "enter"}, {Kind: "wait"}}, true,
		)

		if err != nil {
			return errnie.Error(err)
		}

		value, defined := rehearsal.grade(episode, action, anchor, endpoint)

		if !defined {
			return errnie.Error(errnie.Err(errnie.Validation, "rehearsal: graded pool lost its quote or fee", nil))
		}

		strength, authority := 0.0, 0.0

		for _, region := range development.Regions {
			strength += region.Strength
			authority += region.Strength * region.Authority
		}

		if strength == 0 {
			rehearsal.mutex.Lock()
			rehearsal.progress.Unsupported++
			rehearsal.mutex.Unlock()
			return nil
		}

		experience := Experience{Symbol: episode.Symbol, Columns: space.Columns, Context: conditions,
			Action: action, Outcome: value, Authority: authority / strength}

		if err := learned.Observe(experience.Symbol, conditions, action, value, experience.Authority); err != nil {
			return errnie.Error(err)
		}

		select {
		case <-ctx.Done():
			return nil
		case rehearsal.experiences <- experience:
			rehearsal.mutex.Lock()
			rehearsal.progress.Decisions++
			rehearsal.progress.LastSymbol = episode.Symbol
			rehearsal.progress.LastAction = action.Kind
			rehearsal.progress.LastReturn = value
			rehearsal.mutex.Unlock()
		}

		return nil
	}

	return errnie.Error(errnie.Err(errnie.Validation, "rehearsal: decision anchor missing from prefix", nil))
}

func (rehearsal *Rehearsal) episodes() []hindsight.Episode {
	episodes := make([]hindsight.Episode, 0, len(rehearsal.observations))

	for symbol, symbolObservations := range rehearsal.observations {
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
slice returns the warmup plus decision observation span for one episode. The
lead-in begins at the first captured observation for this symbol. This rebuilds
the observation grid causally; it does not reproduce the live signal pipeline.
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

	return observations[:anchorIndex+1], observations[anchorIndex], observations[endpointIndex], true
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

/*
grade reports a top-of-book exercise return using captured entry ask and endpoint
bid with the current venue fee schedule. It is not a fill simulation: depth,
slippage and historical fee tiers are not present in Observation. Waiting has
zero return when no positive net opportunity was missed.
*/
func (rehearsal *Rehearsal) grade(
	episode hindsight.Episode, action Action,
	anchor, endpoint hindsight.Observation,
) (float64, bool) {
	fee := rehearsal.price.FeeIfAvailable(episode.Symbol)

	if !episode.HasObservedExcursion || fee == nil || fee.Fee == nil ||
		!anchor.HasAsk || !anchor.HasBid || !endpoint.HasBid ||
		anchor.Bid <= 0 || anchor.Ask < anchor.Bid || endpoint.Bid <= 0 {
		return 0, false
	}

	entry := rehearsal.price.WithFee(episode.Symbol, decimal.NewFromFloat64(anchor.Ask), broker.BUY)
	exit := rehearsal.price.WithFee(episode.Symbol, decimal.NewFromFloat64(endpoint.Bid), broker.SELL)
	value := exit.Sub(entry).Div(entry).Float64()

	if action.Kind == "enter" {
		return value, true
	}

	if action.Kind == "wait" {
		return -max(0, value), true
	}

	return 0, false
}

/*
balance takes an equally sized random subset of each available class. No episode
is repeated within a worker's pass. Missing classes stay absent; counts expose
that limitation. Reversal descriptors are excluded to avoid counting the same
legs again under a second label.
*/
func (rehearsal *Rehearsal) balance(episodes []hindsight.Episode) []hindsight.Episode {
	var classes [3][]hindsight.Episode
	ungraded := uint64(0)

	for _, episode := range episodes {
		_, anchor, endpoint, ok := rehearsal.slice(episode)
		value, defined := rehearsal.grade(episode, Action{Kind: "enter"}, anchor, endpoint)

		if !ok || !defined {
			ungraded++
			continue
		}

		class := 0

		if episode.ObservedExcursion < 0 {
			class = 2
		}

		if episode.ObservedExcursion >= 0 && value <= 0 {
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
	rehearsal.progress.Ungraded = ungraded
	rehearsal.progress.PerWorker = uint64(len(balanced))
	rehearsal.progress.Status = "replaying"

	if len(balanced) == 0 {
		rehearsal.progress.Status = "waiting for gradeable episodes"
	}

	return balanced
}
