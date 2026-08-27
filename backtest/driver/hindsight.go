package driver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/bytedance/sonic"
	"github.com/spf13/cobra"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/backtest"
	"github.com/theapemachine/symm/backtest/hindsight"
	"github.com/theapemachine/symm/utils"
)

/*
RealizedReport wraps the perfect-execution hindsight analysis for one capture:
the complete per-symbol breakdown plus a capture-wide summary of how much of
the tape's theoretical value the system did not collect.
*/
type RealizedReport struct {
	CaptureID             int64                        `json:"captureId"`
	Status                string                       `json:"status,omitempty"`
	Symbols               []hindsight.PerSymbol        `json:"symbols"`
	MissedPct             float64                      `json:"missedPct"`
	RealizedPct           float64                      `json:"realizedPct"`
	LossPct               float64                      `json:"lossPct"`
	UpboundPct            float64                      `json:"upboundPct"`
	MissedLegs            int                          `json:"missedLegs"`
	TotalLegs             int                          `json:"totalLegs"`
	LossPositions         int                          `json:"lossPositions"`
	ValueCaptureRate      float64                      `json:"valueCaptureRate"`
	LegCaptureRate        float64                      `json:"legCaptureRate"`
	DiagnosticCoverage    float64                      `json:"diagnosticCoverage"`
	RootCauses            []hindsight.RootCauseSummary `json:"rootCauses"`
	Recommendations       []hindsight.Recommendation   `json:"recommendations"`
	LossRootCauses        []hindsight.RootCauseSummary `json:"lossRootCauses"`
	LossRecommendations   []hindsight.Recommendation   `json:"lossRecommendations"`
}

/*
analyzeHindsight handles `symm backtest --hindsight`. It reads one captured
session's tape and decision stream, computes the perfect-execution ceiling and
the missed-opportunity breakdown, and writes the machine-readable report to
stdout unless --out names a target file. This is the headless path the
dashboard and regression suites call to learn exactly which moves the system
missed and which measurement scores were reading when it declined them.
*/
func analyzeHindsight(command *cobra.Command) error {
	dataPath := utils.ResolveDataPath()

	store, err := backtest.NewStore(filepath.Join(dataPath, "symm.sqlite"))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "hindsight: open store", err))
	}

	defer store.Close()

	captureID, _ := command.Flags().GetInt64("capture")

	if captureID == 0 {
		captures, listErr := store.ListCaptures()

		if listErr != nil {
			return errnie.Error(errnie.Err(errnie.Internal, "hindsight: list captures", listErr))
		}

		if len(captures) == 0 {
			return errnie.Error(errnie.Err(errnie.Validation, "hindsight: no captures to analyze", nil))
		}

		captureID = captures[0].ID
	}

	startedAt, endedAt, boundsErr := store.Bounds(captureID)

	if boundsErr != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "hindsight: capture bounds", boundsErr))
	}

	reducer := hindsight.NewReducer()
	frames, release, err := store.Frames(captureID, time.Time{})

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "hindsight: open frames", err))
	}

	reduced, reduceErr := streamTape(reducer, frames)

	release()

	if reduceErr != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "hindsight: reduce tape", reduceErr))
	}

	if reduced == 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "hindsight: capture has no trade tape", nil))
	}

	decisions, eventsErr := streamDecisions(store, startedAt, endedAt)

	if eventsErr != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "hindsight: read decisions", eventsErr))
	}

	reports, analysisErr := hindsight.Analyze(reducer, decisions)

	if analysisErr != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "hindsight: analyze", analysisErr))
	}

	report := summarize(captureID, reports)

	encoded, marshalErr := json.MarshalIndent(report, "", "  ")

	if marshalErr != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "hindsight: encode report", marshalErr))
	}

	if outputPath, _ := command.Flags().GetString("out"); outputPath != "" {
		if writeErr := os.WriteFile(outputPath, append(encoded, '\n'), 0o644); writeErr != nil {
			return errnie.Error(errnie.Err(errnie.IO, "hindsight: write report", writeErr))
		}

		errnie.Info(fmt.Sprintf("hindsight: wrote %d-symbol report to %s", len(reports), outputPath))

		return nil
	}

	_, printErr := fmt.Println(string(encoded))

	return printErr
}

/*
streamTape walks one capture's frames in order and reduces the trade prints to
per-symbol series. A cheap channel prefix gate keeps the book and heartbeat
frames — the vast majority of a capture — out of the decoder.
*/
func streamTape(reducer *hindsight.Reducer, frames func() (backtest.Frame, bool)) (int, error) {
	reducedCount := 0

	for {
		frame, ok := frames()

		if !ok {
			break
		}

		if frame.Endpoint != "public" || !isMarketTapePayload(frame.Payload) {
			continue
		}

		if err := reducer.Ingest(frame.Payload, frame.ReceivedAt); err != nil {
			return 0, err
		}

		reducedCount++
	}

	errnie.Info(fmt.Sprintf("hindsight: reduced %d trade/market frames into %d symbol series", reducedCount, len(reducer.Symbols())))

	return reducedCount, nil
}

/*
isMarketTapePayload probes whether a frame carries trade prints or ticker quotes for tape reduction.
*/
func isMarketTapePayload(payload []byte) bool {
	if len(payload) < len(`{"channel":"trade"`) {
		return false
	}

	return bytes.HasPrefix(payload, []byte(`{"channel":"trade"`)) ||
		bytes.HasPrefix(payload, []byte(`{"channel":"ticker"`))
}

