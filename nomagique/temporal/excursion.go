package temporal

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
ExcursionServer walks a path and reports the moves it actually made.

The path is read as alternating legs. A leg runs while the path continues its
way and closes when the path retraces a proportion of what that leg
travelled — never on the first step against it, which would cut every move
into noise.

The bar a leg has to clear is derived from the path's own behaviour: the
dispersion of its per-step log return, scaled over the horizon the way a
random walk scales. A path that barely moves has a low bar; a violent one has
a high bar; neither is compared against a number chosen in advance.
*/
type ExcursionServer struct {
	*runtime.System

	// The path, held as the three positions a move is made of rather than as
	// the steps between them. History is not retained: a leg is summarised by
	// where it began and how far it got.
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

	legs int64

	reported struct {
		anchor     float64
		ignition   float64
		extremum   float64
		excursion  float64
		qualifying float64
		confirmed  bool
		found      bool
	}
}

func NewExcursion(ctx context.Context) *ExcursionServer {
	server := &ExcursionServer{
		System: runtime.NewSystem(ctx, "temporal.excursion"),
		rising: true,
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write advances the path by one step.
*/
func (server *ExcursionServer) Write(ctx context.Context, call Excursion_write) error {
	args := call.Args()
	value := args.Value()

	// A path is walked as proportions of where it stands, so a step at or
	// below zero has no proportion to be read against.
	if value <= 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"temporal.excursion: a path is read in proportion to where it stands, so it cannot stand at or below zero",
			nil,
		))
	}

	horizon := int(args.Horizon())
	retrace := args.Retrace()

	if horizon < 1 || retrace <= 0 || retrace >= 1 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"temporal.excursion: a leg closes on a proportion of itself over a horizon of at least one step",
			nil,
		))
	}

	server.observe(value)

	if !server.opened {
		server.anchor, server.ignition, server.extremum = value, value, value
		server.opened = true
		return nil
	}

	qualifying := server.qualifying(args.Sigmas(), horizon, args.Floor())
	server.reported.qualifying = qualifying

	server.step(value, qualifying*retrace, qualifying)
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

	server.count++
	delta := change - server.mean
	server.mean += delta / server.count
	server.squares += delta * (change - server.mean)
}

/*
qualifying is how far a leg must travel to count as a move.
*/
func (server *ExcursionServer) qualifying(
	sigmas float64, horizon int, floor float64,
) float64 {
	if server.count < 2 {
		return floor
	}

	sigma := math.Sqrt(server.squares / server.count)
	derived := sigmas * sigma * math.Sqrt(float64(horizon))

	if derived > floor {
		return derived
	}

	return floor
}

/*
step carries the path one position further, closing the open leg when the
path has retraced enough of it to say the move is over.
*/
func (server *ExcursionServer) step(value, confirm, qualifying float64) {
	travelled := (value - server.extremum) / server.extremum

	if server.rising && travelled >= 0 {
		server.extremum = value
		return
	}

	if !server.rising && travelled <= 0 {
		server.extremum = value
		return
	}

	if math.Abs(travelled) < confirm {
		return
	}

	server.close(qualifying)

	server.anchor = server.ignition
	server.ignition = server.extremum
	server.extremum = value
	server.rising = !server.rising
}

/*
close reports the leg that just ended, if it travelled far enough to be a move
rather than the path milling about.
*/
func (server *ExcursionServer) close(qualifying float64) {
	server.legs++

	excursion := (server.extremum - server.ignition) / server.ignition

	if math.Abs(excursion) < qualifying {
		return
	}

	server.reported.anchor = server.anchor
	server.reported.ignition = server.ignition
	server.reported.extremum = server.extremum
	server.reported.excursion = excursion
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

	results.SetSteps(int64(server.count))
	results.SetLegs(server.legs)
	results.SetQualifying(server.reported.qualifying)

	if server.count >= 2 {
		results.SetSigma(math.Sqrt(server.squares / server.count))
	}

	results.SetFound(server.reported.found)

	// A move that was not completed is not a move. Reporting the open leg's
	// positions would hand back a beginning as though it were an outcome.
	if !server.reported.found {
		return nil
	}

	results.SetAnchor(server.reported.anchor)
	results.SetIgnition(server.reported.ignition)
	results.SetExtremum(server.reported.extremum)
	results.SetExcursion(server.reported.excursion)
	results.SetConfirmed(server.reported.confirmed)

	server.reported.found = false
	return nil
}
