package trend

import (
	"context"

	"github.com/cinar/indicator/v2/trend"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
EnvelopeServer calculates the Moving Average Envelope (upper, middle, lower).
*/
type EnvelopeServer struct {
	*runtime.System
	calculator *trend.Envelope[float64]
	close chan float64
	upperOut <-chan float64
	middleOut <-chan float64
	lowerOut <-chan float64
	upper float64
	middle float64
	lower float64
	count int
}

func NewEnvelope(ctx context.Context) *EnvelopeServer {
	close := make(chan float64, 1)
	calculator := trend.NewEnvelopeWithSma[float64]()

	server := &EnvelopeServer{
		System: runtime.NewSystem(ctx, "financial.trend.envelope"),
		calculator: calculator,
		close: close,
	}

	server.upperOut, server.middleOut, server.lowerOut = calculator.ComputeWithContext(ctx, close)

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *EnvelopeServer) Write(ctx context.Context, call Envelope_write) error {
	closeVal := call.Args().Close()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case server.close <- closeVal:
	}

	server.count++

	if server.count > server.calculator.IdlePeriod() {
		readFirst := false
		readSecond := false
		readThird := false

		for !readFirst || !readSecond || !readThird {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case res, ok := <-server.upperOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.trend.envelope.Write] upper channel closed",
						nil,
					))
				}

				server.upper = res
				readFirst = true
			case res, ok := <-server.middleOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.trend.envelope.Write] middle channel closed",
						nil,
					))
				}

				server.middle = res
				readSecond = true
			case res, ok := <-server.lowerOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.trend.envelope.Write] lower channel closed",
						nil,
					))
				}

				server.lower = res
				readThird = true
			}
		}
	}

	return nil
}

/*
Done returns calculated indicator results.
*/
func (server *EnvelopeServer) Done(ctx context.Context, call Envelope_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.trend.envelope.Done] failed to allocate done results",
			err,
		))
	}

	results.SetUpper(server.upper)
	results.SetMiddle(server.middle)
	results.SetLower(server.lower)
	return nil
}
