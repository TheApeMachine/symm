package store

import (
	"context"
	"net/http/httptest"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/store/storetest"
)

func TestPolicyGates(t *testing.T) {
	Convey("Given policy epochs trained under different gates", t, func() {
		ctx := context.Background()
		backend := storetest.NewBackend()
		server := httptest.NewServer(backend)
		defer server.Close()
		backend.Buckets["symm"] = true

		publish := func(epoch uint64, action string, gate *data.PolicyGate) {
			value := &data.Value{Origin: data.PolicyOrigin, Role: action, Scope: data.PolicyScope, Epoch: epoch, Tick: 1}
			value.FormatPrefix()
			So(data.AppendPolicy(value, "R01/R04", []float64{0.01, 30, 4, 0.01, 0.6}), ShouldBeNil)

			if gate != nil {
				So(data.AppendPolicyGate(value, *gate), ShouldBeNil)
			}

			backend.Objects[value.Prefix] = value.Bytes()
		}

		ungated := data.PolicyGate{TickEpoch: 3, Enabled: false, SurpriseZ: 1, Warmup: data.PolicyWarmup{Rule: data.WarmupSettled, FastRate: 0.15, SlowRate: 0.01}, LearningRate: 0.01}
		gated := ungated
		gated.Enabled = true
		publish(20, "enter", &ungated)
		publish(20, "exit", &ungated)
		publish(30, "noop", &gated)

		reader, err := NewReader(ctx, "symm", storetest.Options(server.URL))
		So(err, ShouldBeNil)
		client := NewClient(ctx, "symm", storetest.Options(server.URL))

		Convey("Reader and Client read one gate per policy epoch, oldest first", func() {
			ungated.Epoch, gated.Epoch = 20, 30

			for _, read := range []func() ([]data.PolicyGate, error){reader.PolicyGates, func() ([]data.PolicyGate, error) { return PolicyGates(client) }} {
				gates, err := read()
				So(err, ShouldBeNil)
				So(gates, ShouldResemble, []data.PolicyGate{ungated, gated})
			}
		})

		Convey("A policy epoch without a gate tag is an error", func() {
			publish(40, "enter", nil)

			_, err := reader.PolicyGates()
			So(err, ShouldNotBeNil)
		})

		Convey("Reading never writes", func() {
			stored := len(backend.Objects)
			_, err := reader.PolicyGates()
			So(err, ShouldBeNil)
			So(backend.Objects, ShouldHaveLength, stored)
		})
	})
}
