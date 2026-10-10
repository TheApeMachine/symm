package strategy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
	symm "github.com/theapemachine/symm/system"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"golang.org/x/sync/errgroup"
)

/*
Training writes the token paths of stored excursions as S3 keys (Train) and
paper trades the live market by matching each symbol's live token path against
that key space (Step). The key space is the trie: a prefix listing returns every
stored path that continues the live one. Once all of them end in the same
action and the matched path is at least the configured minimum confidence long
(one per token), that action is taken through the Desk.
*/
type Training struct {
	*runtime.System
	detector         *Detector
	desk             *broker.Desk
	catalog          *tables.Catalog
	storeTee         runtime.Tee
	grid             *store.Grid
	keys             []string
	confidence       int
	paths            map[string]*livePath
	mu               sync.Mutex
	decisions        map[string]*wire.DecisionT
	decisionsVersion atomic.Uint64
	blobs            blobReader
	statsMu          sync.Mutex
	stats            map[string]*pathStats
	tree             treeCache
	fragments        fragmentLog
}

/*
blobReader reads a token path's blob; the catalog in production.
*/
type blobReader interface {
	GetBlob(ctx context.Context, key string) ([]byte, error)
}

/*
pathStats is what Train stores in a token path's blob: the gain c/b-1 and the
B->C duration of the excursion the path was cut from. Blobs written before
these statistics existed hold "{}" and carry none.
*/
type pathStats struct {
	Gain        *float64 `json:"gain,omitempty"`
	HoldSeconds *float64 `json:"hold_seconds,omitempty"`
}

/*
Stored path actions: the final segment of every key.
*/
const (
	actionEnter = "enter.json"
	actionExit  = "exit.json"
)

/*
maxKeyBytes is the S3 object key limit: 1024 bytes of UTF-8. A key is its
tokens and its action joined by "/".
*/
const maxKeyBytes = 1024

/*
fitFrom is the index of the oldest token kept so that the tokens from it on,
joined by "/" and followed by "/" and an action of actionBytes bytes, fit one
object key. Train stores tokens[fitFrom:]; Step keeps its live path to what
fits beside the shortest action, the longest prefix any stored key can have.
*/
func fitFrom[T ~string | ~[]byte](tokens []T, actionBytes int) int {
	size := actionBytes

	for idx := len(tokens) - 1; idx >= 0; idx-- {
		size += len(tokens[idx]) + 1

		if size > maxKeyBytes {
			return idx + 1
		}
	}

	return 0
}

/*
fragmentTicks is the tick range Train reads for a fragment from low to its
endpoint: low through the tick before the endpoint.
*/
func fragmentTicks(low, endpoint int64) (int64, int64) {
	return low, endpoint - 1
}

/*
livePath is one symbol's token path since its last reset. last is the most
recent observed token, so a run of identical tokens advances the path once,
exactly as Train collapses them.
*/
type livePath struct {
	last   string
	tokens []string
	since  time.Time
}

func NewTraining(
	ctx context.Context,
	price *broker.Price,
	desk *broker.Desk,
	catalog *tables.Catalog,
	storeTee runtime.Tee,
) *Training {
	system := runtime.NewSystem(ctx, "training", price)

	training := &Training{
		System:    system,
		detector:  NewDetector(ctx, storeTee, price),
		desk:      desk,
		catalog:   catalog,
		storeTee:  storeTee,
		grid:      store.NewGrid(),
		paths:     make(map[string]*livePath),
		decisions: make(map[string]*wire.DecisionT),
		stats:     make(map[string]*pathStats),
	}

	if catalog != nil {
		training.blobs = catalog
	}

	// The detector labels every excursion against friction. Without it the
	// training system halts here instead of learning from mislabeled tape.
	if err := training.detector.Error(); err != nil {
		training.Error(errnie.Err(
			errnie.Internal, "[training] detector is not usable", err,
		))

		return training
	}

	if err := errnie.Require(map[string]any{
		"desk":    desk,
		"catalog": catalog,
	}); err != nil {
		training.Error(errnie.Err(
			errnie.Validation, "[training] cannot paper trade without desk and catalog", err,
		))

		return training
	}

	// Step matches against the key space as it stands at startup. Keys that
	// Train writes during this run become visible on the next start.
	keys, err := sortedKeys(catalog.ListBlobs(ctx, ""))

	if err != nil {
		training.Error(errnie.Err(
			errnie.IO, "[training] unable to list the stored token paths", err,
		))

		return training
	}

	training.keys = keys
	training.confidence = symm.Cfg.Learning.MinimumPathConfidence

	if training.confidence <= 0 {
		training.Error(errnie.Err(
			errnie.Validation, "[training] learning.minimum_path_confidence must be a positive token count", nil,
		))

		return training
	}

	errnie.Info(fmt.Sprintf("[training] matching against %d stored token paths", len(keys)))

	training.Train()
	training.Transition(runtime.INIT)
	return training
}

