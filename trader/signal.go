package trader

import (
	"bytes"
	"context"
	"flag"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/signal/causal"
	"github.com/theapemachine/symm/signal/correlation"
	"github.com/theapemachine/symm/signal/cvd"
	"github.com/theapemachine/symm/signal/depthflow"
	"github.com/theapemachine/symm/signal/exhaust"
	"github.com/theapemachine/symm/signal/fluid"
	"github.com/theapemachine/symm/signal/hawkes"
	"github.com/theapemachine/symm/signal/leadlag"
	"github.com/theapemachine/symm/signal/liquidity"
	"github.com/theapemachine/symm/signal/manifold"
	"github.com/theapemachine/symm/signal/pumpdump"
	"github.com/theapemachine/symm/signal/sentiment"
	"github.com/theapemachine/symm/signal/toxicity"
)

/*
ingestRoles are the raw tree roles the trader replays into the signals each tick.
level3 carries per-order add/delete/fill events (authenticated ws-l3) and is the
prerequisite for principled toxicity (cancel vs fill), depthflow spoof shape, and
exhaust per-level thinning — L2 book quantity deltas alone cannot tell a cancel
from a fill. A role absent from the tree simply yields no frames, so listing
level3 here is safe before live capture exists.
*/
var ingestRoles = []string{"book", "level3", "ticker", "trade", "ohlc"}

/*
Signal replays ingest frames from the tree for each spectrum signal's declared roles.
Ingest keys frames by role/scope/timestamp; Measure receives matching rows once.
*/
type Signal struct {
	ctx                 context.Context
	cancel              context.CancelFunc
	pool                *qpool.Q[any]
	tree                *dmt.Tree
	signals             map[logic.SourceType]market.Signal
	lastTimestamp       int64
	prevObservedStamp   int64
	lastTimestampByRole map[string]int64
	lastObservedByRole  map[string]int64
	lastRoleCount       map[string]int
	quoteCurrency       string
	cachedFramesByRole  map[string][]*datura.Artifact
	cachedMaxSeenByRole map[string]int64
	cachedCursorByRole  map[string]int64
}

type roleFrameResult struct {
	role      string
	frames    []*datura.Artifact
	maxSeen   int64
	scanUntil int64
}

type signalMeasureResult struct {
	origin       logic.SourceType
	measurements []*datura.Artifact
}

type batchResetter interface {
	ResetBatch()
}

/*
NewSignal constructs the signal.
*/
func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
	tree *dmt.Tree,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signals := map[logic.SourceType]market.Signal{
		logic.SourceHawkes:      hawkes.NewSignal(ctx, tree),
		logic.SourceFluid:       fluid.NewSignal(ctx, pool, tree),
		logic.SourcePumpDump:    pumpdump.NewSignal(ctx, tree),
		logic.SourceCausal:      causal.NewSignal(ctx, tree),
		logic.SourceDepthFlow:   depthflow.NewSignal(ctx, tree),
		logic.SourceLeadLag:     leadlag.NewSignal(ctx, tree),
		logic.SourceLiquidity:   liquidity.NewSignal(ctx, tree),
		logic.SourceSentiment:   sentiment.NewSignal(ctx, tree),
		logic.SourceToxicity:    toxicity.NewSignal(ctx, tree),
		logic.SourceCorrelation: correlation.NewSignal(ctx, tree),
		logic.SourceExhaustion:  exhaust.NewSignal(ctx, tree),
		logic.SourceCVD:         cvd.NewSignal(ctx, tree),
		logic.SourceManifold:    manifold.NewSignal(ctx, tree),
	}

	return &Signal{
		ctx:                 ctx,
		cancel:              cancel,
		pool:                pool,
		tree:                tree,
		signals:             signals,
		lastTimestamp:       0,
		lastTimestampByRole: make(map[string]int64),
		lastObservedByRole:  make(map[string]int64),
		lastRoleCount:       make(map[string]int),
		quoteCurrency:       strings.ToUpper(viper.GetString("market.quote_currency")),
	}
}

/*
Observe builds the cross-section peer snapshot for this tick. It runs once,
single-threaded, before Measure fans out — so every signal reads a complete,
consistent peer context instead of each racily Observing the same ticker rows
from its own goroutine. Re-observing recent rows is idempotent: the cross-section
caps history and overwrites per-symbol state.
*/
func (signal *Signal) Observe(crossSection *market.CrossSection) {
	if crossSection == nil {
		return
	}

	// Warm the frame cache on the first phase of this tick cycle.
	// This consolidates subsequent reads in Measure into simple cache lookups.
	signal.loadFrames()

	// Feed the cached ticker frames to the cross-section snapshot
	tickerFrames := signal.cachedFramesByRole["ticker"]
	for _, artifact := range tickerFrames {
		for rowIndex := 0; ; rowIndex++ {
			row, err := market.SymbolFromTicker(artifact, rowIndex)
			if err != nil {
				break
			}
			errnie.Error(crossSection.Observe(row))
		}
	}

	// Record one market-wide breadth sample per tick, after the full universe
	// has been observed — breadth is a cross-sectional reading, not per-symbol.
	crossSection.RecordBreadth(crossSection.Breadth(time.Time{}))

	// Warm the peer-snapshot cache single-threaded so the concurrent signal
	// reads in Measure are pure lookups and never mutate the cross-section.
	crossSection.WarmPeers(crossSection.MinBarsRequired())
}

