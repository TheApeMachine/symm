package data

import (
	"context"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
MeasurementServiceServer builds one measurement out of the metrics a signal
published for a single observation.

A signal graph routes the metrics it derived into this service, which carries
them together with the identity of what was observed and which signal observed
it. Done emits the measurement and clears it, because a measurement describes
one observation rather than an accumulation.
*/
type MeasurementServiceServer struct {
	*runtime.System
	measurement Measurement
	assigned    bool
}

func NewMeasurementService(ctx context.Context) *MeasurementServiceServer {
	server := &MeasurementServiceServer{
		System: runtime.NewSystem(ctx, "data.measurement"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write records the observation a signal measured and the metrics it published.
*/
func (server *MeasurementServiceServer) Write(ctx context.Context, call MeasurementService_write) error {
	args := call.Args()

	_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[data.measurement.Write] failed to create message",
			err,
		))
	}

	measurement, err := NewRootMeasurement(segment)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[data.measurement.Write] failed to create measurement",
			err,
		))
	}

	measurement.SetEpoch(args.Epoch())
	measurement.SetTick(args.Tick())
	measurement.SetTimestamp(args.Timestamp())
	measurement.SetEntity(args.Entity())
	measurement.SetSource(args.Source())
	measurement.SetSnr(args.Snr())
	measurement.SetMaturity(args.Maturity())
	measurement.SetSeparation(args.Separation())

	if err := server.identify(measurement, args); err != nil {
		return err
	}

	if err := server.carry(measurement, args); err != nil {
		return err
	}

	server.measurement = measurement
	server.assigned = true

	return nil
}

/*
Done emits the measurement as an encoded value the graph can route onward.
*/
func (server *MeasurementServiceServer) Done(ctx context.Context, call MeasurementService_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[data.measurement.Done] failed to allocate results",
			err,
		))
	}

	results.SetStatus(runtime.Status(server.Status()))

	if !server.assigned {
		return nil
	}

	encoded, err := server.measurement.Message().Marshal()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[data.measurement.Done] failed to marshal measurement",
			err,
		))
	}

	if err := results.SetRead(encoded); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[data.measurement.Done] failed to set read",
			err,
		))
	}

	server.measurement = Measurement{}
	server.assigned = false

	return nil
}

/*
identify carries what was observed, which is the identity a reader addresses
the measurement by.
*/
func (server *MeasurementServiceServer) identify(
	measurement Measurement,
	args MeasurementService_write_Params,
) error {
	identity, err := args.Id()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[data.measurement.identify] failed to read id argument",
			err,
		))
	}

	if err := measurement.SetId(identity); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[data.measurement.identify] failed to set id",
			err,
		))
	}

	label, err := args.Label()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[data.measurement.identify] failed to read label argument",
			err,
		))
	}

	if err := measurement.SetLabel(label); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[data.measurement.identify] failed to set label",
			err,
		))
	}

	return nil
}

/*
carry copies the published metrics and metadata into the measurement.
*/
func (server *MeasurementServiceServer) carry(
	measurement Measurement,
	args MeasurementService_write_Params,
) error {
	metrics, err := args.Metrics()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[data.measurement.carry] failed to read metrics argument",
			err,
		))
	}

	if metrics.IsValid() {
		if err := measurement.SetMetrics(metrics); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"[data.measurement.carry] failed to set metrics",
				err,
			))
		}
	}

	metadata, err := args.Metadata()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[data.measurement.carry] failed to read metadata argument",
			err,
		))
	}

	if !metadata.IsValid() {
		return nil
	}

	if err := measurement.SetMetadata(metadata); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[data.measurement.carry] failed to set metadata",
			err,
		))
	}

	return nil
}