/*
Step receives the live market Measurement, turns it into the symbol's region
token, and advances that symbol's live token path.
*/
func (training *Training) Step(prior *data.Measurement) *data.Measurement {
	regions := training.grid.RegionScores(prior)
	training.advance(prior.Label, string(regions.Token()), prior.At)

	return prior.Next(training.Name(), regionMetrics(regions))
}

/*
regionMetrics reports the frame's region evidence for the impulse map:
each evidenced region's brightness and metric count, the winning region (the
token Step advanced on), and the pinned metrics left out as undefined and the
unpinned ones. A region without evidence is absent, not zero.
*/
func regionMetrics(regions store.Regions) map[string]float64 {
	metrics := map[string]float64{
		"region_winner":    float64(regions.Winner),
		"region_runner_up": float64(regions.RunnerUp),
		"region_undefined": float64(regions.Undefined),
		"region_unpinned":  float64(regions.Unpinned),
	}

	for region := 1; region < len(regions.Counts); region++ {
		if regions.Counts[region] == 0 {
			continue
		}

		metrics[fmt.Sprintf("region_brightness:R%02d", region)] = regions.Brightness[region]
		metrics[fmt.Sprintf("region_members:R%02d", region)] = float64(regions.Counts[region])
	}

	return metrics
}

/*
advance extends the symbol's path with token and matches it against the stored
paths. No match restarts the path from token. Matches that all end in one action
on a path of at least the minimum confidence take that action and reset the
path. Matches that disagree, or agree on a shorter path, keep accumulating.
*/
func (training *Training) advance(symbol, token string, at time.Time) {
	live, ok := training.paths[symbol]

	if !ok {
		live = &livePath{}
		training.paths[symbol] = live
	}

	if live.last == token {
		return
	}

	live.last = token

	if len(live.tokens) == 0 {
		live.since = at
	}

	live.tokens = append(live.tokens, token)
	// A path longer than any stored key can be a prefix of matches nothing;
	// it keeps its most recent tokens, the same rule Train stores by.
	live.tokens = live.tokens[fitFrom(live.tokens, min(len(actionEnter), len(actionExit))):]
	candidates := training.match(live.tokens)

	if len(candidates) == 0 && len(live.tokens) > 1 {
		live.tokens = []string{token}
		live.since = at
		candidates = training.match(live.tokens)
	}

	path := strings.Join(live.tokens, "/")

	if len(candidates) == 0 {
		live.tokens = nil
		training.decide(symbol, "reset", path, candidates, at, "no stored path starts with "+path)
		return
	}

	actions := keyActions(candidates)

	if len(actions) > 1 {
		training.decide(symbol, "accumulate", path, candidates, at, "stored paths disagree on the action")
		return
	}

	if len(live.tokens) < training.confidence {
		training.decide(symbol, "accumulate", path, candidates, at, fmt.Sprintf(
			"stored paths agree on %s below minimum confidence %d", actions[0].Name, training.confidence,
		))

		return
	}

	match := training.matched(actions, path, len(live.tokens), candidates, at, at.Sub(live.since))
	live.tokens = nil
	training.decide(symbol, actions[0].Name, path, candidates, at, training.act(
		symbol, actions[0].Name, path, candidates, at, at.Sub(live.since), match,
	))
}

