package ui

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"runtime"
	"testing"

	"github.com/gofiber/fiber/v3"
	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/strategy"
)

/* learningBooks represents the legitimate startup state before the first book. */
type learningBooks struct{}

func (learningBooks) Book(_ string, read func(*spotbook.Book)) { read(nil) }

func TestHubSetLearner(t *testing.T) {
	Convey("Given a mounted learning owner and its durable journal", t, func() {
		ctx, cancel := context.WithCancel(t.Context())
		repository, err := store.NewSQLite(t.TempDir() + "/events.sqlite")
		So(err, ShouldBeNil)
		agent, err := strategy.NewAgent(ctx, learning.NewGrid(), learningBooks{},
			func(string) kraken.InstrumentPair { return kraken.InstrumentPair{} },
			func(string) *kraken.TradeVolumeFee { return nil }, decimal.NewFromInt64(200),
			func(hindsight.LearningEvent) error { return nil })
		So(err, ShouldBeNil)
		hub := &Hub{ctx: ctx, app: fiber.New(), store: repository}
		hub.SetLearner(agent, "test-run")
		done := make(chan struct{})
		go func() {
			defer close(done)
			for ctx.Err() == nil {
				agent.Step(nil)
				runtime.Gosched()
			}
		}()
		Reset(func() { cancel(); <-done; So(repository.Close(), ShouldBeNil) })

		Convey("The state endpoint exposes startup truth without fabricated wallets", func() {
			response, err := hub.app.Test(httptest.NewRequest("GET", "/learning", nil))
			So(err, ShouldBeNil)
			defer response.Body.Close()
			So(response.StatusCode, ShouldEqual, 200)
			var view strategy.LearningView
			So(json.NewDecoder(response.Body).Decode(&view), ShouldBeNil)
			So(view.Status, ShouldEqual, "waiting for market observations")
			So(view.Lanes, ShouldBeEmpty)
		})

		Convey("The journal endpoint returns only persisted events", func() {
			response, err := hub.app.Test(httptest.NewRequest("GET", "/learning/events?symbol=TEST%2FUSD", nil))
			So(err, ShouldBeNil)
			defer response.Body.Close()
			So(response.StatusCode, ShouldEqual, 200)
			var events []hindsight.LearningEvent
			So(json.NewDecoder(response.Body).Decode(&events), ShouldBeNil)
			So(events, ShouldBeEmpty)
		})
	})
}
