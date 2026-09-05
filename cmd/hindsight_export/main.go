/*
hindsight_export writes the decision record of a captured run to JSONL, so a
run can be shared and analyzed without shipping the multi-gigabyte capture tape
it came from.

The capture database holds the whole market record. What explains a run's
behaviour is a much smaller thing: for each planning round, the gate it stopped
at, the council distribution it stopped on, and the search it ran when it got
that far. This reads exactly that and nothing else.

	hindsight_export <events.sqlite> [flags]

	-run     run id (default: the most recent run in the database)
	-out     output file (default: stdout)
	-symbol  only this symbol
	-status  only this predictiveStatus (repeatable, comma-separated)
	-acted   only rounds whose action was not "nothing"
	-limit   stop after N rounds (0 = no limit)

One JSON object per line, so the output greps, streams, and loads into any
dataframe without a parser.
*/
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/telemetry/generated/telemetry"
)

/*
round is one planning round as it is shared: the identity needed to find it
again in the capture, the gate outcome, and the evidence that produced it.
*/
type round struct {
	Run      string `json:"run"`
	Sequence uint64 `json:"sequence"`
	Ordinal  uint64 `json:"ordinal"`
	Tick     int64  `json:"tick"`
	Symbol   string `json:"symbol"`
	At       int64  `json:"at"`

	// Where the round ended up.
	Action           string `json:"action"`
	PredictiveStatus string `json:"predictiveStatus"`
	PredictiveReady  bool   `json:"predictiveReady"`
	Reason           string `json:"reason"`
	Cause            string `json:"cause"`

	// What the council said. Probabilities are the full distribution, not the
	// argmax: a 0.27 winner over a 0.26 runner-up is a different market read
	// than a 0.9 winner, and only the distribution shows that.
	Confidence    float64            `json:"confidence"`
	Probabilities map[string]float64 `json:"moveProbabilities,omitempty"`
	Consensus     map[string]float64 `json:"consensus,omitempty"`

	// What the search did, when it ran at all.
	Search *searchRound `json:"search,omitempty"`

	// What entering would have cost at that moment.
	Execution map[string]float64 `json:"execution,omitempty"`

	// Signal metric measurements when requested with -metrics.
	Metrics map[string]float64 `json:"metrics,omitempty"`

	ForecastSource   string  `json:"forecastSource,omitempty"`
	ForecastModel    string  `json:"forecastModel,omitempty"`
	ForecastHorizon  int64   `json:"forecastHorizon,omitempty"`
	CalibrationCount uint64  `json:"calibrationCount,omitempty"`
	ReferencePrice   string  `json:"referencePrice,omitempty"`
	ProposedNotional string  `json:"proposedNotional,omitempty"`
	TaskSkill        float64 `json:"taskSkill,omitempty"`
}

/*
searchRound is the causal search's own account of the round: what it concluded,
how hard it looked, and how the root actions compared.
*/
type searchRound struct {
	RecommendedAction    string         `json:"recommendedAction"`
	IdentificationStatus string         `json:"identificationStatus"`
	DecisionUnavailable  bool           `json:"decisionUnavailable"`
	Iterations           int64          `json:"iterations"`
	Horizon              int64          `json:"horizon"`
	MaxDepth             int64          `json:"maxDepth"`
	TotalNodes           int64          `json:"totalNodes"`
	ExpectedOutcome      float64        `json:"expectedOutcome"`
	OutcomeUncertainty   float64        `json:"outcomeUncertainty"`
	TransitionSource     string         `json:"transitionSource,omitempty"`
	DominantMove         string         `json:"consensusDominantMove,omitempty"`
	Participants         int64          `json:"consensusParticipants,omitempty"`
	Vetoes               []string       `json:"vetoes,omitempty"`
	Synergies            []string       `json:"synergies,omitempty"`
	Branches             []searchBranch `json:"branches,omitempty"`
}

/* searchBranch is one root action's aggregate statistics. */
type searchBranch struct {
	Action             string  `json:"action"`
	Visits             int64   `json:"visits"`
	MeanReward         float64 `json:"meanReward"`
	BlendedValue       float64 `json:"blendedValue"`
	RewardStd          float64 `json:"rewardStd"`
	CounterfactualMass float64 `json:"counterfactualMass"`
	CausalExpectation  float64 `json:"causalExpectation,omitempty"`
	CausalDefined      bool    `json:"causalExpectationDefined"`
	Pruned             bool    `json:"pruned"`
}