/*
act takes the matched action through the Desk when the position state admits
it, and reports what happened.

The learned exit is the primary exit and always wins: it sells everything
still open, ends an entry still being worked, and on a position the capacity
monitor is already fully exiting it is recorded beside that exit so both
triggers show. The monitor (in the Desk) only ever reduces exposure.

An entry is sized from the matched paths' statistics, which live in their
blobs, so it is sized off the market path and its outcome replaces this
decision's reason when it is known.
*/
func (training *Training) act(
	symbol, action, path string, candidates []string, at time.Time, span time.Duration, match broker.Match,
) string {
	state := training.desk.State(symbol)

	switch action {
	case "enter":
		if state != broker.FLAT {
			return "ignored: position is not flat"
		}

		go func() {
			edge := training.edge(candidates)
			edge.Match = &match
			result := outcome(training.desk.Enter(symbol, edge))

			training.decide(symbol, action, path, candidates, at, fmt.Sprintf(
				"%s (sized from %d matched gains, %d durations, path span %s)",
				result, len(edge.Gains), len(edge.Holds), span,
			))
		}()

		return "sizing entry"
	case "exit":
		switch state {
		case broker.HOLDING:
			training.desk.Matched(symbol, match)
			return "learned_exit " + outcome(training.desk.Exit(symbol))
		case broker.EXITING:
			training.desk.Matched(symbol, match)
			return "learned_exit recorded beside the exit already in progress: " + outcome(training.desk.Exit(symbol))
		}

		return "ignored: no filled position to exit"
	}

	errnie.Error(errnie.Err(
		errnie.UnprocessableContent, "[training] stored path ends in unknown action "+action, nil,
	))

	return "ignored: unknown action " + action
}

/*
matched is the trie match an action is taken on, for the position's
lifecycle: the confidence is the path length in tokens, the threshold the
configured minimum.
*/
func (training *Training) matched(
	actions []*wire.NamedNumberT, path string, tokens int, candidates []string, at time.Time, span time.Duration,
) broker.Match {
	counts := make(map[string]int, len(actions))

	for _, action := range actions {
		counts[action.Name] = int(action.Value)
	}

	name := ""

	if len(actions) > 0 {
		name = actions[0].Name
	}

	return broker.Match{
		At:         at,
		Action:     name,
		Path:       path,
		Tokens:     tokens,
		Candidates: len(candidates),
		Actions:    counts,
		Confidence: tokens,
		Threshold:  training.confidence,
		Span:       span,
	}
}

func outcome(err error) string {
	if err != nil {
		errnie.Error(err)
		return "desk refused: " + err.Error()
	}

	return "submitted"
}

/*
edge gathers the statistics of the matched paths from their blobs, cached per
key. A blob without statistics, or one that cannot be read, contributes none;
read failures are logged.
*/
func (training *Training) edge(candidates []string) broker.Edge {
	edge := broker.Edge{}

	if training.blobs == nil {
		return edge
	}

	ctx := context.Background()

	if training.System != nil {
		ctx = training.Context()
	}

	for _, key := range candidates {
		training.statsMu.Lock()
		stats, ok := training.stats[key]
		training.statsMu.Unlock()

		if !ok {
			stats = &pathStats{}
			body, err := training.blobs.GetBlob(ctx, key)

			if err == nil {
				err = json.Unmarshal(body, stats)
			}

			if err != nil {
				errnie.Warn("[training] path statistics unreadable for " + key + ": " + err.Error())
				continue
			}

			training.statsMu.Lock()
			training.stats[key] = stats
			training.statsMu.Unlock()
		}

		if stats.Gain != nil {
			edge.Gains = append(edge.Gains, *stats.Gain)
		}

		if stats.HoldSeconds != nil {
			edge.Holds = append(edge.Holds, time.Duration(*stats.HoldSeconds*float64(time.Second)))
		}
	}

	return edge
}

