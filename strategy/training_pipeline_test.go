package strategy

import (
	"bytes"
	"fmt"
	"math"
	"strconv"
	"testing"
	"time"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning/associative"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
)

/*
tape builds one fragment: frames of readings from several producers moving
together, which is what the grid needs before it can form regions at all.

Producers move on a shared cycle with their own phase, so some quantities move
with each other and others against, which is the relationship the grid is
looking for. Without that they are noise and nothing forms.
*/
func tape(symbol string, frames int, phase float64) [][]*data.Measurement[float64] {
	// Where the record says this leg ignited and where it exhausted. A learner
	// is taught to recognise what ran into each, never to predict a price.
	ignition, extremum := int(float64(frames)*0.55), int(float64(frames)*0.80)

	// Three families on their own cycles. Which family carries the tape shifts
	// as the leg develops, so the regions that light up strongest are not the
	// same ones before ignition as during the run or into exhaustion. That
	// shift is the precursor: without it every moment shares one situation and
	// there is nothing to recognise.
	families := [][]string{
		{"pumpdump", "hawkes"},
		{"liquidity", "depthflow"},
		{"toxicity", "cvd"},
	}
	paces := []float64{1.0, 0.37, 0.13}
	fragment := make([][]*data.Measurement[float64], 0, frames)
	at := time.Unix(0, 0).UTC()

	for frame := range frames {
		now := moment(frame, ignition, extremum)
		readings := make([]*data.Measurement[float64], 0, 6)

		for family, sources := range families {
			for member, source := range sources {
				measurement := data.NewMeasurement[float64](
					fmt.Sprintf("%s-%d", source, frame), symbol, source,
					at.Add(time.Duration(frame)*time.Second), at,
				)
				measurement.Maturity = 1
				measurement.SNR = 4
				measurement.SNRDefined = true
				measurement.Provenance = map[string]string{
					"moment": now,
					"grade": strconv.FormatFloat(
						grade(frame, ignition, extremum, now), 'f', -1, 64,
					),
				}

				// One member of each family runs against the others, so the
				// grid has an inverse relationship to hold a family together.
				direction := 1.0

				if member == 1 {
					direction = -1
				}
				angle := float64(frame)*0.35*paces[family] + phase
				swing := direction * carried(family, now) * math.Sin(angle)
				measurement.Metrics["level"] = data.Metric[float64]{Raw: swing}
				measurement.Metrics["rate"] = data.Metric[float64]{
					Raw: direction * carried(family, now) * math.Cos(angle),
				}
				readings = append(readings, measurement)
			}
		}
		fragment = append(fragment, readings)
	}

	return fragment
}

/*
grade is how strongly the record stands behind the moment it put on one frame.

A call is worth what it captures. Entering is worth the whole leg still ahead of
it, so the strongest entry example is the frame ignition begins on and earlier
frames are worth proportionally less. Exiting is worth what it keeps, so the
strongest exit example is the extremum. Waiting is correct but weakly
informative — it says only that nothing was there — and grading it as highly as
an entry is what lets the moment merely seen most often win every situation the
two have in common.
*/
func grade(frame, ignition, extremum int, moment string) float64 {
	switch moment {
	case "enter":
		return 1 - float64(ignition-frame)/float64(ignition)
	case "exit":
		return 1 - float64(extremum-frame)/float64(extremum)
	case "hold":
		return float64(extremum-frame) / float64(extremum-ignition)
	}

	return 0.1
}

/*
carried is how hard one family moves at one moment of the leg. The ordering it
produces — which family is strongest — is what the agent is being asked to
recognise, and it is deliberately different at every moment.
*/
func carried(family int, moment string) float64 {
	weights := map[string][3]float64{
		"wait":  {1.0, 0.25, 0.2},
		"enter": {0.3, 1.0, 0.25},
		"hold":  {0.25, 0.4, 1.0},
		"exit":  {0.4, 0.2, 0.3},
	}

	return weights[moment][family]
}

/*
moment is what the record says one observation was. The leg running into
ignition is what entry recognition trains on, and the leg running into the
extremum is what exit recognition trains on.
*/
func moment(frame, ignition, extremum int) string {
	switch {
	case frame >= ignition-8 && frame < ignition:
		return "enter"
	case frame >= extremum-8 && frame < extremum:
		return "exit"
	case frame >= ignition && frame < extremum:
		return "hold"
	}

	return "wait"
}

