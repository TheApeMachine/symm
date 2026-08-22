package driver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/bytedance/sonic"
	"github.com/spf13/cobra"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/backtest"
	"github.com/theapemachine/symm/backtest/hindsight"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/utils"
)

/*
RealizedReport wraps the perfect-execution hindsight analysis for one capture:
the complete per-symbol breakdown plus a capture-wide summary of how much of
the tape's theoretical value the system did not collect.
*/
type RealizedReport struct {
	CaptureID  int64                 `json:"captureId"`
	Status     string                `json:"status,omitempty"`
	Symbols    []hindsight.PerSymbol `json:"symbols"`
	MissedPct  float64               `json:"missedPct"`
	UpboundPct float64               `json:"upboundPct"`
	MissedLegs int                   `json:"missedLegs"`
	TotalLegs  int                   `json:"totalLegs"`
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

		if frame.Endpoint != "public" || !isTradePayload(frame.Payload) {
			continue
		}

		if err := reducer.Ingest(frame.Payload, frame.ReceivedAt); err != nil {
			return 0, err
		}

		reducedCount++
	}

	errnie.Info(fmt.Sprintf("hindsight: reduced %d trade frames into %d symbol series", reducedCount, len(reducer.Symbols())))

	return reducedCount, nil
}

/*
isTradePayload is a light prefix probe that accepts a captured payload only
when it begins as a trade update, so the reducer's deeper decode is not invoked
on book or heartbeat frames.
*/
func isTradePayload(payload []byte) bool {
	return len(payload) >= len(`{"channel":"trade"`) && bytes.HasPrefix(payload, []byte(`{"channel":"trade"`))
}