/*
excursionStats is the blob Train stores under both token paths of one
excursion.
*/
func excursionStats(excursion *data.Measurement) []byte {
	stats := pathStats{}

	if b, c, err := tables.DetectionPrices(excursion); err == nil && b.Sign() > 0 {
		gain := c.Float64()/b.Float64() - 1
		stats.Gain = &gain
	}

	if !excursion.At.IsZero() && !excursion.From.IsZero() && excursion.At.After(excursion.From) {
		hold := excursion.At.Sub(excursion.From).Seconds()
		stats.HoldSeconds = &hold
	}

	body, err := json.Marshal(stats)

	if err != nil {
		return []byte("{}")
	}

	return body
}

/*
match returns the stored keys under the path, with exactly the semantics of an
S3 ListObjectsV2 prefix listing over the startup snapshot: keys are sorted
bytewise, so every key that starts with the prefix is one contiguous run.
*/
func (training *Training) match(tokens []string) []string {
	prefix := strings.Join(tokens, "/") + "/"
	start, _ := slices.BinarySearch(training.keys, prefix)
	end := start

	for end < len(training.keys) && strings.HasPrefix(training.keys[end], prefix) {
		end++
	}

	return training.keys[start:end]
}

/*
keyActions counts the candidate keys per final action, in first-seen order.
*/
func keyActions(keys []string) []*wire.NamedNumberT {
	actions := make([]*wire.NamedNumberT, 0)

	for _, key := range keys {
		action := strings.TrimSuffix(key[strings.LastIndex(key, "/")+1:], ".json")
		index := slices.IndexFunc(actions, func(named *wire.NamedNumberT) bool {
			return named.Name == action
		})

		if index < 0 {
			actions = append(actions, &wire.NamedNumberT{Name: action})
			index = len(actions) - 1
		}

		actions[index].Value++
	}

	return actions
}

/*
sortedKeys drains a key listing into bytewise order. A listing error is
returned, never treated as an empty key space.
*/
func sortedKeys(listing iter.Seq2[string, error]) ([]string, error) {
	keys := make([]string, 0)

	for key, err := range listing {
		if err != nil {
			return nil, errnie.Error(err)
		}

		keys = append(keys, key)
	}

	slices.Sort(keys)
	return keys, nil
}

/*
decide publishes the symbol's latest matching decision for the UI.
*/
func (training *Training) decide(
	symbol, action, path string, candidates []string, at time.Time, reason string,
) {
	decision := &wire.DecisionT{
		Id:           uuid.NewString(),
		Action:       action,
		Symbol:       symbol,
		At:           at.UnixNano(),
		Alternatives: keyActions(candidates),
		Confidence:   float64(strings.Count(path, "/") + 1),
		Cause:        path,
		Reason:       fmt.Sprintf("path=%s candidates=%d %s", path, len(candidates), reason),
	}

	training.mu.Lock()
	training.decisions[symbol] = decision
	training.mu.Unlock()

	training.decisionsVersion.Add(1)
}

/*
DecisionsVersion reports the monotonic revision of the matching decisions.
*/
func (training *Training) DecisionsVersion() uint64 {
	return training.decisionsVersion.Load()
}

/*
DecisionsWire exports the latest matching decision per symbol for the UI.
*/
func (training *Training) DecisionsWire() *wire.StrategyFrameT {
	training.mu.Lock()
	defer training.mu.Unlock()

	decisions := make([]*wire.DecisionT, 0, len(training.decisions))

	for _, decision := range training.decisions {
		decisions = append(decisions, decision)
	}

	slices.SortFunc(decisions, func(left, right *wire.DecisionT) int {
		return strings.Compare(left.Symbol, right.Symbol)
	})

	return &wire.StrategyFrameT{Decisions: decisions}
}

