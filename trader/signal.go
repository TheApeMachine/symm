package trader

import (
	"context"
	"sort"
	"time"

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
	cachedFramesByRole  map[string][]*datura.Artifact
	cachedMaxSeenByRole map[string]int64
	cachedCursorByRole  map[string]int64
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
observed timestamp cursor. The tree keys are second-granular before their uuid
suffix, so seeking each elapsed second prefix crosses second boundaries without
falling back to a full role scan after the first scan cursor. The observed cursor
filter keeps each tick to genuinely new frames and there is no lookahead pad.

It returns both maxSeen (the newest artifact timestamp actually observed) and
scannedThrough (the role cursor through wall-clock now). Empty scans advance only
the per-role scan cursor, not the global market timestamp, so quiet roles do not
rescan old empty seconds forever and PollInterval still reflects real ingest.

ponytail: this still scans whole seconds up to wall-clock now because the tree
key does not expose a seek-from-exact-nanosecond suffix. Upgrade by indexing
role/timestamp/ns directly when datura/dmt supports lower-bound seek.
*/
func (signal *Signal) loadRoleFrames(role string, scanPrev int64, observedPrev int64) ([]*datura.Artifact, int64, int64) {
	roleArtifacts := make([]*datura.Artifact, 0)
	maxSeen := observedPrev
	now := time.Now().UTC()
	scannedThrough := scanPrev
	seen := make(map[string]struct{})

	for _, prefix := range roleSeekPrefixes(role, scanPrev, now) {
		for artifact := range signal.tree.Seek(prefix) {
			artifactRole, err := artifact.Role()
			if err != nil || artifactRole != role {
				continue
			}

			uuid, err := artifact.Uuid()
			if err == nil && len(uuid) > 0 {
				key := string(uuid)
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
			}

			if artifact.Timestamp() <= observedPrev {
				continue
			}

			if timestamp := artifact.Timestamp(); timestamp > maxSeen {
				maxSeen = timestamp
			}

			roleArtifacts = append(roleArtifacts, artifact)
		}
	}

	if scanPrev > 0 {
		scannedThrough = max(maxSeen, now.UnixNano())
	} else {
		scannedThrough = maxSeen
	}

	sort.Slice(roleArtifacts, func(indexA, indexB int) bool {
		return roleArtifacts[indexA].Timestamp() < roleArtifacts[indexB].Timestamp()
	})

	return roleArtifacts, maxSeen, scannedThrough
}

func roleSeekPrefixes(role string, prev int64, now time.Time) [][]byte {
	if prev <= 0 {
		return [][]byte{[]byte(role + "/")}
	}

	start := time.Unix(0, prev).UTC().Truncate(time.Second)
	end := now.UTC().Truncate(time.Second)

	if end.Before(start) {
		end = start
	}

	prefixes := make([][]byte, 0, int(end.Sub(start)/time.Second)+1)

	for cursor := start; !cursor.After(end); cursor = cursor.Add(time.Second) {
		prefixes = append(prefixes, []byte(role+"/"+cursor.Format("2006/01/02/15/04/05")+"/"))
	}

	return prefixes
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
	if len(signal.lastTimestampByRole) == 0 && signal.lastTimestamp > 0 {
		for _, roleName := range ingestRoles {
			signal.lastTimestampByRole[roleName] = signal.lastTimestamp
			signal.lastObservedByRole[roleName] = signal.lastTimestamp
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
	for _, role := range ingestRoles {
		roleArtifacts, maxSeen, cursor := signal.loadRoleFrames(
			role,
			signal.lastTimestampByRole[role],
			signal.lastObservedByRole[role],
		)
		framesByRole[role] = roleArtifacts
		roleCount[role] = len(roleArtifacts)
		maxSeenByRole[role] = maxSeen
		cursorByRole[role] = cursor
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

	measurements := make([]*datura.Artifact, 0)

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

	// Each signal owns mutable state. Score one signal at a time and store the
	// measurement artifacts directly, so qpool is not used as a large artifact
	// transport and measurements still persist in the tree.
	for origin, sig := range signal.signals {
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

					signal.tree.InsertArtifact(measured.Prefix("role", "scope", "origin", "timestamp"), measured)
					measurements = append(measurements, measured)
				}
			}
		}
	}

	return measurements
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
