package data

import (
	"context"
	"fmt"
	"testing"

	capnp "capnproto.org/go/capnp/v3"
	. "github.com/smartystreets/goconvey/convey"
)

/* extendCutFixture models partial signal initialization and independent logic delivery. */
func extendCutFixture(params ExtendCut_write_Params, signals, logic int, baseComplete, logicComplete bool) error {
	params.SetEpoch(7)
	params.SetSequence(97)
	record, err := params.NewBase()
	if err != nil {
		return err
	}
	record.SetTypeId(MetricCut_TypeID)
	cut, err := NewMetricCut(params.Segment())
	if err != nil {
		return err
	}
	cut.SetEpoch(7)
	cut.SetSequence(99)
	cut.SetComplete(baseComplete)
	if err := cut.SetSymbol("BTC/USD"); err != nil {
		return err
	}
	if err := cut.SetProvenance("source capture identity"); err != nil {
		return err
	}
	metrics, err := cut.NewMetrics(int32(signals))
	if err != nil {
		return err
	}
	for index := range signals {
		metric := metrics.At(index)
		if err := metric.SetIdentity(fmt.Sprintf("signal:metric%d", index)); err != nil {
			return err
		}
		metric.SetValue(float64(index))
		metric.SetPresent(baseComplete || index == 0)
		metric.SetEpoch(7)
		metric.SetSequence(int64(index % 99))
	}
	if err := record.SetValue(capnp.Struct(cut).ToPtr()); err != nil {
		return err
	}
	identities, err := params.NewIdentities(int32(logic))
	if err != nil {
		return err
	}
	for index := range logic {
		if err := identities.Set(index, fmt.Sprintf("logic:metric%d", index)); err != nil {
			return err
		}
	}
	if !logicComplete {
		return nil
	}
	values, err := params.NewValues(int32(logic))
	if err != nil {
		return err
	}
	present, err := params.NewPresent(int32(logic))
	if err != nil {
		return err
	}
	for index := range logic {
		values.Set(index, -float64(index+1))
		present.Set(index, true)
	}
	return nil
}

func TestExtendCutWrite(t *testing.T) {
	Convey("The grid receives a cut only when signal and logic inputs are complete", t, func() {
		client := ExtendCut_ServerToClient(NewExtendCut())
		defer client.Release()
		ctx := context.Background()
		for _, state := range []struct{ signals, logic bool }{{false, false}, {false, true}, {true, true}, {true, false}, {true, true}} {
			So(client.Write(ctx, func(params ExtendCut_write_Params) error {
				return extendCutFixture(params, 2, 2, state.signals, state.logic)
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)
			future, release := client.Done(ctx, nil)
			result, err := future.Struct()
			So(err, ShouldBeNil)
			record, err := result.Row()
			So(err, ShouldBeNil)
			So(record.TypeId(), ShouldEqual, uint64(MetricCut_TypeID))
			pointer, err := record.Value()
			So(err, ShouldBeNil)
			cut := MetricCut(pointer.Struct())
			So(cut.Complete(), ShouldEqual, state.signals && state.logic)
			metrics, err := cut.Metrics()
			So(err, ShouldBeNil)
			So(metrics.Len(), ShouldEqual, 4)
			So(metrics.At(0).Value(), ShouldEqual, 0)
			So(metrics.At(0).Present(), ShouldBeTrue)
			So(metrics.At(0).Sequence(), ShouldEqual, 0)
			So(metrics.At(1).Sequence(), ShouldEqual, 1)
			So(metrics.At(2).Present(), ShouldEqual, state.logic)
			if state.logic {
				So(metrics.At(2).Sequence(), ShouldEqual, 97)
				So(metrics.At(2).Value(), ShouldEqual, -1)
			}
			if cut.Complete() {
				So(result.Which(), ShouldEqual, Gathered_Which_gathered)
				values, err := result.Gathered().Values()
				So(err, ShouldBeNil)
				So(values.Len(), ShouldEqual, 4)
			}
			if !cut.Complete() {
				So(result.Which(), ShouldEqual, Gathered_Which_idle)
			}
			release()
		}
	})
	Convey("Logic cannot overwrite a signal coordinate", t, func() {
		client := ExtendCut_ServerToClient(NewExtendCut())
		defer client.Release()
		So(client.Write(context.Background(), func(params ExtendCut_write_Params) error {
			if err := extendCutFixture(params, 2, 2, true, true); err != nil {
				return err
			}
			identities, err := params.Identities()
			if err != nil {
				return err
			}
			return identities.Set(0, "signal:metric0")
		}), ShouldBeNil)
		So(client.WaitStreaming(), ShouldNotBeNil)
	})
}

func TestExtendCutWriteCausality(t *testing.T) {
	Convey("Logic and signal stamps cannot point beyond the immutable base cut", t, func() {
		for _, invalid := range []string{"logic future", "logic epoch", "signal future", "missing complete coordinate", "duplicate signal identity"} {
			Convey(invalid, func() {
				client := ExtendCut_ServerToClient(NewExtendCut())
				defer client.Release()
				So(client.Write(context.Background(), func(params ExtendCut_write_Params) error {
					if err := extendCutFixture(params, 2, 2, true, true); err != nil {
						return err
					}
					record, err := params.Base()
					if err != nil {
						return err
					}
					pointer, err := record.Value()
					if err != nil {
						return err
					}
					metrics, err := MetricCut(pointer.Struct()).Metrics()
					if err != nil {
						return err
					}
					switch invalid {
					case "logic future":
						params.SetSequence(100)
					case "logic epoch":
						params.SetEpoch(8)
					case "signal future":
						metrics.At(0).SetSequence(100)
					case "missing complete coordinate":
						metrics.At(0).SetPresent(false)
					case "duplicate signal identity":
						return metrics.At(1).SetIdentity("signal:metric0")
					}
					return nil
				}), ShouldBeNil)
				So(client.WaitStreaming(), ShouldNotBeNil)
			})
		}
	})
}

func BenchmarkExtendCutWrite(b *testing.B) {
	client := ExtendCut_ServerToClient(NewExtendCut())
	defer client.Release()
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		// Shipping signal cut has 411 coordinates and Category declares 55 hypotheses.
		if err := client.Write(ctx, func(params ExtendCut_write_Params) error { return extendCutFixture(params, 411, 55, true, true) }); err != nil {
			b.Fatal(err)
		}
		if err := client.WaitStreaming(); err != nil {
			b.Fatal(err)
		}
		future, release := client.Done(ctx, nil)
		_, err := future.Struct()
		release()
		if err != nil {
			b.Fatal(err)
		}
	}
}