/*
Train rehearses the stored excursions of past runs on the frozen grid in the background.
It selects profitable friction-clearing excursions ("up"), retrieves their signal measurements,
assembles them as peers in a training frame, and runs them through the grid.
*/
func (training *Training) Train() error {
	if training == nil || training.catalog == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[training] catalog is required",
			nil,
		))
	}

	go func() {
		trained := 0

		defer func() {
			errnie.Info(fmt.Sprintf("[training] Train finished: %d up excursions processed into token paths", trained))
		}()

		for excursion := range training.catalog.Excursions(training.Context()) {
			if excursion == nil {
				continue
			}

			if excursion.Meta("type") != excursionUp {
				training.fragments.add(excursion, nil)
				continue
			}
			trained++

			group, ctx := errgroup.WithContext(training.Context())

			group.Go(func() error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}

				tokens := [2][][]byte{}

				for idx, fragment := range [][]string{{"start_tick", "b_tick"}, {"b_tick", "c_tick"}} {
					ticks := make(map[int64][]*data.Measurement)

					// A fragment is the tape strictly before its end: the
					// precursor stops before B's own trade and the move
					// before C's. Signals carry the tick current when they
					// were produced, never earlier than their trade's, so
					// [low, end-1] holds nothing derived from the endpoint.
					low, end := fragmentTicks(
						int64(data.Pull(excursion.Read(fragment[0])).Metric.Raw),
						int64(data.Pull(excursion.Read(fragment[1])).Metric.Raw),
					)

					if end < low {
						// B is the first trade of the scanned tape: there
						// is no precursor to store.
						continue
					}

					for tape, err := range training.catalog.ExcursionTape(
						training.Context(),
						excursion.Epoch,
						excursion.Label,
						low,
						end,
					) {
						if err != nil {
							training.Error(errnie.Err(
								errnie.IO,
								"[training] tape read error",
								err,
							))

							continue
						}

						if tape != nil {
							ticks[tape.Tick] = append(ticks[tape.Tick], tape)
						}
					}

					for tick := low; tick <= end; tick++ {
						signals := ticks[tick]

						if len(signals) == 0 {
							continue
						}

						train := data.NewMeasurement(
							signals[0].Epoch,
							excursion.Label,
							"training",
							int64(data.Pull(excursion.Read("start_idx")).Metric.Raw),
							tick,
						)

						// The tick's frame spans the venue time its signals
						// cover: from the earliest window start to the latest.
						for _, signal := range signals {
							if signal.At.After(train.At) {
								train.At = signal.At
							}

							if !signal.From.IsZero() && (train.From.IsZero() || signal.From.Before(train.From)) {
								train.From = signal.From
							}
						}

						train.Peers(signals...)
						train.Write()

						token := training.grid.Observe(train)

						if len(tokens[idx]) > 0 && bytes.Equal(
							token, tokens[idx][len(tokens[idx])-1],
						) {
							continue
						}

						tokens[idx] = append(tokens[idx], token)
					}
				}

				precursor := make([]string, 0, len(tokens[0]))

				for _, token := range tokens[0][fitFrom(tokens[0], len(actionEnter)):] {
					precursor = append(precursor, string(token))
				}

				training.fragments.add(excursion, precursor)

				for idx, action := range [][]byte{[]byte(actionEnter), []byte(actionExit)} {
					// Only the most recent tokens before the fragment end
					// that fit one object key are stored, as Step keeps.
					tokens[idx] = tokens[idx][fitFrom(tokens[idx], len(action)):]

					if len(tokens[idx]) == 0 {
						continue
					}

					tokens[idx] = append(tokens[idx], action)

					training.catalog.PutBlob(
						training.Context(),
						string(bytes.Join(tokens[idx], []byte("/"))),
						excursionStats(excursion),
					)
				}

				return nil
			})

			if err := group.Wait(); err != nil {
				training.Error(errnie.Err(
					errnie.Internal,
					"[training] tape read error",
					err,
				))
			}
		}
	}()

	return nil
}
