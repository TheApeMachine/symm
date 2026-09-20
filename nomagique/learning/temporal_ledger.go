package learning

import (
	context "context"
	"fmt"

	capnp "capnproto.org/go/capnp/v3"
)

/*
PendingReference records features and per-horizon predictions at one issue
sequence to resolve against future reference signals. The sequence is internal
to the ledger: callers hand it delayed observations without having to guarantee
consecutive or unique external step numbers.
*/
type PendingReference struct {
	Seq         int64
	Reference   float64
	Features    []float64
	Predictions []float64
	Horizon     int
	Resolved    int
}

/*
ResolutionOutcome reports the result of resolving one delayed horizon.
*/
type ResolutionOutcome struct {
	Horizon    int
	Prediction float64
	Target     float64
	Error      float64
	Step       int64
}

/*
IssueIntent records predictions and feature state for delayed evaluation.
Predictions holds one issued forecast per horizon, indexed by horizon minus
one; the caller retains its own authoritative sequence for the horizon
parameter, which is kept for outcome telemetry.
*/
type IssueIntent struct {
	Step        int64
	Reference   float64
	Features    []float64
	Predictions []float64
	Horizon     int
}

/*
ResolveIntent observes the current reference and supervises every pending
prediction against each horizon whose subsequent reference has arrived.
*/
type ResolveIntent struct {
	Step      int64
	Reference float64
}

/*
LedgerCommand discriminates one ledger operation. Exactly one intent must be
set; anything else is a shape failure.
*/
type LedgerCommand struct {
	Issue   *IssueIntent
	Resolve *ResolveIntent
}

/*
LedgerReading is the ledger's answer: the outcome of one resolution pass and
its retained counts.
*/
type LedgerReading struct {
	Outcome  *ResolutionOutcome
	Resolved int
	Total    int
	Pending  int
}

/*
TemporalLedger manages delayed target matching without any domain assumptions.
Issue and Resolve walk an internal monotonic sequence rather than the caller
supplied step, so a burst of observations sharing one external step number can
no longer overwrite an unresolved prediction before its reference arrives.

Every issued row is supervised against every horizon whose reference has
arrived: with the references retained per sequence, the row trains horizon h on
the cumulative move from its issue reference to the reference h steps later.
That nested supervision is what makes each task row an honest forecast for its
own horizon rather than a blend of several.
*/
type TemporalLedgerServer struct {
	DownstreamTemporalLedger func(context.Context, LedgerReading) error
	maxHorizon               int
	manifold                 *ResonanceManifoldServer
	coder                    *PredictiveCoderServer
	pending                  map[int64]*PendingReference
	references               map[int64]float64
	seq                      int64
	oldest                   int64
	resolved                 int
	total                    int
	last                     *ResolutionOutcome
	out                      LedgerReading
	err                      error
}

/*
NewTemporalLedgerServer constructs a temporal ledger primitive over the manifold it
supervises and the target primitive that maps reference pairs into supervised
targets. The manifold and the target must be supplied: an absent owner is a
shape failure, not a defaulted one.
*/
func NewTemporalLedgerServer(
	maxHorizon int,
	manifold *ResonanceManifoldServer,
	coder *PredictiveCoderServer,
) *TemporalLedgerServer {
	if maxHorizon <= 0 {
		return &TemporalLedgerServer{err: fmt.Errorf("ledger: horizon must be positive")}
	}

	if manifold == nil || coder == nil {
		return &TemporalLedgerServer{err: fmt.Errorf("ledger: requires a manifold and a coder")}
	}

	return &TemporalLedgerServer{
		maxHorizon: maxHorizon,
		manifold:   manifold,
		coder:      coder,
		pending:    make(map[int64]*PendingReference),
		references: make(map[int64]float64),
		oldest:     1,
	}
}

/*
Write receives a LedgerCommand via Cap'n Proto and returns the resulting reading.
*/
func (temporalLedger *TemporalLedgerServer) Write(ctx context.Context, call TemporalLedger_write) error {
	if temporalLedger.err != nil {
		return temporalLedger.err
	}

	args := call.Args()

	var cmd LedgerCommand
	if args.HasIssue() {
		issueArg, err := args.Issue()
		if err == nil {
			featuresList, _ := issueArg.Features()
			features := make([]float64, featuresList.Len())
			for i := 0; i < featuresList.Len(); i++ {
				features[i] = featuresList.At(i)
			}
			predictionsList, _ := issueArg.Predictions()
			predictions := make([]float64, predictionsList.Len())
			for i := 0; i < predictionsList.Len(); i++ {
				predictions[i] = predictionsList.At(i)
			}
			cmd.Issue = &IssueIntent{
				Step:        issueArg.Step(),
				Reference:   issueArg.Reference(),
				Features:    features,
				Predictions: predictions,
				Horizon:     int(issueArg.Horizon()),
			}
		}
	}

	if args.HasResolve() {
		resolveArg, err := args.Resolve()
		if err == nil {
			cmd.Resolve = &ResolveIntent{
				Step:      resolveArg.Step(),
				Reference: resolveArg.Reference(),
			}
		}
	}

	reading, err := temporalLedger.Execute(cmd)
	if err != nil {
		return err
	}

	if temporalLedger.DownstreamTemporalLedger != nil {
		return temporalLedger.DownstreamTemporalLedger(ctx, reading)
	}

	return nil
}

