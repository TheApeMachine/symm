package ui

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/signal"
	"github.com/theapemachine/symm/store"
)

/*
timelineCache holds the assembled RunIndex for the runs currently under
inspection. Building an index streams one Run's raw capture tape once; holding
the result avoids re-reading the tape for every pan, zoom, and symbol change
while keeping the retained set explicitly bounded (§62 — events move, state
stays; nothing here retains a growing market-event database).

An index of a Run still being captured goes stale the moment the process writes
another frame, so each entry records when it was built and is rebuilt once it
is older than timelineFreshness.
*/
type timelineCache struct {
	mutex   sync.Mutex
	entries map[hindsight.RunID]*timelineEntry
	order   []hindsight.RunID
}

type timelineEntry struct {
	index  *hindsight.RunIndex
	loaded time.Time
}

const (
	// timelineRuns is how many Run indices are retained at once.
	timelineRuns = 2
	// timelineFreshness is how long an index of a live Run may be reused
	// before the tape is re-read.
	timelineFreshness = 30 * time.Second
)

func newTimelineCache() *timelineCache {
	return &timelineCache{
		entries: make(map[hindsight.RunID]*timelineEntry),
		order:   make([]hindsight.RunID, 0, timelineRuns),
	}
}

/*
index returns the RunIndex for one Run, reading the raw capture tape when the
cached projection is absent or stale.
*/
func (cache *timelineCache) index(
	engine *store.SQLite,
	run hindsight.RunID,
) (*hindsight.RunIndex, error) {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()

	entry, known := cache.entries[run]

	if known && time.Since(entry.loaded) < timelineFreshness {
		return entry.index, nil
	}

	observations, err := engine.ListMarketObservations(string(run))

	if err != nil {
		return nil, err
	}

	built := &timelineEntry{
		index:  hindsight.NewRunIndex(run, observations),
		loaded: time.Now(),
	}

	if !known {
		cache.order = append(cache.order, run)

		for len(cache.order) > timelineRuns {
			delete(cache.entries, cache.order[0])
			cache.order = cache.order[1:]
		}
	}

	cache.entries[run] = built

	return built.index, nil
}

/*
registerTimeline mounts the Episode-discovery reads. They project the capture
tape — the external market record — and never read a witness, so the moments
they mark are selected by market coordinates alone (§27).
*/
func (hub *Hub) registerTimeline() {
	// /hindsight/metric-map serves the declared semantic identity of every
	// production (source, metric) pair — what the number physically means,
	// where it may legitimately travel, and what must never be inferred from
	// it. Inspection shows these statements verbatim; a metric with no entry
	// reads as undeclared rather than being given an invented meaning.
	hub.app.Get("/hindsight/metric-map", func(c fiber.Ctx) error {
		return c.JSON(signal.Semantics())
	})

	// /hindsight/resident answers "what did SYMM actually hold here?" as opposed
	// to "what did this envelope carry?". For each signal family it walks the
	// instrument's own captures backwards from the coordinate and takes the
	// latest value causally available there, reporting the exact origin and the
	// age of every carried fact (§18, §19). It never looks forward (§31), and it
	// never resolves by timestamp proximity (§9).
	hub.app.Get("/hindsight/resident", func(c fiber.Ctx) error {
		if hub.store == nil {
			return fiber.NewError(
				fiber.StatusServiceUnavailable,
				"capture store unavailable",
			)
		}

		run := query(c, "run")
		symbol := query(c, "symbol")
		sequence := hindsight.CaptureSequence(parseUintQuery(c.Query("seq")))

		if run == "" || symbol == "" || sequence == 0 {
			return fiber.NewError(
				fiber.StatusBadRequest,
				"run, symbol and seq are required",
			)
		}

		index, err := hub.timelines.index(hub.store, hindsight.RunID(run))

		if err != nil {
			return err
		}

		budget := intQuery(c, "budget", 64)

		if budget > 512 {
			budget = 512
		}

		at := time.Time{}

		if observation, known := index.ObservationAt(symbol, sequence); known {
			at = observation.At()
		}

		resident, err := hindsight.ResolveResident(
			hindsight.RunID(run),
			symbol,
			sequence,
			at,
			index.CapturesBefore(symbol, sequence, budget),
			hub.store,
			budget,
		)

		if err != nil {
			return err
		}

		return c.JSON(resident)
	})

	// /hindsight/timeline projects one Run onto the capture axis for one
	// instrument: the declared coordinate's bucketed shape, the Episodes the
	// declared selector found on it, the transport spans that carried it, and
	// the instrument index ranked by observed market magnitude.
	hub.app.Get("/hindsight/timeline", func(c fiber.Ctx) error {
		// No capture store attached is unavailable, not empty (§43). Saying so
		// with a status keeps the reader from reading an absent tape as a run
		// in which nothing happened.
		if hub.store == nil {
			return fiber.NewError(
				fiber.StatusServiceUnavailable,
				"capture store unavailable",
			)
		}

		run := query(c, "run")

		if run == "" {
			return fiber.NewError(fiber.StatusBadRequest, "run is required")
		}

		index, err := hub.timelines.index(hub.store, hindsight.RunID(run))

		if err != nil {
			return err
		}

		request := hindsight.TimelineRequest{
			Run:     hindsight.RunID(run),
			Symbol:  query(c, "symbol"),
			Policy:  timelinePolicy(c),
			Axis:    hindsight.TimelineAxis(query(c, "axis")),
			Buckets: int(parseUintQuery(c.Query("buckets"))),
			FromSeq: hindsight.CaptureSequence(parseUintQuery(c.Query("from"))),
			ToSeq:   hindsight.CaptureSequence(parseUintQuery(c.Query("to"))),
			Symbols: c.Query("symbols") == "1",
		}

		// With no instrument declared, the highest-magnitude instrument on the
		// tape is projected so the view opens on something inspectable. That is
		// a market ranking, never a ranking by what SYMM thought (§27).
		if request.Symbol == "" {
			summaries := index.Summaries(request.Policy)

			if len(summaries) > 0 {
				request.Symbol = summaries[0].Symbol
			}
		}

		timeline := index.Project(request)

		return c.JSON(struct {
			hindsight.Timeline
			IndexedAt time.Time `json:"indexedAt"`
		}{
			Timeline:  timeline,
			IndexedAt: index.BuiltAt(),
		})
	})
}