/*
The learning path runs end to end: recorded frames enter the grid, the grid
forms regions, and every agent writes what it recognised into its own memory.
*/
func TestTrainingLearnsFromATape(t *testing.T) {
	fragments := [][][]*data.Measurement[float64]{
		tape("BTC/USD", 400, 0),
		tape("ETH/USD", 400, 1.1),
	}
	training := NewTrainingOver(fragments, 3)

	// One step is one frame, so the tape is replayed until formation is behind
	// it and whole legs have been seen since.
	for range 16000 {
		training.Step(nil)

		if err := training.Error(); err != nil {
			t.Fatalf("training failed: %v", err)
		}
	}

	if !training.Space().Formed {
		t.Fatal("expected the grid to have formed regions from the tape")
	}
	regions, _, err := training.Space().Regions("BTC/USD")

	if err != nil {
		t.Fatal(err)
	}

	if len(regions) == 0 {
		t.Fatal("expected the tape to have lit up at least one region")
	}
	t.Logf("grid formed %d columns into %d active regions", len(training.Space().Columns), len(regions))

	for index, region := range regions {
		t.Logf(
			"  region %d: id=%d members=%d strength=%.4f authority=%.4f",
			index, region.ID, region.Members, region.Strength, region.Authority,
		)
	}

	if len(training.Learners()) != 3 {
		t.Fatalf("expected three learners, received %d", len(training.Learners()))
	}

	// Every agent wrote what it recognised into its own memory.
	for index, memory := range training.Memories() {
		tree := core.To[*iradix.Tree[[]byte]](
			transport.NewApply(memory, nil).Next(nil),
		)

		if tree == nil || tree.Len() == 0 {
			t.Fatalf("agent %d learned nothing", index)
		}
		moments := map[string]int{}
		iterator := tree.Root().Iterator()
		iterator.SeekPrefix([]byte("b/"))

		for key, _, more := iterator.Next(); more; key, _, more = iterator.Next() {
			if class, _, named := bytes.Cut(key[2:], []byte("/")); named {
				moments[string(class)]++
			}
		}
		t.Logf("agent %d learned %d links across moments %v", index, tree.Len(), moments)

		for _, required := range []string{"enter", "exit", "hold", "wait"} {
			if moments[required] == 0 {
				t.Fatalf("agent %d learned nothing about %q: %v", index, required, moments)
			}
		}

		// Storing is not recognising. Shown a sequence it was taught ran into
		// ignition, the agent has to answer with entry rather than with the
		// moment it simply saw most often.
		recognised(t, index, tree)
	}
}

/*
recognised asks one agent about a sequence it was taught ran into ignition, and
about one it was taught ran into exhaustion, and checks it answers with the
moment rather than with whatever it saw most.
*/
func recognised(t *testing.T, agent int, tree *iradix.Tree[[]byte]) {
	t.Helper()
	for _, moment := range []string{"enter", "exit"} {
		opening := append([]byte("b/"), []byte(moment+"/")...)
		context := []byte(nil)
		iterator := tree.Root().Iterator()
		iterator.SeekPrefix(opening)

		if key, _, more := iterator.Next(); more {
			context = bytes.Clone(key[len(opening):])
		}

		if len(context) == 0 {
			t.Fatalf("agent %d stored no %q sequence to be asked about", agent, moment)
		}
		// The question is asked fresh each time. A retained cell alternates
		// between handing its value over and ending the run, so reading one
		// straight after writing it lands on whichever phase is next rather
		// than on the answer.
		recall := associative.NewRecall(transport.NewIO(core.From(tree)), transport.NewIO(
			core.From(cognition.Evaluation{
				Context: context, Config: cognition.DefaultConfig(), Step: 1 << 20,
			}),
		))
		answered := tests.Drain(t, recall, nil)
		tests.Sound(t, recall)

		if len(answered) == 0 {
			t.Fatalf("agent %d did not answer about %q", agent, moment)
		}
		reading := answered[len(answered)-1].(cognition.Evaluation)
		t.Logf(
			"  agent %d shown a %q sequence -> %q (confidence %.3f, contrast %.3f, support %d, runner-up %q)",
			agent, moment, reading.WinnerClass, reading.Confidence,
			reading.Contrast, reading.Support, reading.RunnerUp,
		)

		if reading.WinnerClass != moment {
			t.Fatalf(
				"agent %d shown a %q sequence answered %q",
				agent, moment, reading.WinnerClass,
			)
		}
	}
}