/*
loadRoleFrames replays every ingest frame for one role that arrived after the
observed timestamp cursor. It uses the radix tree's lower-bound iterator against
the role/timestamp keyspace instead of manufacturing one prefix per elapsed
second. The observed cursor still filters same-second rows older than the last
processed nanosecond.
*/
func (signal *Signal) loadRoleFrames(role string, scanPrev int64, observedPrev int64) ([]*datura.Artifact, int64, int64) {
	if signal == nil || signal.tree == nil {
		panic(errnie.Err(errnie.Validation, "signal: nil tree while loading "+role, nil))
	}

	roleArtifacts := make([]*datura.Artifact, 0)
	maxSeen := observedPrev
	nowNano := time.Now().UTC().UnixNano()
	scannedThrough := scanPrev
	seen := make(map[string]struct{})
	rolePrefix := []byte(role + "/")
	startKey := rolePrefix

	if observedPrev > 0 {
		startKey = []byte(role + "/" + datura.FormatTimestamp(observedPrev) + "/")
	}

	signal.tree.WalkLowerBound(startKey, func(key, value []byte) bool {
		if !bytes.HasPrefix(key, rolePrefix) {
			return false
		}
		if len(key) <= len(rolePrefix) || key[len(rolePrefix)] < '0' || key[len(rolePrefix)] > '9' {
			return false
		}

		if len(value) == 0 {
			panic(errnie.Err(errnie.Validation, "signal: empty tree value under "+string(key), nil))
		}

		artifact := &datura.Artifact{}
		if _, err := artifact.Unpack(value); err != nil {
			panic(errnie.Err(errnie.Validation, "signal: unpack "+string(key), err))
		}

		artifactRole, err := artifact.Role()
		if err != nil {
			panic(errnie.Err(errnie.Validation, "signal: artifact role unreadable under "+string(key), err))
		}
		if artifactRole != role {
			panic(errnie.Err(
				errnie.Validation,
				"signal: lower-bound role mismatch under "+string(key)+" got "+artifactRole+" want "+role,
				nil,
			))
		}

		timestamp := artifact.Timestamp()
		if timestamp <= observedPrev {
			return true
		}
		if timestamp > nowNano {
			return false
		}

		if !signal.acceptsArtifact(artifact) {
			return true
		}

		uuid, err := artifact.Uuid()
		if err != nil {
			panic(errnie.Err(errnie.Validation, "signal: artifact uuid unreadable under "+string(key), err))
		}
		if len(uuid) == 0 {
			panic(errnie.Err(errnie.Validation, "signal: artifact missing uuid under "+string(key), nil))
		}

		uuidKey := string(uuid)
		if _, ok := seen[uuidKey]; ok {
			return true
		}
		seen[uuidKey] = struct{}{}

		if timestamp > maxSeen {
			maxSeen = timestamp
		}

		roleArtifacts = append(roleArtifacts, artifact)

		return true
	})

	if maxSeen > nowNano {
		panic(errnie.Err(errnie.Validation, "signal: observed future timestamp for "+role, nil))
	}

	if maxSeen > observedPrev {
		scannedThrough = maxSeen
	} else {
		scannedThrough = max(scannedThrough, nowNano)
	}

	sort.Slice(roleArtifacts, func(indexA, indexB int) bool {
		return roleArtifacts[indexA].Timestamp() < roleArtifacts[indexB].Timestamp()
	})

	return roleArtifacts, maxSeen, scannedThrough
}

func (signal *Signal) acceptsArtifact(artifact *datura.Artifact) bool {
	if signal == nil || signal.quoteCurrency == "" || artifact == nil {
		return true
	}

	scope, err := artifact.Scope()
	if err == nil && scope != "" {
		return symbolMatchesQuoteCurrency(scope, signal.quoteCurrency)
	}

	for rowIndex := 0; ; rowIndex++ {
		symbol := datura.Peek[string](artifact, "data", rowIndex, "symbol")
		if symbol == "" {
			break
		}

		if symbolMatchesQuoteCurrency(symbol, signal.quoteCurrency) {
			return true
		}
	}

	return false
}

func snapshotRole(role string) bool {
	switch role {
	case "book", "ticker", "ohlc":
		return true
	default:
		return false
	}
}