/*
streamDecisions reads the analytical event stream and decodes the decision
moments hindsight needs, restricted to decisions whose own venue time falls
inside the capture window. The analytical stream accumulates across every run
in one store, so this gate — not the wall-clock write time — is what stops a
leg in one capture being attributed to a decision from a different session.
*/
func decodeDecisionEvent(
	kind string,
	payload []byte,
) ([]hindsight.Decision, bool, error) {
	switch kind {
	case audit.DecisionBatchEvent, "[]types.Decision", "[]*types.Decision":
		var batch []hindsight.Decision
		if err := sonic.Unmarshal(payload, &batch); err != nil {
			return nil, true, err
		}
		return batch, true, nil
	case "types.Decision", "*types.Decision":
		var decision hindsight.Decision
		if err := sonic.Unmarshal(payload, &decision); err != nil {
			return nil, true, err
		}
		return []hindsight.Decision{decision}, true, nil
	default:
		return nil, false, nil
	}
}

func streamDecisions(
	store *backtest.Store,
	from time.Time,
	to time.Time,
) ([]hindsight.Decision, error) {
	next, err := store.Events()

	if err != nil {
		return nil, err
	}

	decisions := []hindsight.Decision{}

	for {
		kind, payload, _, ok := next()

		if !ok {
			break
		}

		batch, decisionEvent, decodeErr := decodeDecisionEvent(kind, payload)

		if !decisionEvent {
			continue
		}

		if decodeErr != nil {
			errnie.Error(errnie.Err(errnie.Internal, "hindsight: decode decision event", decodeErr))
			continue
		}

		for _, decision := range batch {
			if decision.At.Before(from) {
				continue
			}

			if !to.IsZero() && decision.At.After(to) {
				continue
			}

			decisions = append(decisions, decision)
		}
	}

	errnie.Info(fmt.Sprintf("hindsight: read %d decision moments", len(decisions)))

	return decisions, nil
}

/*
summarize aggregates the per-symbol analysis into a capture-wide hindsight
report.
*/
func summarize(captureID int64, reports []hindsight.PerSymbol) RealizedReport {
	summary := RealizedReport{
		CaptureID:           captureID,
		Symbols:             reports,
		DiagnosticCoverage:  hindsight.DiagnosticCoverage(reports),
		RootCauses:          hindsight.RootCauseSummaries(reports),
		Recommendations:     hindsight.AggregateRecommendations(reports),
		LossRootCauses:      hindsight.LossRootCauseSummaries(reports),
		LossRecommendations: hindsight.AggregateLossRecommendations(reports),
	}

	for _, report := range reports {
		summary.MissedPct += report.MissedPct
		summary.RealizedPct += report.RealizedPct
		summary.LossPct += report.LossPct
		summary.UpboundPct += report.UpboundPct
		summary.MissedLegs += report.MissedLegs
		summary.TotalLegs += report.Legs
		summary.LossPositions += report.LossPositions
	}

	if summary.UpboundPct > 0 {
		summary.ValueCaptureRate = summary.RealizedPct / summary.UpboundPct
	}

	if summary.TotalLegs == 0 {
		summary.LegCaptureRate = 1
		return summary
	}

	summary.LegCaptureRate = float64(summary.TotalLegs-summary.MissedLegs) /
		float64(summary.TotalLegs)

	return summary
}

/*
Hindsight runs the perfect-execution analysis for one capture and publishes the
report to the dashboard. The analysis streams through an independent store
connection so it never competes with the playback session for the single
pooled connection, and it runs off the command loop so a long tape never
blocks playback controls.
*/
func (driver *Driver) Hindsight(captureID int64) {
	if captureID == 0 {
		return
	}

	if !driver.hindsightRunning.CompareAndSwap(0, captureID) {
		return
	}

	go func() {
		defer driver.hindsightRunning.Store(0)

		driver.publishHindsight(RealizedReport{
			CaptureID: captureID,
			Status:    "analyzing",
		})

		report, err := driver.buildHindsight(captureID)

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.Internal,
				"hindsight: analyze capture",
				err,
			))

			driver.publishHindsight(RealizedReport{
				CaptureID: captureID,
				Status:    "error",
			})

			return
		}

		report.Status = "ready"
		driver.publishHindsight(report)
	}()
}

/*
buildHindsight streams one capture's tape and decision stream on a separate
store connection and reduces them into the realized report.
*/
func (driver *Driver) buildHindsight(captureID int64) (RealizedReport, error) {
	store, err := driver.store.Reopen()

	if err != nil {
		return RealizedReport{}, err
	}

	defer store.Close()

	startedAt, endedAt, err := store.Bounds(captureID)

	if err != nil {
		return RealizedReport{}, err
	}

	reducer := hindsight.NewReducer()
	frames, release, err := store.Frames(captureID, time.Time{})

	if err != nil {
		return RealizedReport{}, err
	}

	reduced, reduceErr := streamTape(reducer, frames)

	release()

	if reduceErr != nil {
		return RealizedReport{}, reduceErr
	}

	if reduced == 0 {
		return RealizedReport{}, fmt.Errorf("hindsight: capture %d has no trade tape", captureID)
	}

	decisions, eventsErr := streamDecisions(store, startedAt, endedAt)

	if eventsErr != nil {
		return RealizedReport{}, eventsErr
	}

	reports, analysisErr := hindsight.Analyze(reducer, decisions)

	if analysisErr != nil {
		return RealizedReport{}, analysisErr
	}

	return summarize(captureID, reports), nil
}