func (temporalLedger *TemporalLedgerServer) Done(ctx context.Context, call TemporalLedger_done) error {
	return nil
}

func (temporalLedger *TemporalLedgerServer) Execute(command LedgerCommand) (LedgerReading, error) {
	if temporalLedger.err != nil {
		return LedgerReading{}, temporalLedger.err
	}

	if (command.Issue == nil) == (command.Resolve == nil) {
		return LedgerReading{}, fmt.Errorf("ledger: command must set exactly one intent")
	}

	if command.Issue != nil {
		temporalLedger.issue(command.Issue)
	}

	if command.Resolve != nil {
		if err := temporalLedger.resolve(command.Resolve); err != nil {
			return LedgerReading{}, err
		}
	}

	temporalLedger.out = LedgerReading{
		Outcome:  temporalLedger.last,
		Resolved: temporalLedger.resolved,
		Total:    temporalLedger.total,
		Pending:  len(temporalLedger.pending),
	}

	return temporalLedger.out, nil
}

func (temporalLedger *TemporalLedgerServer) AsValue() func(LedgerCommand) LedgerReading {
	return func(cmd LedgerCommand) LedgerReading {
		reading, _ := temporalLedger.Execute(cmd)
		return reading
	}
}

func (temporalLedger *TemporalLedgerServer) Error() error {
	return temporalLedger.err
}

/*
issue records predictions and feature state for delayed evaluation. The ledger
assigns its own strictly increasing sequence so resolution order is
unambiguous.
*/
func (temporalLedger *TemporalLedgerServer) issue(intent *IssueIntent) {
	if intent.Reference <= 0 || len(intent.Features) == 0 {
		return
	}

	temporalLedger.seq++

	horizon := intent.Horizon

	if horizon < 1 {
		horizon = 1
	}

	if horizon > temporalLedger.maxHorizon {
		horizon = temporalLedger.maxHorizon
	}

	temporalLedger.pending[temporalLedger.seq] = &PendingReference{
		Seq:         temporalLedger.seq,
		Reference:   intent.Reference,
		Features:    append([]float64(nil), intent.Features...),
		Predictions: append([]float64(nil), intent.Predictions...),
		Horizon:     horizon,
	}
	temporalLedger.references[temporalLedger.seq] = intent.Reference
	temporalLedger.prune()
}

/*
resolve observes the current reference and supervises every pending prediction
against each horizon whose subsequent reference has arrived, in issue order.
A row issued at sequence s trains horizon h once the reference at s+h exists,
so one sample per horizon is generated per step regardless of how the external
step numbers jump or repeat. The outcome reports the row's own chosen horizon
once its delayed target arrives.
*/
func (temporalLedger *TemporalLedgerServer) resolve(intent *ResolveIntent) error {
	if intent.Reference <= 0 || temporalLedger.seq == 0 || temporalLedger.maxHorizon < 1 {
		return nil
	}

	refSeq := temporalLedger.seq + 1
	temporalLedger.references[refSeq] = intent.Reference

	var outcome *ResolutionOutcome

	for key := temporalLedger.oldest; key <= temporalLedger.seq; key++ {
		item, found := temporalLedger.pending[key]
		if !found {
			continue
		}

		// References are stored for every issued sequence; the row can be
		// supervised up to the horizon whose reference has already arrived.
		available := refSeq - item.Seq

		if available > int64(temporalLedger.maxHorizon) {
			available = int64(temporalLedger.maxHorizon)
		}

		if available <= int64(item.Resolved) {
			break
		}

		for horizon := item.Resolved + 1; horizon <= int(available); horizon++ {
			current, found := temporalLedger.references[item.Seq+int64(horizon)]

			if !found {
				break
			}

			target, err := temporalLedger.transform(current, item.Reference)

			if err != nil {
				return fmt.Errorf("ledger: resolve failed for horizon %d: %w", horizon, err)
			}

			prediction := 0.0

			if horizon-1 < len(item.Predictions) {
				prediction = item.Predictions[horizon-1]
			}

			if err := temporalLedger.observeTask(horizon, item.Features, prediction, target); err != nil {
				return fmt.Errorf("ledger: resolve failed for horizon %d: %w", horizon, err)
			}

			item.Resolved = horizon
			temporalLedger.total++

			if outcome == nil || horizon <= outcome.Horizon {
				outcome = &ResolutionOutcome{
					Horizon:    horizon,
					Prediction: prediction,
					Target:     target,
					Error:      target - prediction,
					Step:       intent.Step,
				}
				temporalLedger.last = outcome
			}
		}

		if item.Resolved >= temporalLedger.maxHorizon {
			delete(temporalLedger.pending, key)
			temporalLedger.resolved++
		}
	}

	for temporalLedger.oldest <= temporalLedger.seq {
		if _, found := temporalLedger.pending[temporalLedger.oldest]; found {
			break
		}
		temporalLedger.oldest++
	}

	return nil
}

