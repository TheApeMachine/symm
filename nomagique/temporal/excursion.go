package temporal

import (
	"context"
	"encoding/json"
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
ExcursionServer walks a path and reports the moves it actually made.

A reversal must exceed the measured random-walk scale over the open leg.
The characteristic span is the mean length of completed directional runs,
including the current run while no completed run exists. Qualifying uses the
root mean square magnitude of completed legs (mean and dispersion together).
Before any leg closes, the random-walk scale over the measured run span is used.
Both bounds are floored by the smallest observed nonzero log return.
All comparisons use log returns; the reported excursion remains a price ratio.
*/
type ExcursionServer struct {
	*runtime.System
	*excursionPath
	paths map[string]*excursionPath
}

/* excursionPath retains one market's sufficient statistics and open leg. */
type excursionPath struct {
	epoch            int64
	scope            string
	sequence         int64
	anchorSequence   int64
	ignitionSequence int64
	extremumSequence int64

	// The path, held as the three positions a move is made of rather than as
	// the steps between them. History is not retained: a leg is summarised by
	// where it began and how far it got.
	anchorIndex   int
	ignitionIndex int
	extremumIndex int
	observations  int

	anchor   float64
	ignition float64
	extremum float64
	rising   bool
	opened   bool

	// Welford over the log returns, which is what the qualifying bar is read
	// from. Retaining the returns themselves would keep a window that says
	// nothing the running moments do not.
	previous float64
	count    float64
	mean     float64
	squares  float64

	legs       int64
	legSteps   float64
	floor      float64
	direction  float64
	runSteps   float64
	runs       float64
	span       float64
	legSquares float64

	reported struct {
		anchorSequence       int64
		ignitionSequence     int64
		extremumSequence     int64
		confirmationSequence int64
		anchorIndex          int
		ignitionIndex        int
		extremumIndex        int
		anchor               float64
		ignition             float64
		extremum             float64
		excursion            float64
		qualifying           float64
		confirmed            bool
		found                bool
	}
}

func NewExcursion(ctx context.Context) *ExcursionServer {
	server := &ExcursionServer{
		System:        runtime.NewSystem(ctx, "temporal.excursion"),
		excursionPath: &excursionPath{rising: true},
		paths:         make(map[string]*excursionPath),
	}

	server.paths[""] = server.excursionPath
	server.Transition(runtime.READY)
	return server
}

/*
Write advances the path by one step.
*/
func (server *ExcursionServer) Write(ctx context.Context, call Excursion_write) error {
	args := call.Args()
	scope, err := args.Scope()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "excursion: scope", err))
	}
	if args.Epoch() < 0 || args.Sequence() < 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "excursion: invalid stamp", nil))
	}
	path, found := server.paths[scope]
	if !found || path.epoch != args.Epoch() {
		path = &excursionPath{rising: true, epoch: args.Epoch(), scope: scope}
		server.paths[scope] = path
	}
	server.excursionPath = path
	if server.opened && server.epoch > 0 && args.Sequence() <= server.sequence {
		return errnie.Error(errnie.Err(errnie.Validation, "excursion: sequence must advance", nil))
	}
	server.sequence = args.Sequence()
	return server.Step(args.Value())
}

/* Step advances the canonical excursion calculation with one positive observation. */
func (server *ExcursionServer) Step(value float64) error {

	// A path is walked as proportions of where it stands, so a step at or
	// below zero has no proportion to be read against.
	if value <= 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"temporal.excursion: a path is read in proportion to where it stands, so it cannot stand at or below zero",
			nil,
		))
	}

	if server.epoch == 0 {
		server.sequence = int64(server.observations)
	}
	server.observations++
	server.observe(value)

	if !server.opened {
		server.anchor, server.ignition, server.extremum = value, value, value
		server.anchorSequence, server.ignitionSequence, server.extremumSequence = server.sequence, server.sequence, server.sequence
		server.opened = true
		return nil
	}

	server.legSteps++
	qualifying := server.qualifying()
	server.reported.qualifying = qualifying

	confirm := math.Max(server.floor, server.sigma()*math.Sqrt(server.legSteps))
	server.step(value, confirm, qualifying)
	return nil
}

/*
observe folds one step into the dispersion the qualifying bar is read from.
*/
func (server *ExcursionServer) observe(value float64) {
	if server.previous <= 0 {
		server.previous = value
		return
	}

	change := math.Log(value / server.previous)
	server.previous = value

	if change != 0 {
		magnitude := math.Abs(change)

		if server.floor == 0 || magnitude < server.floor {
			server.floor = magnitude
		}

		direction := math.Copysign(1, change)

		if server.direction != 0 && direction != server.direction {
			server.runs++
			server.span += (server.runSteps - server.span) / server.runs
			server.runSteps = 0
		}

		server.direction = direction
	}

	server.runSteps++
	server.count++
	delta := change - server.mean
	server.mean += delta / server.count
	server.squares += delta * (change - server.mean)
}

/*
qualifying is how far a leg must travel to count as a move.
*/
func (server *ExcursionServer) qualifying() float64 {
	if server.legs > 0 {
		return math.Max(server.floor, math.Sqrt(server.legSquares/float64(server.legs)))
	}

	return math.Max(server.floor, server.sigma()*math.Sqrt(server.horizon()))
}

