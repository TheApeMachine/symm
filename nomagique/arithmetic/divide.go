package arithmetic

import (
	"context"

	"github.com/theapemachine/errnie"
)

/*
DivideServer owns division.

A zero divisor is not an error here. The quotient is undefined, and saying so
is what division by zero means: the result carries that forward as infinity or
as not-a-number, and whichever metric depended on it reports undefined. Refusing
the call instead would abort the whole evaluation, so one undefined quotient
would erase every other metric measured from the same observation — including
all the ones that were perfectly well defined.
*/
type DivideServer struct {
	out float64
}

func (srv *DivideServer) Write(ctx context.Context, call Divide_write) error {
	srv.out = call.Args().A() / call.Args().B()
	return nil
}

func (srv *DivideServer) Done(ctx context.Context, call Divide_done) error {
	res, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"arithmetic: alloc divide results failed",
			err,
		))
	}

	res.SetOut(srv.out)
	srv.out = 0

	return nil
}

func NewDivide() *DivideServer {
	return &DivideServer{}
}
