package cmd

import (
	"context"
	"net/http/httptest"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/store/storetest"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/ui"
)

/*
TestStartPaperTrading: a model make run cannot trade (none stored, or one
trained under other gate settings) never aborts make run; paper trading
stays off (booter.sequitur is nil here, so starting it would panic) and the
problem goes out in the retained tree frame (its content: TestPublishTree).
*/
func TestStartPaperTrading(t *testing.T) {
	Convey("Given make run's dashboard booter over an in-memory bucket", t, func() {
		viper.SetConfigFile("cfg/config.yml")
		So(viper.ReadInConfig(), ShouldBeNil)

		saved := system.Cfg
		system.Cfg = system.NewConfig()
		defer func() { system.Cfg = saved }()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		backend := storetest.NewBackend()
		backend.Buckets["symm"] = true
		server := httptest.NewServer(backend)
		defer server.Close()

		reader, err := store.NewReader(ctx, "symm", storetest.Options(server.URL))
		So(err, ShouldBeNil)

		boot := func() error {
			tree, err := strategy.LoadCognitionTree(ctx, reader, system.Cfg.Learning.MinimumPathConfidence, system.Cfg.Resonance)
			So(err, ShouldBeNil)
			So(tree.Problem(), ShouldNotBeNil)

			booter := &Booter{dashboard: true, hub: ui.NewHub(ctx), tree: tree}
			return booter.startPaperTrading()
		}

		Convey("No stored model: make run goes on, paper trading inactive", func() {
			So(boot(), ShouldBeNil)
		})

		Convey("A model trained under other gate settings: make run goes on, paper trading inactive", func() {
			fast, slow := 0.15, system.Cfg.Resonance.LearningRate
			value := &data.Value{Origin: data.PolicyOrigin, Role: "enter", Scope: data.PolicyScope, Epoch: 7, Tick: 1}
			value.FormatPrefix()
			data.AppendVector(value, []float64{0.02, 30, 3, 0.02, 0.66}, append(append([]string{}, data.PolicyKeys...), data.PolicyPathTag+"R01/R04")...)
			So(data.AppendPolicyGate(value, data.PolicyGate{
				TickEpoch: 3, Enabled: !system.Cfg.Resonance.Gate, SurpriseZ: system.Cfg.Resonance.SurpriseZ,
				Warmup:       data.PolicyWarmup{Rule: data.WarmupSettled, FastRate: fast, SlowRate: slow},
				LearningRate: system.Cfg.Resonance.LearningRate,
			}), ShouldBeNil)
			backend.Objects[value.Prefix] = value.Bytes()

			So(boot(), ShouldBeNil)
		})
	})
}