func coalesceSnapshotFrames(role string, frames []*datura.Artifact) []*datura.Artifact {
	if !snapshotRole(role) || len(frames) < 2 {
		return frames
	}

	latest := make(map[string]*datura.Artifact)
	passthrough := make([]*datura.Artifact, 0)

	for _, frame := range frames {
		if frame == nil {
			continue
		}

		scope := snapshotFrameScope(frame)
		if scope == "" {
			passthrough = append(passthrough, frame)
			continue
		}

		prior := latest[scope]
		if prior == nil || frame.Timestamp() >= prior.Timestamp() {
			latest[scope] = frame
		}
	}

	out := make([]*datura.Artifact, 0, len(latest)+len(passthrough))
	out = append(out, passthrough...)

	for _, frame := range latest {
		out = append(out, frame)
	}

	// ponytail: L2 book/ticker/OHLC are treated as current snapshots here, so
	// intra-pass snapshot churn is collapsed. Trade and level3 stay uncollapsed;
	// if L2 churn itself becomes a first-class signal, add a dedicated aggregate.
	sort.Slice(out, func(indexA, indexB int) bool {
		return out[indexA].Timestamp() < out[indexB].Timestamp()
	})

	return out
}

func snapshotFrameScope(frame *datura.Artifact) string {
	scope, err := frame.Scope()
	if err == nil && strings.Contains(scope, "/") {
		return scope
	}

	return datura.Peek[string](frame, "data", 0, "symbol")
}

func symbolMatchesQuoteCurrency(symbol string, quoteCurrency string) bool {
	if quoteCurrency == "" {
		return true
	}

	_, quote, ok := strings.Cut(symbol, "/")

	return ok && strings.ToUpper(quote) == quoteCurrency
}

/*
loadFrames fetches and caches artifacts from the tree across all statically
declared required roles. Caching prevents redundant tree queries between
Observe and Measure in one trader tick.
*/
func (signal *Signal) loadFrames() {
	if signal.cachedFramesByRole != nil {
		return
	}

	// Initialize per-role cursors from global lastTimestamp if manually set by tests on first run.
	if len(signal.lastTimestampByRole) == 0 {
		startStamp := signal.lastTimestamp
		if startStamp <= 0 {
			if flag.Lookup("test.v") != nil {
				startStamp = 0
			} else {
				startStamp = time.Now().UTC().UnixNano()
			}
		}
		for _, roleName := range ingestRoles {
			signal.lastTimestampByRole[roleName] = startStamp
			signal.lastObservedByRole[roleName] = startStamp
		}
	}

	framesByRole := make(map[string][]*datura.Artifact)
	roleCount := make(map[string]int)
	maxSeenByRole := make(map[string]int64)
	cursorByRole := make(map[string]int64)

	// Every book/level3 frame is replayed — no coalescing. Microstructure
	// signals (toxicity cancel/fill, depthflow spoof, exhaust thinning) need to
	// see the full intra-tick order-book churn; collapsing to the latest row per
	// symbol would erase exactly the events they measure. UI throttling, if any,
	// belongs in the broadcast path, not here.
	var wait sync.WaitGroup
	results := make(chan roleFrameResult, len(ingestRoles))

	for _, role := range ingestRoles {
		scanPrev := signal.lastTimestampByRole[role]
		observedPrev := signal.lastObservedByRole[role]

		wait.Go(func() {
			roleArtifacts, maxSeen, cursor := signal.loadRoleFrames(
				role,
				scanPrev,
				observedPrev,
			)

			results <- roleFrameResult{
				role:      role,
				frames:    roleArtifacts,
				maxSeen:   maxSeen,
				scanUntil: cursor,
			}
		})
	}

	wait.Wait()
	close(results)

	for result := range results {
		frames := coalesceSnapshotFrames(result.role, result.frames)
		framesByRole[result.role] = frames
		roleCount[result.role] = len(frames)
		maxSeenByRole[result.role] = result.maxSeen
		cursorByRole[result.role] = result.scanUntil
	}

	signal.cachedFramesByRole = framesByRole
	signal.lastRoleCount = roleCount
	signal.cachedMaxSeenByRole = maxSeenByRole
	signal.cachedCursorByRole = cursorByRole
}