/*
transform maps one resolved reference pair into its supervised target through
the configured target primitive.
*/
func (temporalLedger *TemporalLedgerServer) transform(current, past float64) (float64, error) {
	if temporalLedger.coder == nil {
		return 0, fmt.Errorf("ledger: coder is nil")
	}

	var val float64
	_, seg, _ := capnp.NewMessage(capnp.SingleSegment(nil))

	switch temporalLedger.coder.targetName {
	case "Directional":
		if temporalLedger.coder.directionalTarget == nil {
			return 0, fmt.Errorf("ledger: directional target nil")
		}
		temporalLedger.coder.directionalTarget.DownstreamDirectionalTarget = func(ctx context.Context, v float64) error { val = v; return nil }
		args, _ := NewDirectionalTarget_write_Params(seg)
		args.SetPast(past)
		args.SetCurrent(current)
		temporalLedger.coder.directionalTarget.WriteParams(context.Background(), args)
	case "Binary":
		if temporalLedger.coder.binaryTarget == nil {
			return 0, fmt.Errorf("ledger: binary target nil")
		}
		temporalLedger.coder.binaryTarget.DownstreamBinaryTarget = func(ctx context.Context, v float64) error { val = v; return nil }
		args, _ := NewBinaryTarget_write_Params(seg)
		args.SetPast(past)
		args.SetCurrent(current)
		temporalLedger.coder.binaryTarget.WriteParams(context.Background(), args)
	case "Identity":
		if temporalLedger.coder.identityTarget == nil {
			return 0, fmt.Errorf("ledger: identity target nil")
		}
		temporalLedger.coder.identityTarget.DownstreamIdentityTarget = func(ctx context.Context, v float64) error { val = v; return nil }
		args, _ := NewIdentityTarget_write_Params(seg)
		args.SetPast(past)
		args.SetCurrent(current)
		temporalLedger.coder.identityTarget.WriteParams(context.Background(), args)
	case "Delta":
		if temporalLedger.coder.deltaTarget == nil {
			return 0, fmt.Errorf("ledger: delta target nil")
		}
		temporalLedger.coder.deltaTarget.DownstreamDeltaTarget = func(ctx context.Context, v float64) error { val = v; return nil }
		args, _ := NewDeltaTarget_write_Params(seg)
		args.SetPast(past)
		args.SetCurrent(current)
		temporalLedger.coder.deltaTarget.WriteParams(context.Background(), args)
	case "Ratio":
		if temporalLedger.coder.ratioTarget == nil {
			return 0, fmt.Errorf("ledger: ratio target nil")
		}
		temporalLedger.coder.ratioTarget.DownstreamRatioTarget = func(ctx context.Context, v float64) error { val = v; return nil }
		args, _ := NewRatioTarget_write_Params(seg)
		args.SetPast(past)
		args.SetCurrent(current)
		temporalLedger.coder.ratioTarget.WriteParams(context.Background(), args)
	default:
		return 0, fmt.Errorf("ledger: unknown target name")
	}

	return val, nil
}

/*
observeTask forwards one supervised sample to the manifold's task head.
*/
func (temporalLedger *TemporalLedgerServer) observeTask(
	horizon int,
	features []float64,
	prediction float64,
	target float64,
) error {
	if temporalLedger.manifold == nil {
		return fmt.Errorf("ledger: manifold is nil")
	}
	return temporalLedger.manifold.observeTask(horizon, features, prediction, target)
}

func (temporalLedger *TemporalLedgerServer) prune() {
	if temporalLedger.seq <= int64(temporalLedger.maxHorizon) {
		return
	}

	purgeBelow := temporalLedger.seq - int64(temporalLedger.maxHorizon)
	for key := range temporalLedger.references {
		if key < purgeBelow {
			delete(temporalLedger.references, key)
		}
	}
}

func NewTemporalLedger() *TemporalLedgerServer {
	return &TemporalLedgerServer{}
}
