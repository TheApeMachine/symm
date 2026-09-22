package momentum

import (
	"context"

	indicator "github.com/cinar/indicator/v2/momentum"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
IchimokuCloudServer calculates the Ichimoku Cloud.
*/
type IchimokuCloudServer struct {
	*runtime.System
	calculator *indicator.IchimokuCloud[float64]
	high chan float64
	low chan float64
	close chan float64
	conversionLineOut <-chan float64
	conversionLine float64
	baseLineOut <-chan float64
	baseLine float64
	leadingSpanAOut <-chan float64
	leadingSpanA float64
	leadingSpanBOut <-chan float64
	leadingSpanB float64
	laggingLineOut <-chan float64
	laggingLine float64
	count int
}

func NewIchimokuCloud(ctx context.Context) *IchimokuCloudServer {
	high := make(chan float64, 1)
	low := make(chan float64, 1)
	close := make(chan float64, 1)
	calculator := indicator.NewIchimokuCloud[float64]()

	server := &IchimokuCloudServer{
		System: runtime.NewSystem(ctx, "financial.momentum.ichimoku_cloud"),
		calculator: calculator,
		high: high,
		low: low,
		close: close,
	}

	server.conversionLineOut, server.baseLineOut, server.leadingSpanAOut, server.leadingSpanBOut, server.laggingLineOut = calculator.ComputeWithContext(ctx, high, low, close)

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *IchimokuCloudServer) Write(ctx context.Context, call IchimokuCloud_write) error {
	highVal := call.Args().High()
	lowVal := call.Args().Low()
	closeVal := call.Args().Close()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case server.high <- highVal:
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case server.low <- lowVal:
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case server.close <- closeVal:
	}

	server.count++

	if server.count > server.calculator.IdlePeriod() {
		readConversionLine := false
		readBaseLine := false
		readLeadingSpanA := false
		readLeadingSpanB := false
		readLaggingLine := false

		for !readConversionLine || !readBaseLine || !readLeadingSpanA || !readLeadingSpanB || !readLaggingLine {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case res, ok := <-server.conversionLineOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.momentum.ichimoku_cloud.Write] conversionLine channel closed",
						nil,
					))
				}

				server.conversionLine = res
				readConversionLine = true
			case res, ok := <-server.baseLineOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.momentum.ichimoku_cloud.Write] baseLine channel closed",
						nil,
					))
				}

				server.baseLine = res
				readBaseLine = true
			case res, ok := <-server.leadingSpanAOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.momentum.ichimoku_cloud.Write] leadingSpanA channel closed",
						nil,
					))
				}

				server.leadingSpanA = res
				readLeadingSpanA = true
			case res, ok := <-server.leadingSpanBOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.momentum.ichimoku_cloud.Write] leadingSpanB channel closed",
						nil,
					))
				}

				server.leadingSpanB = res
				readLeadingSpanB = true
			case res, ok := <-server.laggingLineOut:
				if !ok {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[financial.momentum.ichimoku_cloud.Write] laggingLine channel closed",
						nil,
					))
				}

				server.laggingLine = res
				readLaggingLine = true
			}
		}
	}

	return nil
}

/*
Done returns calculated indicator results.
*/
func (server *IchimokuCloudServer) Done(ctx context.Context, call IchimokuCloud_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.momentum.ichimoku_cloud.Done] failed to allocate done results",
			err,
		))
	}

	results.SetConversionLine(server.conversionLine)
	results.SetBaseLine(server.baseLine)
	results.SetLeadingSpanA(server.leadingSpanA)
	results.SetLeadingSpanB(server.leadingSpanB)
	results.SetLaggingLine(server.laggingLine)
	return nil
}
