package sequence

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
WindowServer retains the most recent readings of several quantities and hands
them back as one matrix.

The rows are arrivals and the columns are quantities, newest row last. Only
complete arrivals are retained: a reading missing a quantity cannot contribute
to that quantity's relationships, and retaining it with a zero in the gap
would assert that the quantity sat still when it was simply not published.
*/
type WindowServer struct {
	*runtime.System
	rows [][]float64
	cols int
	span int
}

func NewWindow(ctx context.Context) *WindowServer {
	server := &WindowServer{
		System: runtime.NewSystem(ctx, "sequence.window"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write retains one arrival.
*/
func (server *WindowServer) Write(ctx context.Context, call Window_write) error {
	span := int(call.Args().Span())

	if span < 2 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"sequence.window: a relationship cannot be read over fewer than two arrivals",
			nil,
		))
	}

	server.span = span

	values, err := call.Args().Value()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"sequence.window: failed to read the arriving values",
			err,
		))
	}

	if !values.IsValid() || values.Len() == 0 {
		return nil
	}

	present, err := call.Args().Present()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"sequence.window: failed to read which values arrived",
			err,
		))
	}

	if server.cols == 0 {
		server.cols = values.Len()
	}

	if values.Len() != server.cols {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"sequence.window: an arrival carries a different number of quantities than the ones before it",
			nil,
		))
	}

	row := make([]float64, server.cols)

	for index := range server.cols {
		// A quantity that did not arrive leaves the whole reading out rather
		// than standing in for itself with a zero.
		if present.IsValid() && index < present.Len() && !present.At(index) {
			return nil
		}

		row[index] = values.At(index)
	}

	server.rows = append(server.rows, row)

	// The oldest arrival leaves as the newest arrives, so the reading always
	// rests on the same amount of evidence once it is full.
	if len(server.rows) > server.span {
		server.rows = server.rows[len(server.rows)-server.span:]
	}

	return nil
}

/*
Done hands back what is retained, flattened row by row.
*/
func (server *WindowServer) Done(ctx context.Context, call Window_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"sequence.window: failed to allocate results",
			err,
		))
	}

	results.SetStatus(runtime.Status(server.Status()))
	results.SetRows(int32(len(server.rows)))
	results.SetCols(int32(server.cols))
	results.SetFull(server.span > 0 && len(server.rows) >= server.span)

	if len(server.rows) == 0 {
		return nil
	}

	held, err := results.NewOut(int32(len(server.rows) * server.cols))

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"sequence.window: failed to allocate the retained matrix",
			err,
		))
	}

	for index, row := range server.rows {
		for column, value := range row {
			held.Set(index*server.cols+column, value)
		}
	}

	return nil
}