/*
streamDecisions reads the analytical event stream and decodes the decision
moments hindsight needs, restricted to decisions whose own venue time falls
inside the capture window. The analytical stream accumulates across every run
in one store, so this gate — not the wall-clock write time — is what stops a
leg in one capture being attributed to a decision from a different session.
*/
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

		if kind != "[]types.Decision" {
			continue
		}

		var batch []hindsight.Decision

		if decodeErr := sonic.Unmarshal(payload, &batch); decodeErr != nil {
			errnie.Error(errnie.Err(errnie.Internal, "hindsight: decode decision batch", decodeErr))
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
	summary := RealizedReport{CaptureID: captureID, Symbols: reports}

	for _, report := range reports {
		summary.MissedPct += report.MissedPct
		summary.UpboundPct += report.UpboundPct
		summary.MissedLegs += report.MissedLegs
		summary.TotalLegs += report.Legs
	}

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

/*
publishHindsight emits one hindsight wire frame for the dashboard.
*/
func (driver *Driver) publishHindsight(report RealizedReport) {
	driver.ui.Push(&wire.FrameT{
		Type:  wire.FrameHindsightFrame,
		Value: hindsightWire(report),
	})
}

func hindsightWire(report RealizedReport) *wire.HindsightFrameT {
	symbols := make([]*wire.HindsightSymbolT, 0, len(report.Symbols))

	for _, symbol := range report.Symbols {
		opportunities := make([]*wire.HindsightOpportunityT, 0, len(symbol.Opportunities))

		for _, opportunity := range symbol.Opportunities {
			journal := make([]*wire.HindsightSignalT, 0, len(opportunity.Journal))

			for _, decision := range opportunity.Journal {
				journal = append(journal, hindsightSignalWire(signalWireFields{
					At:                  decision.At,
					Action:              decision.Action,
					Reason:              decision.Reason,
					Cause:               decision.Cause,
					GraphScore:          decision.GraphScore,
					ThesisScore:         decision.ThesisScore,
					ThesisConfidence:    decision.ThesisConfidence,
					ThesisSupport:       decision.ThesisSupport,
					ThesisContradiction: decision.ThesisContradiction,
					ThesisConditions:    decision.ThesisConditions,
					Direction:           decision.Direction,
					Confidence:          decision.Confidence,
					AdmissionThreshold:  decision.AdmissionThreshold,
					Opportunity:         decision.Opportunity,
					OpportunityType:     decision.OpportunityType,
					PredictiveReady:     decision.PredictiveReady,
					PredictiveStatus:    decision.PredictiveStatus,
					Alternatives:        decision.Alternatives,
				}))
			}

			opportunities = append(opportunities, &wire.HindsightOpportunityT{
				Leg: &wire.HindsightLegT{
					Symbol: opportunity.Leg.Symbol, BuyAt: opportunity.Leg.BuyAt.UnixNano(),
					SellAt: opportunity.Leg.SellAt.UnixNano(), BuyPrice: opportunity.Leg.BuyPrice,
					SellPrice: opportunity.Leg.SellPrice, ProfitPct: opportunity.Leg.ProfitPct,
				},
				Signal: hindsightSignalWire(signalWireFields{
					At:                  opportunity.Signal.At,
					Action:              opportunity.Signal.Action,
					Reason:              opportunity.Signal.Reason,
					Cause:               opportunity.Signal.Cause,
					GraphScore:          opportunity.Signal.GraphScore,
					ThesisScore:         opportunity.Signal.ThesisScore,
					ThesisConfidence:    opportunity.Signal.ThesisConfidence,
					ThesisSupport:       opportunity.Signal.ThesisSupport,
					ThesisContradiction: opportunity.Signal.ThesisContradiction,
					ThesisConditions:    opportunity.Signal.ThesisConditions,
					Direction:           opportunity.Signal.Direction,
					Confidence:          opportunity.Signal.Confidence,
					AdmissionThreshold:  opportunity.Signal.AdmissionThreshold,
					Opportunity:         opportunity.Signal.Opportunity,
					OpportunityType:     opportunity.Signal.Type,
					PredictiveReady:     opportunity.Signal.PredictiveReady,
					PredictiveStatus:    opportunity.Signal.PredictiveStatus,
					Alternatives:        opportunity.Signal.Alternatives,
				}),
				Journal:  journal,
				Why:      opportunity.Why,
				Captured: opportunity.Captured, Missed: opportunity.Missed,
			})
		}

		symbols = append(symbols, &wire.HindsightSymbolT{
			Symbol: symbol.Symbol, UpboundPct: symbol.UpboundPct,
			RealizedPct: symbol.RealizedPct, MissedPct: symbol.MissedPct,
			Legs: int64(symbol.Legs), MissedLegs: int64(symbol.MissedLegs),
			Opportunities: opportunities,
		})
	}

	return &wire.HindsightFrameT{
		CaptureId: report.CaptureID, Status: report.Status, Symbols: symbols,
		MissedPct: report.MissedPct, UpboundPct: report.UpboundPct,
		MissedLegs: int64(report.MissedLegs), TotalLegs: int64(report.TotalLegs),
	}
}

/*
signalWireFields is the shared flat shape for one decision moment on the wire,
whether it is the signal pinned to a missed leg or one entry in its journal.
*/
type signalWireFields struct {
	At                  time.Time
	Action              string
	Reason              string
	Cause               string
	GraphScore          float64
	ThesisScore         float64
	ThesisConfidence    float64
	ThesisSupport       float64
	ThesisContradiction float64
	ThesisConditions    float64
	Direction           float64
	Confidence          float64
	AdmissionThreshold  float64
	Opportunity         bool
	OpportunityType     string
	PredictiveReady     bool
	PredictiveStatus    string
	Alternatives        map[string]float64
}

/*
hindsightSignalWire encodes one decision moment with its full thesis context.
*/
func hindsightSignalWire(fields signalWireFields) *wire.HindsightSignalT {
	return &wire.HindsightSignalT{
		At:                  fields.At.UnixNano(),
		Action:              fields.Action,
		Reason:              fields.Reason,
		Cause:               fields.Cause,
		GraphScore:          fields.GraphScore,
		ThesisScore:         fields.ThesisScore,
		ThesisConfidence:    fields.ThesisConfidence,
		ThesisSupport:       fields.ThesisSupport,
		ThesisContradiction: fields.ThesisContradiction,
		ThesisConditions:    fields.ThesisConditions,
		Direction:           fields.Direction,
		Confidence:          fields.Confidence,
		AdmissionThreshold:  fields.AdmissionThreshold,
		Opportunity:         fields.Opportunity,
		OpportunityType:     fields.OpportunityType,
		PredictiveReady:     fields.PredictiveReady,
		PredictiveStatus:    fields.PredictiveStatus,
		Alternatives:        hindsightNumbers(fields.Alternatives),
	}
}

func hindsightNumbers(values map[string]float64) []*wire.NamedNumberT {
	names := make([]string, 0, len(values))

	for name := range values {
		names = append(names, name)
	}

	sort.Strings(names)
	result := make([]*wire.NamedNumberT, 0, len(names))

	for _, name := range names {
		result = append(result, &wire.NamedNumberT{Name: name, Value: values[name]})
	}

	return result
}
