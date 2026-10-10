package strategy

import (
	"context"
	"fmt"
	"iter"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/ui"
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
	price    *broker.Price
	detector *Detector
	desk     *broker.Desk
	catalog  *tables.Catalog
	storeTee runtime.Tee
	grid     *store.Grid
	keys     []string
	paths    *sync.Map
	blobs    blobReader
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
	actionNoop  = "noop.json"
)

/*
fragmentTicks is the tick range Train reads for a fragment from low to its
endpoint: low through the tick before the endpoint.
*/
func fragmentTicks(low, endpoint int64) (int64, int64) {
	return low, endpoint - 1
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
		System:   system,
		price:    price,
		detector: NewDetector(ctx, storeTee, price),
		desk:     desk,
		catalog:  catalog,
		storeTee: storeTee,
		grid:     store.NewGrid(),
		paths:    &sync.Map{},
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
	errnie.Info(fmt.Sprintf("[training] matching against %d stored token paths", len(keys)))

	training.Transition(runtime.INIT)
	return training
}

/*
CognitionTree exports the stored S3 token prefixes for the learning dashboard.
*/
func (training *Training) CognitionTree() ui.CognitionTreeExport {
	return ui.CognitionTreeExport{
		Keys: training.keys,
	}
}

/*
Step receives the live market Measurement, turns it into the symbol's region
token, and advances that symbol's live token path.
*/
func (training *Training) Step(prior *data.Measurement) *data.Measurement {
	regions := training.grid.RegionScores(prior)
	next := prior.Next(training.Name(), regionMetrics(regions))

	if regions.Winner == store.NoEvidence {
		return next
	}

	action, reason, path, depth := training.advance(prior.Label, string(regions.Token()))

	if path != "" {
		next.SetMeta("token_path", path)
		next.SetMeta("token_depth", strconv.Itoa(depth))
	}

	if action != "" {
		next.SetMeta("decision_action", action)
		next.SetMeta("decision_reason", fmt.Sprintf("path=%s depth=%d %s", path, depth, reason))
		next.SetMeta("decision_confidence", strconv.Itoa(depth))
	}

	return next
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
paths. If all matches agree on one action and the path is at least the minimum
confidence long, that action is taken.
*/
func (training *Training) advance(symbol, token string) (string, string, string, int) {
	found, ok := training.paths.LoadOrStore(symbol, []string{token})
	tokens := found.([]string)

	if ok {
		if len(tokens) > 0 && tokens[len(tokens)-1] == token {
			return "", "", strings.Join(tokens, "/"), len(tokens)
		}

		tokens = append(tokens, token)
	}

	var action string
	prediction := true

	if match := training.match(tokens); len(match) > 0 {
		for _, prefix := range match {
			segment := prefix[strings.LastIndex(prefix, "/")+1:]

			if action != "" && segment != action {
				prediction = false
				break
			}

			action = segment
		}

		if prediction && action != actionNoop {
			actionName := strings.TrimSuffix(action, ".json")
			training.paths.Store(symbol, []string{})
			
			return actionName, training.act(
				symbol, actionName,
			), strings.Join(tokens, "/"), len(tokens)
		}

		training.paths.Store(symbol, tokens)
		return "accumulate", "", strings.Join(tokens, "/"), len(tokens)
	}

	training.paths.Store(symbol, []string{})
	return "", "", "", 0
}

/*
act takes the matched action through the Desk when the position state admits
it, and reports what happened.

Entries are opened at 20% of cash.
Exits sell everything currently open.
*/
func (training *Training) act(symbol, action string) string {
	hasPosition := training.desk.Has(symbol)

	switch action {
	case "enter":
		if hasPosition {
			return "ignored: position already open"
		}

		qty, err := training.price.Quantity(
			symbol, training.desk.Cash("USD"),
		)

		if err != nil {
			training.Error(errnie.Err(
				errnie.NotAcceptable,
				"[training] unable to calculate quantity for "+symbol,
				err,
			))

			return ""
		}

		training.desk.Enter(symbol, qty)
		return "submitted"
	case "exit":
		if !hasPosition {
			return "ignored: no filled position to exit"
		}

		training.desk.Exit(symbol)
		return "learned_exit submitted"
	case "noop":
		return "noop: matched unpromising precursor"
	}

	return "ignored: unknown action " + action
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
