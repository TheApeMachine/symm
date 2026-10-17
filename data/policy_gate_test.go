package data

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPolicyGate(t *testing.T) {
	Convey("Given a policy Value", t, func() {
		policy := &Value{Origin: PolicyOrigin, Role: "enter", Scope: PolicyScope, Epoch: 1792190000000000000, Tick: 1}
		policy.FormatPrefix()
		So(AppendPolicy(policy, "R01/R04", []float64{0.01, 30, 4, 0.01, 0.6}), ShouldBeNil)

		Convey("Without a gate layer it reads as an error", func() {
			_, err := ReadPolicyGate(policy)
			So(err, ShouldNotBeNil)
		})

		Convey("Its gate layer round-trips through the wire, tick epoch exact", func() {
			gate := PolicyGate{TickEpoch: 1792187257188073001, Enabled: true, SurpriseZ: 1.5, Warmup: PolicyWarmup{Rule: WarmupSettled, FastRate: 0.15, SlowRate: 0.01}, LearningRate: 0.01}
			So(AppendPolicyGate(policy, gate), ShouldBeNil)

			stored := New()
			_, err := stored.Write(policy.Bytes())
			So(err, ShouldBeNil)

			read, err := ReadPolicyGate(stored)
			So(err, ShouldBeNil)
			gate.Epoch = policy.Epoch
			So(read, ShouldResemble, gate)
			So(read.Same(PolicyGate{Enabled: true, SurpriseZ: 1.5, Warmup: PolicyWarmup{Rule: WarmupSettled, FastRate: 0.15, SlowRate: 0.01}, LearningRate: 0.01}), ShouldBeTrue)
			So(read.Same(PolicyGate{Enabled: false, SurpriseZ: 1.5, Warmup: PolicyWarmup{Rule: WarmupSettled, FastRate: 0.15, SlowRate: 0.01}, LearningRate: 0.01}), ShouldBeFalse)

			path, err := PolicyPath(stored)
			So(err, ShouldBeNil)
			So(path, ShouldEqual, "R01/R04")
			So(Metrics(stored)["gain"], ShouldEqual, 0.01)
		})

		Convey("A different warm-up rate makes a different gate", func() {
			gate := PolicyGate{Enabled: true, SurpriseZ: 1.5, Warmup: PolicyWarmup{Rule: WarmupSettled, FastRate: 0.15, SlowRate: 0.01}, LearningRate: 0.01}
			other := gate
			other.Warmup.SlowRate = 0.02
			So(gate.Same(other), ShouldBeFalse)
		})

		Convey("Only the settled warm-up is published", func() {
			gate := PolicyGate{TickEpoch: 3, Enabled: true, SurpriseZ: 1, Warmup: PolicyWarmup{Rule: WarmupTicksRule, Ticks: 300}, LearningRate: 0.01}
			So(AppendPolicyGate(policy, gate), ShouldNotBeNil)
		})

		Convey("A policy published under the legacy warmup_ticks layout reads as the ticks rule", func() {
			AppendVector(policy, []float64{1, 1, 300, 0.01},
				append(append([]string{}, PolicyLegacyGateKeys...), PolicyGateLayer, PolicyTicksTag+"1792189318815706000")...)

			stored := New()
			_, err := stored.Write(policy.Bytes())
			So(err, ShouldBeNil)

			read, err := ReadPolicyGate(stored)
			So(err, ShouldBeNil)
			So(read, ShouldResemble, PolicyGate{
				Epoch: policy.Epoch, TickEpoch: 1792189318815706000, Enabled: true, SurpriseZ: 1,
				Warmup: PolicyWarmup{Rule: WarmupTicksRule, Ticks: 300}, LearningRate: 0.01,
			})
			So(read.Warmup.String(), ShouldEqual, "after 300 ticks (legacy)")
			So(read.Same(PolicyGate{Enabled: true, SurpriseZ: 1, Warmup: PolicyWarmup{Rule: WarmupSettled, FastRate: 0.15, SlowRate: 0.01}, LearningRate: 0.01}), ShouldBeFalse)
		})
	})
}