func (server *ExcursionServer) sigma() float64 {
	if server.count == 0 {
		return 0
	}

	return math.Sqrt(server.squares / server.count)
}

func (server *ExcursionServer) horizon() float64 {
	if server.runs == 0 {
		return server.runSteps
	}

	return server.span
}

/*
step carries the path one position further, closing the open leg when the
path has retraced enough of it to say the move is over.
*/
func (server *ExcursionServer) step(value, confirm, qualifying float64) {
	// The first move establishes direction; an initially falling path has
	// not completed an upward leg of zero length.
	if server.legs == 0 && server.extremum == server.ignition {
		server.rising = value >= server.ignition
	}

	travelled := math.Log(value / server.extremum)

	if server.rising && travelled >= 0 {
		server.extremum = value
		server.extremumIndex = server.observations - 1
		server.extremumSequence = server.sequence
		return
	}

	if !server.rising && travelled <= 0 {
		server.extremum = value
		server.extremumIndex = server.observations - 1
		server.extremumSequence = server.sequence
		return
	}

	if math.Abs(travelled) <= confirm {
		return
	}

	server.close(qualifying)

	server.anchorSequence = server.ignitionSequence
	server.ignitionSequence = server.extremumSequence
	server.anchorIndex = server.ignitionIndex
	server.ignitionIndex = server.extremumIndex
	server.extremumIndex = server.observations - 1
	server.extremumSequence = server.sequence
	server.anchor = server.ignition
	server.ignition = server.extremum
	server.extremum = value
	server.rising = !server.rising
	server.legSteps = 1
}

/*
close reports the leg that just ended, if it travelled far enough to be a move
rather than the path milling about.
*/
func (server *ExcursionServer) close(qualifying float64) {
	logExcursion := math.Log(server.extremum / server.ignition)
	server.legs++
	server.legSquares += logExcursion * logExcursion

	if math.Abs(logExcursion) <= qualifying {
		return
	}

	server.reported.anchorSequence = server.anchorSequence
	server.reported.ignitionSequence = server.ignitionSequence
	server.reported.extremumSequence = server.extremumSequence
	server.reported.confirmationSequence = server.sequence
	server.reported.anchorIndex = server.anchorIndex
	server.reported.ignitionIndex = server.ignitionIndex
	server.reported.extremumIndex = server.extremumIndex
	server.reported.anchor = server.anchor
	server.reported.ignition = server.ignition
	server.reported.extremum = server.extremum
	server.reported.excursion = (server.extremum - server.ignition) / server.ignition
	server.reported.confirmed = true
	server.reported.found = true
}

/*
Done reports the move the path last completed, and forgets it.

A completed move is reported once. Clearing it as the next step arrives would
lose a move made between two reads, and leaving it would report the same move
for every step that followed it.
*/
func (server *ExcursionServer) Done(ctx context.Context, call Excursion_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"temporal.excursion: failed to allocate results",
			err,
		))
	}

	results.SetEpoch(server.epoch)
	if err := results.SetScope(server.scope); err != nil {
		return errnie.Error(err)
	}
	results.SetSteps(int64(server.count))
	results.SetLegs(server.legs)
	results.SetQualifying(server.reported.qualifying)

	results.SetSigma(server.sigma())
	results.SetFloor(server.floor)
	results.SetHorizon(server.horizon())

	results.SetNone()

	// A move that was not completed is not a move. Reporting the open leg's
	// positions would hand back a beginning as though it were an outcome.
	if !server.reported.found {
		return nil
	}

	results.SetMove()
	move := results.Move()
	move.SetAnchor(server.reported.anchor)
	move.SetIgnition(server.reported.ignition)
	move.SetExtremum(server.reported.extremum)
	move.SetExcursion(server.reported.excursion)
	move.SetConfirmed(server.reported.confirmed)
	move.SetAnchorSequence(server.reported.anchorSequence)
	move.SetIgnitionSequence(server.reported.ignitionSequence)
	move.SetExtremumSequence(server.reported.extremumSequence)
	move.SetConfirmationSequence(server.reported.confirmationSequence)
	hasPrecursor := server.reported.anchorSequence < server.reported.ignitionSequence
	move.SetHasPrecursor(hasPrecursor)
	row, err := json.Marshal(struct {
		Epoch                int64   `json:"epoch"`
		HasPrecursor         bool    `json:"has_precursor"`
		Symbol               string  `json:"symbol"`
		AnchorSequence       int64   `json:"anchor_sequence"`
		IgnitionSequence     int64   `json:"ignition_sequence"`
		ExtremumSequence     int64   `json:"extremum_sequence"`
		ConfirmationSequence int64   `json:"confirmation_sequence"`
		Anchor               float64 `json:"anchor"`
		Ignition             float64 `json:"ignition"`
		Extremum             float64 `json:"extremum"`
		Excursion            float64 `json:"excursion"`
	}{server.epoch, hasPrecursor, server.scope, server.reported.anchorSequence, server.reported.ignitionSequence, server.reported.extremumSequence, server.reported.confirmationSequence, server.reported.anchor, server.reported.ignition, server.reported.extremum, server.reported.excursion})
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "excursion: encode fragment", err))
	}
	if err := move.SetRow(row); err != nil {
		return errnie.Error(err)
	}

	server.reported.found = false
	return nil
}
