package data

import (
	"context"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
MetricServiceServer builds one metric out of the forms a signal derived for it.

A metric carries the value that was measured and what that value is in. A
signal that derives a normalized or standardized form publishes it as a metric
of its own, so every number stays addressable rather than hidden inside
another.

Done emits the metric and clears it, because a metric describes one value.
*/
type MetricServiceServer struct {
	*runtime.System
	metric Metric
}

func NewMetricService(ctx context.Context) *MetricServiceServer {
	server := &MetricServiceServer{
		System: runtime.NewSystem(ctx, "data.metric"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write records the derived forms of one metric.
*/
func (server *MetricServiceServer) Write(ctx context.Context, call MetricService_write) error {
	args := call.Args()

	_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[data.metric.Write] failed to create message",
			err,
		))
	}

	metric, err := NewRootMetric(segment)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[data.metric.Write] failed to create metric",
			err,
		))
	}

	metric.SetRaw(args.Raw())
	metric.SetUnit(args.Unit())
	metric.SetTimescale(args.Timescale())

	server.metric = metric
	return nil
}

/*
Done emits the metric as an encoded value the graph can route onward.
*/
func (server *MetricServiceServer) Done(ctx context.Context, call MetricService_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[data.metric.Done] failed to allocate results",
			err,
		))
	}

	results.SetStatus(runtime.Status(server.Status()))

	if !server.metric.IsValid() {
		return nil
	}

	encoded, err := server.metric.Message().Marshal()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[data.metric.Done] failed to marshal metric",
			err,
		))
	}

	if err := results.SetRead(encoded); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[data.metric.Done] failed to set read",
			err,
		))
	}

	server.metric = Metric{}
	return nil
}
