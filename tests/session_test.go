package tests_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/signal/pumpdump"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/tests/conditions"
	tickerfixture "github.com/theapemachine/symm/tests/fixtures/ticker"
	"github.com/theapemachine/symm/types"
)

/*
pumpdumpSignals is the default Session factory for harness tests that need a
real quote signal without dragging GPU analyzer work into the paper path.
*/
func pumpdumpSignals(
	ctx context.Context,
	api *websocket.API,
	_ *broker.Instrument,
	channel chan []byte,
) []types.Signal {
	return []types.Signal{pumpdump.NewSignal(ctx, api, channel)}
}

func TestSessionPlay(t *testing.T) {
	Convey("Given a paper Session fed by ticker fixtures", t, func() {
		session, err := tests.NewSession(t, tests.SessionOptions{
			Signals: pumpdumpSignals,
		})
		So(err, ShouldBeNil)

		Convey("When a calm condition is played", func() {
			theses, err := session.Play(conditions.Calm(8).Frames())
			So(err, ShouldBeNil)

			Convey("Then Crypto.Tick advances with measurements", func() {
				So(len(theses), ShouldBeGreaterThan, 0)
				last := tests.Last(theses)
				So(last, ShouldNotBeNil)
				So(last.Tick, ShouldBeGreaterThan, 0)
				So(len(last.Measurements), ShouldBeGreaterThan, 0)
				So(session.Desk().OpenPositions(), ShouldEqual, 0)
			})
		})
	})
}

func TestSessionPumpLiftsRVOL(t *testing.T) {
	Convey("Given calm and pumped Session timelines", t, func() {
		calmSession, err := tests.NewSession(t, tests.SessionOptions{
			Signals: pumpdumpSignals,
		})
		So(err, ShouldBeNil)
		pumpSession, err := tests.NewSession(t, tests.SessionOptions{
			Signals: pumpdumpSignals,
		})
		So(err, ShouldBeNil)

		calmTheses, err := calmSession.Play(conditions.Calm(24).Frames())
		So(err, ShouldBeNil)
		pumpTheses, err := pumpSession.Play(
			conditions.Pump(24, 12, 1.25, 8).Frames(),
		)
		So(err, ShouldBeNil)

		Convey("When RVOL peaks are compared", func() {
			calm, hasCalm := tests.PeakMetric(
				calmTheses, "MATIC/USD", types.MetricRVOL,
			)
			pumped, hasPumped := tests.PeakMetric(
				pumpTheses, "MATIC/USD", types.MetricRVOL,
			)

			Convey("Then the pump condition lifts relative volume through Cut", func() {
				So(hasCalm, ShouldBeTrue)
				So(hasPumped, ShouldBeTrue)
				So(pumped, ShouldBeGreaterThan, calm)
			})
		})
	})
}

func TestSessionPaperSmoke(t *testing.T) {
	Convey("Given a paper Session on a pump condition", t, func() {
		session, err := tests.NewSession(t, tests.SessionOptions{
			Signals: pumpdumpSignals,
		})
		So(err, ShouldBeNil)
		So(session.Paper(), ShouldNotBeNil)
		So(session.Planner(), ShouldNotBeNil)

		Convey("When the pump timeline plays through Tick", func() {
			theses, err := session.Play(
				conditions.Pump(24, 12, 1.25, 8).Frames(),
			)
			So(err, ShouldBeNil)
			last := tests.Last(theses)

			Convey("Then the paper path records measured ticks without silent fills", func() {
				So(last, ShouldNotBeNil)
				So(len(last.Measurements), ShouldBeGreaterThan, 0)
				So(session.Desk().OpenPositions(), ShouldEqual, 0)
				// Decide is not yet on the Tick path; honest empty decisions
				// beat inventing hold/nothing without a reason pipeline.
				So(len(last.Decisions), ShouldEqual, 0)
				So(len(last.Positions), ShouldEqual, 0)
			})
		})
	})
}

func TestSessionConcurrentEmitTick(t *testing.T) {
	Convey("Given a paper Session under concurrent book/trade pressure", t, func() {
		session, err := tests.NewSession(t, tests.SessionOptions{
			Signals: pumpdumpSignals,
		})
		So(err, ShouldBeNil)

		frames := make([]tests.Frame, 0)

		for frame := range conditions.Pump(16, 8, 1.25, 6).Frames() {
			frames = append(frames, frame)
		}

		done := make(chan struct{})

		go func() {
			defer close(done)

			for _, frame := range frames {
				if frame.Channel == "ticker" {
					continue
				}

				session.Emit(frame)
			}
		}()

		Convey("When ticker frames Advance while books/trades Emit", func() {
			advanced := 0

			for _, frame := range frames {
				if frame.Channel != "ticker" {
					continue
				}

				thesis, tickErr := session.Advance(frame)
				So(tickErr, ShouldBeNil)

				if thesis == nil {
					continue
				}

				advanced++
				So(len(thesis.Measurements), ShouldBeGreaterThan, 0)
			}

			<-done

			Convey("Then measured ticks remain coherent without crash", func() {
				So(advanced, ShouldBeGreaterThan, 0)
				So(session.Desk().OpenPositions(), ShouldEqual, 0)
			})
		})
	})
}

func TestMockAPIWire(t *testing.T) {
	Convey("Given a MockAPI", t, func() {
		mock := tests.NewMockAPI()
		api, paper, err := mock.Wire(context.Background())
		So(err, ShouldBeNil)
		So(api, ShouldNotBeNil)
		So(paper, ShouldNotBeNil)
		t.Cleanup(api.Close)

		Convey("When a ticker fixture is emitted", func() {
			seen := 0
			api.On("ticker", func([]byte) { seen++ })
			for frame := range tickerfixture.NewFixture(tickerfixture.UPDATE, 3).Frames() {
				mock.Emit(frame)
			}

			Convey("Then handlers registered through API.On receive frames", func() {
				So(seen, ShouldEqual, 3)
			})
		})
	})
}

func BenchmarkSessionPlay(b *testing.B) {
	session, err := tests.NewSession(b, tests.SessionOptions{
		Signals: pumpdumpSignals,
	})

	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	for b.Loop() {
		if _, err := session.Play(conditions.Calm(16).Frames()); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSessionAdvance(b *testing.B) {
	session, err := tests.NewSession(b, tests.SessionOptions{
		Signals: pumpdumpSignals,
	})

	if err != nil {
		b.Fatal(err)
	}

	frames := make([]tests.Frame, 0, 16)

	for frame := range tickerfixture.NewFixture(tickerfixture.UPDATE, 16).Frames() {
		frames = append(frames, frame)
	}

	b.ReportAllocs()

	for b.Loop() {
		for _, frame := range frames {
			if _, err := session.Advance(frame); err != nil {
				b.Fatal(err)
			}
		}
	}
}