/*
perspectiveRecord is one falsifiable perspective evaluation emitted by an advisor,
correlated directly with the signal metric measurements present on that envelope.
*/
type perspectiveRecord struct {
	Kind               string              `json:"kind"`
	Run                string              `json:"run"`
	Sequence           uint64              `json:"sequence"`
	Ordinal            uint64              `json:"ordinal"`
	Tick               int64               `json:"tick"`
	Symbol             string              `json:"symbol"`
	Advisor            string              `json:"advisor"`
	Class              string              `json:"class"`
	ClaimSequence      uint64              `json:"claimSequence"`
	Classes            map[string]float64  `json:"classes,omitempty"`
	Evidence           map[string][]string `json:"evidence,omitempty"`
	Metrics            map[string]float64  `json:"metrics,omitempty"`
	IssuedAt           int64               `json:"issuedAt"`
	ResolvedAt         int64               `json:"resolvedAt,omitempty"`
	ResolvedCoordinate uint64              `json:"resolvedCoordinate,omitempty"`
	Round              uint64              `json:"round"`
	Lifecycle          string              `json:"lifecycle"`
	ResolvedBy         string              `json:"resolvedBy,omitempty"`
	Clock              string              `json:"clock"`
	LeaseFrom          uint64              `json:"leaseFrom"`
	LeaseUntil         uint64              `json:"leaseUntil"`
	Predictions        []predictionRecord  `json:"predictions"`
}

/* predictionRecord preserves one class's observable future event contract. */
type predictionRecord struct {
	Class  string `json:"class"`
	Event  string `json:"event"`
	Effect string `json:"effect"`
	Move   string `json:"move"`
}

func main() {
	runID := flag.String("run", "", "run id (default: most recent)")
	out := flag.String("out", "", "output file (default: stdout)")
	symbol := flag.String("symbol", "", "only this symbol")
	status := flag.String("status", "", "only these predictiveStatus values (comma-separated)")
	acted := flag.Bool("acted", false, `only rounds whose action is not "nothing"`)
	limit := flag.Int("limit", 0, "stop after N exported records (0 = no limit)")
	metrics := flag.Bool("metrics", false, "include signal metric measurements on each round")
	summarize := flag.Bool("summary", false, "emit one aggregate object instead of per-round lines")
	perspectives := flag.Bool("perspectives", false, "export advisor perspective records instead of decision rounds")
	advisor := flag.String("advisor", "", "only this advisor name (when -perspectives is set)")
	trainingClock := flag.String("training-clock", "", "also export retained metric observations when this clock advances")
	opportunities := flag.Bool("opportunities", false, "also export canonical Hindsight price episodes")
	// Go's flag package stops at the first positional argument, so parse the
	// database path out of the arguments first and let flags appear on either
	// side of it. Without this, `hindsight_export db.sqlite -limit 3` silently
	// ignores every flag.
	database, rest := splitDatabase(os.Args[1:])

	if database == "" {
		fmt.Fprintln(os.Stderr, "usage: hindsight_export <events.sqlite> [flags]")
		flag.PrintDefaults()
		os.Exit(1)
	}

	if err := flag.CommandLine.Parse(rest); err != nil {
		os.Exit(1)
	}

	engine, err := store.NewSQLite(database)

	if err != nil {
		fmt.Fprintln(os.Stderr, "open capture:", err)
		os.Exit(1)
	}

	defer engine.Close()

	selected := *runID

	if selected == "" {
		selected, err = latestRun(engine)

		if err != nil {
			fmt.Fprintln(os.Stderr, "resolve run:", err)
			os.Exit(1)
		}
	}

	states, err := engine.ListStates(selected)

	if err != nil {
		fmt.Fprintln(os.Stderr, "list states:", err)
		os.Exit(1)
	}

	writer := os.Stdout

	if *out != "" {
		writer, err = os.Create(*out)

		if err != nil {
			fmt.Fprintln(os.Stderr, "create output:", err)
			os.Exit(1)
		}

		defer writer.Close()
	}

	buffered := bufio.NewWriter(writer)
	defer buffered.Flush()

	encoder := json.NewEncoder(buffered)
	wanted := statusSet(*status)
	written := 0

	aggregate := newSummary(selected)
	symbols := make(map[string]bool)
	confidences := make([]float64, 0, len(states))

	if *perspectives || *trainingClock != "" {
		observationStream := newTrainingObservationStream(*trainingClock)

		for _, entry := range states {
			if len(entry.Payload) == 0 {
				continue
			}

			state := telemetry.GetRootAsEnvelopeState(entry.Payload, 0)

			if state == nil {
				continue
			}

			var metricsMap map[string]float64

			if *metrics || *trainingClock != "" {
				metricsMap = extractAllMetrics(state)
			}

			if *trainingClock != "" {
				observation, observed, err := observationStream.Observe(
					selected,
					uint64(entry.Envelope.Origin.Sequence),
					entry.Envelope.Ordinal,
					state,
					metricsMap,
				)

				if err != nil {
					fmt.Fprintln(os.Stderr, "export training observation:", err)
					os.Exit(1)
				}

				if observed {
					written++

					if err := encoder.Encode(observation); err != nil {
						fmt.Fprintln(os.Stderr, "encode training observation:", err)
						os.Exit(1)
					}

					if *limit > 0 && written >= *limit {
						buffered.Flush()
						fmt.Fprintf(os.Stderr, "exported %d records from run %s\n", written, selected)

						return
					}
				}
			}

			if !*perspectives || state.PerspectivesLength() == 0 {
				continue
			}

			for perspectiveIndex := 0; perspectiveIndex < state.PerspectivesLength(); perspectiveIndex++ {
				perspective := new(telemetry.EnvelopePerspective)

				if !state.Perspectives(perspective, perspectiveIndex) {
					continue
				}

				advisorName := string(perspective.Advisor())

				if *advisor != "" && advisorName != *advisor {
					continue
				}

				symbolName := string(perspective.Symbol())

				if *symbol != "" && symbolName != *symbol {
					continue
				}

				record := buildPerspectiveRecord(
					selected,
					uint64(entry.Envelope.Origin.Sequence),
					entry.Envelope.Ordinal,
					state.Tick(),
					perspective,
					metricsMap,
				)

				written++

				if err := encoder.Encode(record); err != nil {
					fmt.Fprintln(os.Stderr, "encode perspective:", err)
					os.Exit(1)
				}

				if *limit > 0 && written >= *limit {
					buffered.Flush()
					fmt.Fprintf(os.Stderr, "exported %d records from run %s\n", written, selected)

					return
				}
			}
		}

		if *opportunities {
			episodes, err := writeOpportunityRecords(engine, selected, encoder)

			if err != nil {
				fmt.Fprintln(os.Stderr, "export opportunities:", err)
				os.Exit(1)
			}

			written += episodes
		}

		buffered.Flush()
		fmt.Fprintf(os.Stderr, "exported %d records from run %s\n", written, selected)

		return
	}

	for _, entry := range states {
		if len(entry.Payload) == 0 {
			continue
		}

		state := telemetry.GetRootAsEnvelopeState(entry.Payload, 0)

		if state == nil {
			continue
		}

		strategy := state.Strategy(nil)

		if strategy == nil {
			continue
		}

		for index := range strategy.DecisionsLength() {
			decision := new(telemetry.Decision)

			if !strategy.Decisions(decision, index) {
				continue
			}

			record := buildRound(selected, uint64(entry.Envelope.Origin.Sequence),
				entry.Envelope.Ordinal, state, decision, *metrics)

			if !keep(record, *symbol, wanted, *acted) {
				continue
			}

			written++

			if *summarize {
				aggregate.observe(record, symbols, &confidences)
			} else if err := encoder.Encode(record); err != nil {
				fmt.Fprintln(os.Stderr, "encode round:", err)
				os.Exit(1)
			}

			if *limit > 0 && written >= *limit {
				finish(buffered, aggregate, symbols, confidences, *summarize)
				fmt.Fprintf(os.Stderr, "exported %d rounds from run %s\n", written, selected)

				return
			}
		}
	}

	finish(buffered, aggregate, symbols, confidences, *summarize)
	fmt.Fprintf(os.Stderr, "exported %d rounds from run %s\n", written, selected)
}