/*
timelinePolicy reads the declared selector off the request. Every threshold is
overridable so a reader can state exactly what "interesting" should mean, and
whatever it resolves to travels back with the result.
*/
func timelinePolicy(c fiber.Ctx) hindsight.DiscoveryPolicy {
	policy := hindsight.DefaultDiscoveryPolicy()

	if coordinate := hindsight.MarketCoordinate(query(c, "coordinate")); coordinate.Valid() {
		policy.Coordinate = coordinate
	}

	policy.FloorExcursion = floatQuery(c, "floorExcursion", policy.FloorExcursion)
	policy.ExcursionSigmas = floatQuery(c, "excursionSigmas", policy.ExcursionSigmas)
	policy.RetraceFraction = floatQuery(c, "retraceFraction", policy.RetraceFraction)
	policy.VolatilityRatio = floatQuery(c, "volatilityRatio", policy.VolatilityRatio)
	policy.SpreadRatio = floatQuery(c, "spreadRatio", policy.SpreadRatio)
	policy.DepthRatio = floatQuery(c, "depthRatio", policy.DepthRatio)
	policy.ArrivalRatio = floatQuery(c, "arrivalRatio", policy.ArrivalRatio)
	policy.ExcursionHorizon = intQuery(c, "excursionHorizon", policy.ExcursionHorizon)
	policy.RegimeWindow = intQuery(c, "regimeWindow", policy.RegimeWindow)
	policy.RegimeBaseline = intQuery(c, "regimeBaseline", policy.RegimeBaseline)
	policy.MinObservations = intQuery(c, "minObservations", policy.MinObservations)

	return policy
}

/*
query reads one query parameter as a string this process owns.

The value fiber hands back points into the connection's request buffer, which
fasthttp recycles the moment the response is written. Anything retained past
that — a cache key, a run identity carried onto every observation, a symbol
stored inside an episode — would silently become whatever the next request
wrote over those bytes. That is exactly the class of defect Hindsight exists to
catch, so this surface does not commit it: every retained string is copied at
the boundary.
*/
func query(c fiber.Ctx, name string) string {
	return strings.Clone(c.Query(name))
}

func floatQuery(c fiber.Ctx, name string, fallback float64) float64 {
	raw := c.Query(name)

	if raw == "" {
		return fallback
	}

	value, err := strconv.ParseFloat(raw, 64)

	if err != nil {
		return fallback
	}

	return value
}

func intQuery(c fiber.Ctx, name string, fallback int) int {
	raw := c.Query(name)

	if raw == "" {
		return fallback
	}

	value, err := strconv.Atoi(raw)

	if err != nil {
		return fallback
	}

	return value
}
