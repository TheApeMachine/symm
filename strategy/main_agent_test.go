package strategy

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/cognition"
)


func TestMainAgent(t *testing.T) {
	Convey("Given a MainAgent initialized with shared cognition engine", t, func() {
		engine := cognition.NewEngine(cognition.Config{})
		initialCash := decimal.NewFromInt64(1000)
		mainAgent := NewMainAgent(initialCash, "simulated", engine)

		So(mainAgent, ShouldNotBeNil)
		So(mainAgent.IsHolding("BTC/USD"), ShouldBeFalse)

		now := time.Now().UTC()
		price := decimal.NewFromInt64(50000)

		Convey("When executing a profitable trade sequence with economic refinement (AT-23)", func() {
			entryContext := []byte{0x01, 0x02, 0x03, 0x04}
			exitContext := []byte{0x05, 0x06, 0x07, 0x08}

			enterDecision := ActionDecision{
				Action:     ActionEnter,
				Context:    entryContext,
				Confidence: 0.8,
				Contrast:   0.6,
				Support:    10,
			}

			// 1. Enter long position
			mainAgent.enterLong("BTC/USD", price, enterDecision, now)
			So(mainAgent.IsHolding("BTC/USD"), ShouldBeTrue)
			So(len(mainAgent.entryContexts["BTC/USD"]), ShouldBeGreaterThan, 0)

			// 2. Exit long at higher price (profitable trade)
			higherPrice := decimal.NewFromInt64(55000)
			exitDecision := ActionDecision{
				Action:     ActionExit,
				Context:    exitContext,
				Confidence: 0.9,
				Contrast:   0.7,
				Support:    12,
			}

			mainAgent.exitLong("BTC/USD", higherPrice, exitDecision, now.Add(time.Minute))
			So(mainAgent.IsHolding("BTC/USD"), ShouldBeFalse)
			So(len(mainAgent.outcomes), ShouldEqual, 1)
			So(mainAgent.outcomes[0].ReturnBp, ShouldBeGreaterThan, 0)

			// 3. Verify that cognitive memory was reinforced by economic feedback
			result, err := ask(engine, &cognition.Command{
				Evaluate: &cognition.Question{Context: entryContext},
			})
			So(err, ShouldBeNil)
			So(len(result.Evaluation.Candidates), ShouldBeGreaterThan, 0)
			So(result.Evaluation.Candidates[0].Name, ShouldEqual, string(ActionEnter))
		})

		Convey("When stepping with ActionHold while holding", func() {
			enterDecision := ActionDecision{
				Action:     ActionEnter,
				Context:    []byte{0x01, 0x02},
				Confidence: 0.8,
				Contrast:   0.5,
				Support:    5,
			}
			mainAgent.enterLong("BTC/USD", price, enterDecision, now)
			So(mainAgent.IsHolding("BTC/USD"), ShouldBeTrue)

			currentPrice := decimal.NewFromInt64(50500)

			holdDecision := ActionDecision{
				Action:     ActionHold,
				Context:    []byte{0x03, 0x04},
				Confidence: 0.8,
				Contrast:   0.5,
				Support:    5,
			}

			mainAgent.Step(currentPrice, "BTC/USD", holdDecision)
			So(mainAgent.IsHolding("BTC/USD"), ShouldBeTrue)

		})
	})
}