/*
finish writes the aggregate when summarizing, then flushes whatever was
buffered so the output is complete on either path.
*/
func finish(
	buffered *bufio.Writer,
	aggregate *summary,
	symbols map[string]bool,
	confidences []float64,
	summarize bool,
) {
	if summarize {
		aggregate.finish(symbols, confidences)

		if err := aggregate.write(buffered); err != nil {
			fmt.Fprintln(os.Stderr, "encode summary:", err)
			os.Exit(1)
		}
	}

	buffered.Flush()
}

/*
splitDatabase pulls the first non-flag argument out of the argument list and
returns it with the remaining arguments, so flags may be written before or
after the database path.
*/
func splitDatabase(arguments []string) (string, []string) {
	rest := make([]string, 0, len(arguments))
	database := ""

	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]

		if database == "" && !strings.HasPrefix(argument, "-") {
			database = argument

			continue
		}

		rest = append(rest, argument)
	}

	return database, rest
}

/* latestRun returns the most recently started run in the capture. */
func latestRun(engine *store.SQLite) (string, error) {
	runs, err := engine.ListRuns()

	if err != nil {
		return "", err
	}

	if len(runs) == 0 {
		return "", fmt.Errorf("capture contains no runs")
	}

	return string(runs[0].ID), nil
}

func statusSet(raw string) map[string]bool {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	wanted := make(map[string]bool)

	for _, value := range strings.Split(raw, ",") {
		trimmed := strings.TrimSpace(value)

		if trimmed != "" {
			wanted[trimmed] = true
		}
	}

	return wanted
}

func keep(record round, symbol string, wanted map[string]bool, acted bool) bool {
	if symbol != "" && record.Symbol != symbol {
		return false
	}

	if len(wanted) > 0 && !wanted[record.PredictiveStatus] {
		return false
	}

	if acted && record.Action == "nothing" {
		return false
	}

	return true
}