/*
Measure seeks the tree by role and timestamp, which is how the
websocket stores frames. This allows us to implement a simple
cursor mechanism without a lot of complexity.
*/
func (signal *Signal) Measure(crossSection *market.CrossSection) []*datura.Artifact {
	// Ensure frames are loaded (e.g., if Measure is called directly without Observe)
	signal.loadFrames()

	framesByRole := signal.cachedFramesByRole
	maxSeenByRole := signal.cachedMaxSeenByRole
	cursorByRole := signal.cachedCursorByRole

	// Advance the cursors for each role, remembering the prior global cursor so
	// PollInterval can read the inter-pass ingest cadence from the stamp advance.
	for role, cursor := range cursorByRole {
		if cursor > signal.lastTimestampByRole[role] {
			signal.lastTimestampByRole[role] = cursor
		}
	}

	for role, maxSeen := range maxSeenByRole {
		if maxSeen > signal.lastObservedByRole[role] {
			signal.lastObservedByRole[role] = maxSeen
		}

		if maxSeen > signal.lastTimestamp {
			signal.prevObservedStamp = signal.lastTimestamp
			signal.lastTimestamp = maxSeen
		}
	}

	// Reset cached frames for the next tick
	signal.cachedFramesByRole = nil
	signal.cachedMaxSeenByRole = nil
	signal.cachedCursorByRole = nil

	measurements := make([]*datura.Artifact, 0)

	// Each signal owns mutable state, so frames stay ordered within that one
	// signal. Different signal instances are independent and can score the same
	// ingest batch concurrently without dropping any frame. Measurements are
	// written by the owning signal worker as they are produced, preserving
	// tree-backed same-signal replay for later frames in the same batch.
	for _, result := range signal.measureSignals(framesByRole, crossSection) {
		for _, measured := range result.measurements {
			measurements = append(measurements, measured)
		}
	}

	return measurements
}

func (signal *Signal) measureSignals(
	framesByRole map[string][]*datura.Artifact,
	crossSection *market.CrossSection,
) []signalMeasureResult {
	if signal == nil || len(signal.signals) == 0 {
		return nil
	}

	origins := make([]logic.SourceType, 0, len(signal.signals))

	for origin, sig := range signal.signals {
		if sig == nil {
			continue
		}

		origins = append(origins, origin)
	}

	sort.Slice(origins, func(indexA, indexB int) bool {
		return string(origins[indexA]) < string(origins[indexB])
	})

	results := make(chan signalMeasureResult, len(origins))
	var wait sync.WaitGroup

	for _, origin := range origins {
		sig := signal.signals[origin]

		wait.Go(func() {
			results <- measureSignal(origin, sig, signal.tree, framesByRole, crossSection)
		})
	}

	wait.Wait()
	close(results)

	measured := make([]signalMeasureResult, 0, len(origins))

	for result := range results {
		measured = append(measured, result)
	}

	sort.Slice(measured, func(indexA, indexB int) bool {
		return string(measured[indexA].origin) < string(measured[indexB].origin)
	})

	return measured
}

func measureSignal(
	origin logic.SourceType,
	sig market.Signal,
	tree *dmt.Tree,
	framesByRole map[string][]*datura.Artifact,
	crossSection *market.CrossSection,
) signalMeasureResult {
	result := signalMeasureResult{origin: origin}

	if resetter, ok := sig.(batchResetter); ok {
		resetter.ResetBatch()
	}

	for _, role := range sig.IngestRoles() {
		for _, artifact := range framesByRole[role] {
			for measured := range sig.Measure(artifact, crossSection) {
				if measured == nil {
					errnie.Error(errnie.Err(
						errnie.Validation,
						"trader: signal returned nil measurement",
						nil,
					))

					continue
				}

				measured.WithOrigin(
					string(origin),
				).WithRole(
					"measurement",
				)

				tree.InsertArtifact(measured.Prefix("role", "scope", "origin", "timestamp"), measured)
				result.measurements = append(result.measurements, measured)
			}
		}
	}

	return result
}

func (signal *Signal) Close() error {
	signal.cancel()

	for _, sig := range signal.signals {
		sig.Close()
	}

	return nil
}

/*
RoleCount returns the count of loaded frames for a role.
*/
func (signal *Signal) RoleCount(role string) int {
	return signal.lastRoleCount[role]
}

const (
	// The poll interval is bounded so the loop never busy-spins on a fast burst
	// nor sleeps so long it misses a regime turn. Within the band it tracks the
	// observed inter-frame cadence — the data sets the pace.
	minPollInterval = 10 * time.Millisecond
	maxPollInterval = time.Second
)

/*
PollInterval is the cadence-derived wait before the trader's next pass: the gap
between the two most recent ingest stamps the cursor advanced over, bounded to a
sane band. Before any ingest has been seen it returns the floor so the loop wakes
promptly and discovers the first frames. This replaces the fixed 100ms ticker —
a quiet market is polled slowly, a busy one quickly, without a magic constant.
*/
func (signal *Signal) PollInterval() time.Duration {
	gap := signal.lastTimestamp - signal.prevObservedStamp

	if signal.lastTimestamp <= 0 || signal.prevObservedStamp <= 0 || gap <= 0 {
		return minPollInterval
	}

	interval := time.Duration(gap)

	if interval < minPollInterval {
		return minPollInterval
	}

	if interval > maxPollInterval {
		return maxPollInterval
	}

	return interval
}