/*
What the dashboard is shown is what the learners actually hold: the grid they
were given, and for each of them how much it has learned, under which moments,
and what it answers when asked about a situation it has really seen.
*/
func TestTrainingReportsWhatItRecognised(t *testing.T) {
	fragments := [][][]*data.Measurement[float64]{
		tape("BTC/USD", 400, 0),
		tape("ETH/USD", 400, 1.1),
	}
	training := NewTrainingOver(fragments, 3)

	for range 16000 {
		training.Step(nil)
	}

	if err := training.Error(); err != nil {
		t.Fatalf("training failed: %v", err)
	}
	// Exactly the path the socket takes: the envelope carries the owner, and
	// the hub asks it to serialize when it is sending a learning frame.
	envelope := &types.Envelope{}
	training.Step(envelope)

	if envelope.Learning == nil {
		t.Fatal("expected the envelope to carry the learning owner")
	}
	encoded := envelope.Learning.MarshalFlatbuffer("")

	if len(encoded) == 0 {
		t.Fatal("expected the reading to serialize")
	}
	decoded := telemetry.GetRootAsLearningState(encoded, 0).UnPack()

	if decoded.Recognition == nil {
		t.Fatal("expected recognition in the encoded reading")
	}
	t.Logf(
		"status=%q fragments=%d frames=%d grid=%s formed=%v columns=%d regions=%d",
		decoded.Status, decoded.Recognition.Fragments, decoded.Recognition.Frames,
		decoded.Recognition.Grid.Symbol, decoded.Recognition.Grid.Formed,
		decoded.Recognition.Grid.Columns, len(decoded.Recognition.Grid.Regions),
	)

	if decoded.Status != "recognising precursors" {
		t.Fatalf("expected the learners to be recognising, received %q", decoded.Status)
	}

	// What the dashboard reads: the instruments, their quantities and the
	// regions those settled into. Without these every panel says "waiting".
	if len(decoded.Markets) == 0 {
		t.Fatal("expected the grid to be reported as instruments the dashboard can draw")
	}

	for _, market := range decoded.Markets {
		t.Logf(
			"  market %s: %d quantities, %d regions, depth %d",
			market.Symbol, len(market.Quantities), len(market.Regions), market.Depth,
		)

		if len(market.Quantities) == 0 {
			t.Fatalf("market %s reported no quantities to draw", market.Symbol)
		}
		drawn := 0

		for _, quantity := range market.Quantities {
			if quantity.Present {
				drawn++
			}

			if quantity.Source == "" || quantity.Label == "" {
				t.Fatalf("market %s reported an unnamed quantity", market.Symbol)
			}
		}

		if drawn == 0 {
			t.Fatalf("market %s reported no quantity as present", market.Symbol)
		}
	}

	if len(decoded.Agents) != 3 {
		t.Fatalf("expected three agents reported, received %d", len(decoded.Agents))
	}

	if len(decoded.Recognition.Learners) != 3 {
		t.Fatalf("expected three learners reported, received %d", len(decoded.Recognition.Learners))
	}

	for _, held := range decoded.Recognition.Learners {
		moments := make([]string, 0, len(held.Moments))

		for _, moment := range held.Moments {
			moments = append(moments, fmt.Sprintf("%s=%d", moment.Name, moment.Links))
		}
		t.Logf("  learner %d holds %d links across %v", held.Id, held.Links, moments)

		for _, answer := range held.Answers {
			t.Logf(
				"    asked about %q -> %q (confidence %.3f, contrast %.3f, support %d)",
				answer.Asked, answer.Answered, answer.Confidence, answer.Contrast, answer.Support,
			)
		}

		if held.Links == 0 {
			t.Fatalf("learner %d reported nothing learned", held.Id)
		}

		if len(held.Answers) == 0 {
			t.Fatalf("learner %d answered nothing", held.Id)
		}
	}
}
