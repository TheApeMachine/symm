package data

import (
	"context"
	"strings"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
)

/* ExtendCutServer owns one immutable combined signal and logic observation. */
type ExtendCutServer struct {
	message *capnp.Message
	cut     MetricCut
}

func NewExtendCut() *ExtendCutServer { return &ExtendCutServer{} }

/* Write preserves every signal coordinate and appends only actually delivered logic. */
func (server *ExtendCutServer) Write(ctx context.Context, call ExtendCut_write) error {
	server.Shutdown()
	record, err := call.Args().Base()

	if err != nil {
		return errnie.Error(err)
	}

	if record.TypeId() != MetricCut_TypeID {
		return errnie.Error(errnie.Err(errnie.Validation, "extend cut: native metric cut required", nil))
	}
	pointer, err := record.Value()

	if err != nil {
		return errnie.Error(err)
	}
	base := MetricCut(pointer.Struct())

	if !base.IsValid() {
		return errnie.Error(errnie.Err(errnie.Validation, "extend cut: missing base", nil))
	}
	identities, err := call.Args().Identities()

	if err != nil {
		return errnie.Error(err)
	}
	values, err := call.Args().Values()

	if err != nil {
		return errnie.Error(err)
	}
	present, err := call.Args().Present()

	if err != nil {
		return errnie.Error(err)
	}

	if identities.Len() == 0 || values.Len() != present.Len() || values.Len() != 0 && values.Len() != identities.Len() {
		return errnie.Error(errnie.Err(errnie.Validation, "extend cut: declared logic identities must match values and presence", nil))
	}

	if err := server.prepare(base, identities.Len()); err != nil {
		return err
	}
	metrics, err := server.cut.Metrics()

	if err != nil {
		return errnie.Error(err)
	}
	offset := metrics.Len() - identities.Len()
	seen := make(map[string]bool, metrics.Len())
	for index := range offset {
		identity, err := metrics.At(index).Identity()

		if err != nil {
			return errnie.Error(err)
		}
		metric := metrics.At(index)
		if identity == "" || seen[identity] || base.Complete() && !metric.Present() {
			return errnie.Error(errnie.Err(errnie.Validation, "extend cut: inconsistent base coordinate set", nil))
		}
		if metric.Present() && (metric.Epoch() != base.Epoch() || metric.Sequence() < 0 || metric.Sequence() > base.Sequence()) {
			return errnie.Error(errnie.Err(errnie.Validation, "extend cut: base metric stamp is not causal", nil))
		}
		seen[identity] = true
	}
	for index := range identities.Len() {
		identity, err := identities.At(index)

		if err != nil {
			return errnie.Error(err)
		}

		if identity == "" || seen[identity] {
			return errnie.Error(errnie.Err(errnie.Validation, "extend cut: empty or duplicate logic identity", nil))
		}
		seen[identity] = true
		metric := metrics.At(offset + index)

		if err := metric.SetIdentity(identity); err != nil {
			return errnie.Error(err)
		}

		if index >= present.Len() || !present.At(index) {
			server.cut.SetComplete(false)
			continue
		}
		if call.Args().Epoch() != base.Epoch() || call.Args().Sequence() < 0 || call.Args().Sequence() > base.Sequence() {
			return errnie.Error(errnie.Err(errnie.Validation, "extend cut: logic stamp must be causal in the base epoch", nil))
		}
		metric.SetValue(values.At(index))
		metric.SetPresent(true)
		metric.SetEpoch(call.Args().Epoch())
		metric.SetSequence(call.Args().Sequence())
	}
	return nil
}

/* prepare copies one causal snapshot; it does not select or recompute evidence. */
func (server *ExtendCutServer) prepare(base MetricCut, additional int) error {
	prior, err := base.Metrics()

	if err != nil {
		return errnie.Error(err)
	}
	message, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))

	if err != nil {
		return errnie.Error(err)
	}
	server.message = message
	server.cut, err = NewRootMetricCut(segment)

	if err != nil {
		return errnie.Error(err)
	}
	symbol, err := base.Symbol()

	if err != nil {
		return errnie.Error(err)
	}
	provenance, err := base.Provenance()

	if err != nil {
		return errnie.Error(err)
	}

	if err := server.cut.SetSymbol(symbol); err != nil {
		return errnie.Error(err)
	}

	if err := server.cut.SetProvenance(provenance); err != nil {
		return errnie.Error(err)
	}
	server.cut.SetEpoch(base.Epoch())
	server.cut.SetSequence(base.Sequence())
	server.cut.SetComplete(base.Complete())
	metrics, err := server.cut.NewMetrics(int32(prior.Len() + additional))

	if err != nil {
		return errnie.Error(err)
	}
	for index := range prior.Len() {
		if err := metrics.Set(index, prior.At(index)); err != nil {
			return errnie.Error(err)
		}
	}
	return nil
}

/* Done emits a persisted row even while incomplete, but gates grid input on completeness. */
func (server *ExtendCutServer) Done(ctx context.Context, call ExtendCut_done) error {
	defer server.Shutdown()
	result, err := call.AllocResults()

	if err != nil {
		return errnie.Error(err)
	}
	result.SetIdle()

	if !server.cut.IsValid() {
		return nil
	}
	result.SetEpoch(server.cut.Epoch())
	result.SetSequence(server.cut.Sequence())
	symbol, err := server.cut.Symbol()

	if err != nil {
		return errnie.Error(err)
	}

	if err := result.SetScope(symbol); err != nil {
		return errnie.Error(err)
	}
	record, err := result.NewRow()

	if err != nil {
		return errnie.Error(err)
	}
	record.SetTypeId(MetricCut_TypeID)

	if err := record.SetValue(capnp.Struct(server.cut).ToPtr()); err != nil {
		return errnie.Error(err)
	}

	metrics, err := server.cut.Metrics()

	if err != nil {
		return errnie.Error(err)
	}
	labels, err := result.NewLabels(int32(metrics.Len()))

	if err != nil {
		return errnie.Error(err)
	}
	sources, err := result.NewSources(int32(metrics.Len()))

	if err != nil {
		return errnie.Error(err)
	}
	for index := range metrics.Len() {
		identity, err := metrics.At(index).Identity()

		if err != nil {
			return errnie.Error(err)
		}
		source, _, _ := strings.Cut(identity, ":")

		if err := labels.Set(index, identity); err != nil {
			return errnie.Error(err)
		}

		if err := sources.Set(index, source); err != nil {
			return errnie.Error(err)
		}
	}

	if !server.cut.Complete() {
		return errnie.Error(result.SetPhase("initializing"))
	}
	result.SetGathered()

	if err := result.SetPhase("ready"); err != nil {
		return errnie.Error(err)
	}
	values, err := result.Gathered().NewValues(int32(metrics.Len()))

	if err != nil {
		return errnie.Error(err)
	}
	present, err := result.Gathered().NewPresent(int32(metrics.Len()))

	if err != nil {
		return errnie.Error(err)
	}
	epochs, err := result.NewEpochs(int32(metrics.Len()))

	if err != nil {
		return errnie.Error(err)
	}
	sequences, err := result.NewSequences(int32(metrics.Len()))

	if err != nil {
		return errnie.Error(err)
	}
	for index := range metrics.Len() {
		metric := metrics.At(index)
		values.Set(index, metric.Value())
		present.Set(index, metric.Present())
		epochs.Set(index, metric.Epoch())
		sequences.Set(index, metric.Sequence())
	}
	return nil
}

func (server *ExtendCutServer) Shutdown() {
	if server.message != nil {
		server.message.Release()
	}
	server.message, server.cut = nil, MetricCut{}
}
