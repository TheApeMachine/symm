package strategy

import (
	"bytes"
	"context"
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
}

/*
livePath is one symbol's token path since its last reset. last is the most
recent observed token, so a run of identical tokens advances the path once,
exactly as Train collapses them.
*/
type livePath struct {
	last   string
	tokens []string
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
	training.advance(prior.Label, string(training.grid.Observe(prior)), prior.At)

	return prior.Next(training.Name())
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
	live.tokens = append(live.tokens, token)
	candidates := training.match(live.tokens)

	if len(candidates) == 0 && len(live.tokens) > 1 {
		live.tokens = []string{token}
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

	live.tokens = nil
	training.decide(symbol, actions[0].Name, path, candidates, at, training.act(symbol, actions[0].Name))
}

/*
act takes the matched action through the Desk when the position state admits
it, and reports what happened.
*/
func (training *Training) act(symbol, action string) string {
	state := training.desk.State(symbol)

	switch action {
	case "enter":
		if state != broker.FLAT {
			return "ignored: position is not flat"
		}

		return outcome(training.desk.Enter(symbol))
	case "exit":
		if state != broker.HOLDING {
			return "ignored: no filled position to exit"
		}

		return outcome(training.desk.Exit(symbol))
	}

	errnie.Error(errnie.Err(
		errnie.UnprocessableContent, "[training] stored path ends in unknown action "+action, nil,
	))

	return "ignored: unknown action " + action
}

func outcome(err error) string {
	if err != nil {
		errnie.Error(err)
		return "desk refused: " + err.Error()
	}

	return "submitted"
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
		for excursion := range training.catalog.Excursions(training.Context()) {
			if excursion.Meta("type") != excursionUp {
				continue
			}

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

					for tape, err := range training.catalog.ExcursionTape(
						training.Context(),
						excursion.Epoch,
						excursion.Label,
						int64(data.Pull(excursion.Read(fragment[0])).Metric.Raw),
						int64(data.Pull(excursion.Read(fragment[1])).Metric.Raw)-2,
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

					for tick := int64(data.Pull(excursion.Read(
						fragment[0],
					)).Metric.Raw); tick <= int64(data.Pull(excursion.Read(
						fragment[1],
					)).Metric.Raw); tick++ {
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

				for idx, action := range [][]byte{[]byte("enter.json"), []byte("exit.json")} {
					if len(tokens[idx]) == 0 {
						continue
					}

					tokens[idx] = append(tokens[idx], action)

					training.catalog.PutBlob(
						training.Context(),
						string(bytes.Join(tokens[idx], []byte("/"))),
						[]byte("{}"),
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
