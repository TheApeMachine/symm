package strategy

import (
	"bytes"
	"context"
	"io"
	"net/http/httptest"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/store/storetest"
	"github.com/theapemachine/symm/system"
)

type writerFunc func(p []byte) (int, error)

func (write writerFunc) Write(p []byte) (int, error) { return write(p) }

func TestCoderState(t *testing.T) {
	Convey("Given a training that replayed the first half of the ticks and saved its coder state", t, func() {
		defer withGate(true, 0)()
		ctx := context.Background()
		ticks := featureTicks(400, "BTC/USD", "ETH/USD")

		backend := storetest.NewBackend()
		server := httptest.NewServer(backend)
		defer server.Close()
		backend.Buckets["symm"] = true
		reader, err := store.NewReader(ctx, "symm", storetest.Options(server.URL))
		So(err, ShouldBeNil)

		bucket := writerFunc(func(p []byte) (int, error) {
			value := data.New()

			if _, err := value.Write(p); err != nil {
				return 0, err
			}

			backend.Objects[value.Prefix] = bytes.Clone(p)
			return len(p), nil
		})

		publishPolicy := func(gate data.PolicyGate) {
			value := &data.Value{Origin: data.PolicyOrigin, Role: "enter", Scope: data.PolicyScope, Epoch: gate.Epoch, Tick: 1}
			value.FormatPrefix()
			So(data.AppendPolicy(value, "R01/R04", []float64{0.02, 30, 3, 0.02, 0.66}), ShouldBeNil)
			So(data.AppendPolicyGate(value, gate), ShouldBeNil)
			backend.Objects[value.Prefix] = value.Bytes()
		}

		trainGate, err := newChordGate(ctx, system.Cfg.Resonance)
		So(err, ShouldBeNil)
		replay := newTapeReplay(ctx, trainGate, nil)

		for _, value := range ticks[:200] {
			So(replay.observe(value), ShouldBeNil)
		}

		settings := trainGate.settings
		settings.Epoch, settings.TickEpoch = 9, 3
		So(trainGate.save(bucket, settings, 200, replay.tapes), ShouldBeNil)
		publishPolicy(settings)

		readerSource := stateSource{keys: reader.Keys, download: reader.Download}

		Convey("each symbol's state records the predictions it dropped, and its gate", func() {
			states, err := policyCoderStates(readerSource, settings)
			So(err, ShouldBeNil)
			So(states, ShouldHaveLength, 2)

			for _, state := range states {
				So(state.Fields["pending_dropped"], ShouldBeGreaterThan, 0)
				So(state.Gate.Same(settings), ShouldBeTrue)
				So(state.Gate.Epoch, ShouldEqual, 9)

				moments, err := momentsOf(state, settings)
				So(err, ShouldBeNil)
				So(moments, ShouldResemble, replay.tapes[state.Symbol].surprise)
			}
		})

		Convey("make run continuing that policy epoch gates every later tick exactly as the training does", func() {
			tree, err := LoadCognitionTree(ctx, reader, 2, system.Cfg.Resonance)
			So(err, ShouldBeNil)
			So(tree.Problem(), ShouldBeNil)
			So(tree.seeded, ShouldNotBeNil)

			sequitur, err := NewSequitur(ctx, &frameSink{}, tree)
			So(err, ShouldBeNil)

			cold, err := NewSequitur(ctx, &frameSink{}, testTree(map[string]string{}))
			So(err, ShouldBeNil)
			differs := false

			for _, value := range ticks[200:] {
				So(replay.observe(value), ShouldBeNil)
				_, err := io.Copy(sequitur, value)
				So(err, ShouldBeNil)
				_, err = io.Copy(cold, value)
				So(err, ShouldBeNil)

				for _, symbol := range replay.order {
					So(sequitur.tapes[symbol].surprise, ShouldResemble, replay.tapes[symbol].surprise)
					differs = differs || cold.tapes[symbol].surprise != replay.tapes[symbol].surprise
				}
			}

			// Without the state, run's gate would have started cold.
			So(differs, ShouldBeTrue)

			for _, symbol := range replay.order {
				trained, _, err := trainGate.solver.SaveSymbol(symbol)
				So(err, ShouldBeNil)
				live, _, err := sequitur.gate.solver.SaveSymbol(symbol)
				So(err, ShouldBeNil)
				So(live, ShouldResemble, trained)
			}
		})

		Convey("the next training continues the newest state trained under its settings", func() {
			states, epoch, skipped, err := newestCoderStates(readerSource, settings)
			So(err, ShouldBeNil)
			So(epoch, ShouldEqual, 9)
			So(skipped, ShouldEqual, 0)
			So(states, ShouldHaveLength, 2)

			other := settings
			other.SurpriseZ = settings.SurpriseZ + 1
			states, epoch, skipped, err = newestCoderStates(readerSource, other)
			So(err, ShouldBeNil)
			So(states, ShouldBeNil)
			So(epoch, ShouldEqual, 0)
			So(skipped, ShouldEqual, 1)
		})

		// unusable is make run's view of a newer policy epoch it cannot
		// continue: no error, no model, an error banner naming the fix.
		unusable := func() *CognitionProblem {
			tree, err := LoadCognitionTree(ctx, reader, 2, system.Cfg.Resonance)
			So(err, ShouldBeNil)
			So(tree.Empty(), ShouldBeTrue)
			So(tree.seeded, ShouldBeNil)
			So(tree.Problem(), ShouldNotBeNil)
			So(tree.Problem().Level, ShouldEqual, ProblemError)
			So(tree.Problem().Fix, ShouldEqual, "re-run detect and train on the tick epoch 3")
			return tree.Problem()
		}

		newer := settings
		newer.Epoch = 11

		Convey("a policy epoch with no coder state is an error problem", func() {
			publishPolicy(newer)
			So(unusable().Reason, ShouldContainSubstring, "policy epoch 11 stores no coder state")
		})

		Convey("a coder state that does not decode is an error problem", func() {
			publishPolicy(newer)
			backend.Objects[data.Prefix(data.CoderOrigin, data.CoderStateRole, "BTC/USD", 11)+"0000000000000000200.bin"] = []byte("garbage")
			So(unusable().Reason, ShouldContainSubstring, "policy epoch 11's coder state is unusable")
		})

		Convey("a coder state trained under other settings than its policies is an error problem", func() {
			publishPolicy(newer)
			other := newer
			other.SurpriseZ = newer.SurpriseZ + 1

			for _, symbol := range replay.order {
				blob, _, err := trainGate.solver.SaveSymbol(symbol)
				So(err, ShouldBeNil)
				value := data.NewValue(data.CoderOrigin, data.CoderStateRole, symbol)
				value.Epoch, value.Tick = 11, 200
				value.FormatPrefix()
				So(data.AppendCoderState(value, momentsVector(replay.tapes[symbol].surprise, 0), blob, other), ShouldBeNil)
				backend.Objects[value.Prefix] = value.Bytes()
			}

			So(unusable().Reason, ShouldContainSubstring, "was trained under")
		})

		Convey("a store that becomes unreachable stays fatal", func() {
			server.Close()
			_, err := LoadCognitionTree(ctx, reader, 2, system.Cfg.Resonance)
			So(err, ShouldNotBeNil)
		})
	})
}
