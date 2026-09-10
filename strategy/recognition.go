package strategy

import (
	"bytes"
	"encoding/binary"
	"strconv"
	"time"

	flatbuffers "github.com/google/flatbuffers/go"
	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning/associative"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/telemetry/generated/telemetry"
)

/*
Recognition is what the learners have made of the tape so far, in the shape the
dashboard already reads.

The wallet fields the old learner filled stay empty because there is no wallet:
a precursor learner holds no position and pays no fee, and writing a number into
those fields would be describing something that does not exist. What is filled
is what genuinely exists — the grid every learner is being shown, and what each
of them has committed to memory.
*/
type Recognition struct {
	state *telemetry.LearningStateT
}

/* MarshalFlatbuffer serializes the reading for the dashboard socket. */
func (recognition *Recognition) MarshalFlatbuffer(string) []byte {
	if recognition == nil || recognition.state == nil {
		return nil
	}
	builder := flatbuffers.NewBuilder(1024)
	builder.Finish(recognition.state.Pack(builder))

	return builder.FinishedBytes()
}

/*
MarshalFlatbuffer reads the learners only when the dashboard is actually taking
a frame.

The ring steps every envelope and the socket sends learning far less often, so
building the reading on every step would walk every learner's whole memory and
recall against it hundreds of times for each frame that survives. The envelope
carries the owner; the reading is taken when it is wanted.
*/
func (training *Training) MarshalFlatbuffer(focus string) []byte {
	return training.State().MarshalFlatbuffer(focus)
}

/*
State reads what the grid is showing and what every learner currently holds,
without disturbing any of them.
*/
func (training *Training) State() *Recognition {
	at := time.Now().UTC().UnixNano()
	held := training.mounted.Load()

	// The record has not been read yet. Saying so is the reading: an unmounted
	// tape is not a tape with nothing on it.
	if held == nil {
		return &Recognition{state: &telemetry.LearningStateT{
			AtNs: at, Status: "reading the record",
		}}
	}

	return held.state(at, training.frames.Load())
}

/* state is what one mounted tape and its learners currently amount to. */
func (held *replay) state(at int64, frames uint64) *Recognition {
	state := &telemetry.LearningStateT{
		AtNs:     at,
		Steps:    frames,
		Status:   held.status(),
		Restored: held.space.Formed,
		Markets:  held.markets(at),
	}

	// Each memory is read exactly once. Reading it twice would take the store
	// through two delivery phases, and the second read would find nothing.
	learners := make([]*telemetry.LearningLearnerT, 0, len(held.memories))

	for index, memory := range held.memories {
		held := learner(index, memory)
		state.Agents = append(state.Agents, held.agent)
		state.Decisions += uint64(held.links)
		learners = append(learners, held.held)
	}
	state.Recognition = held.recognised(at, frames, learners)
	state.Rehearsal = held.rehearsal(frames, learners)

	return &Recognition{state: state}
}

/*
rehearsal restores the per-learner tape lanes the dashboard draws: one track
per learner, the mounted fragment it is being shown, and one arrow for each
precursor answer its memory currently holds. The new pipeline has no wallet,
so the grading fields stay at zero and only the lanes that genuinely exist
are populated.
*/
func (held *replay) rehearsal(
	frames uint64, learners []*telemetry.LearningLearnerT,
) *telemetry.LearningRehearsalT {
	rehearsal := &telemetry.LearningRehearsalT{
		Status:       held.status(),
		Workers:      int32(len(learners)),
		Episodes:     uint64(held.fragments),
		Trained:      uint64(held.fragments),
		Observations: frames,
		Budget:       uint64(max(held.fragments, 1)),
		Runs:         1,
		Tracks:       make([]*telemetry.LearningTrackT, 0, len(learners)),
	}
	steps, symbol, entry, exit := held.tapeSteps()

	for index, learner := range learners {
		rehearsal.Tracks = append(
			rehearsal.Tracks, learnerTrack(index, learner, steps, symbol, entry, exit, frames),
		)
	}

	return rehearsal
}

/* tapeSteps projects the first mounted fragment into one scalar per frame. */
func (held *replay) tapeSteps() (
	[]*telemetry.LearningStepT, string, int32, int32,
) {
	if len(held.tape) == 0 {
		return nil, "", -1, -1
	}
	leg := held.tape[0]
	steps := make([]*telemetry.LearningStepT, 0, len(leg))
	symbol := ""
	entry, exit := int32(-1), int32(-1)

	for index, frame := range leg {
		value, defined := frameValue(frame)

		if symbol == "" && len(frame) > 0 {
			symbol = frame[0].Label
		}
		moment := frameMoment(frame)

		if moment == "enter" && entry < 0 {
			entry = int32(index)
		}

		if moment == "exit" {
			exit = int32(index)
		}
		at := int64(0)

		if len(frame) > 0 && !frame[0].At.IsZero() {
			at = frame[0].At.UnixNano()
		}
		steps = append(steps, &telemetry.LearningStepT{
			AtNs: at, Value: value, Defined: defined,
		})
	}

	return steps, symbol, entry, exit
}

/* frameValue picks the most legible scalar a frame carries. */
func frameValue(frame []*data.Measurement[float64]) (float64, bool) {
	for _, measurement := range frame {
		if measurement == nil || len(measurement.Metrics) == 0 {
			continue
		}

		for _, key := range []string{"level", "rate", "value", "raw"} {
			if metric, ok := measurement.Metrics[key]; ok {
				return metric.Raw, true
			}
		}

		for _, metric := range measurement.Metrics {
			return metric.Raw, true
		}
	}

	return 0, false
}

/* frameMoment reads the moment the tape named this frame, if it named one. */
func frameMoment(frame []*data.Measurement[float64]) string {
	for _, measurement := range frame {
		if measurement == nil {
			continue
		}

		if moment := measurement.Provenance["moment"]; moment != "" {
			return moment
		}
	}

	return ""
}

/* learnerTrack builds one lane and the arrows that learner's memory holds. */
func learnerTrack(
	index int,
	learner *telemetry.LearningLearnerT,
	steps []*telemetry.LearningStepT,
	symbol string,
	entry, exit int32,
	frames uint64,
) *telemetry.LearningTrackT {
	length := int32(len(steps))
	playhead := int32(0)

	if length > 0 {
		playhead = int32(frames % uint64(length))
	}
	marks := make([]*telemetry.LearningMarkT, 0, len(learner.Answers))

	for mark, answer := range learner.Answers {
		marks = append(marks, &telemetry.LearningMarkT{
			Id:      uint64(mark),
			Index:   momentIndex(answer.Asked, entry, exit, length),
			Kind:    answer.Answered,
			Value:   answer.Confidence,
			Graded:  answer.Confidence > 0,
			Verdict: answer.Answered,
		})
	}

	return &telemetry.LearningTrackT{
		Id:     int32(index),
		Symbol: symbol,
		Index:  playhead,
		Length: length,
		Stride: 1,
		Steps:  steps,
		Marks:  marks,
		Entry:  entry,
		Exit:   exit,
		Queued: max(length-1, 0),
	}
}

/* momentIndex places a precursor answer on the lane it describes. */
func momentIndex(moment string, entry, exit, length int32) int32 {
	if length <= 0 {
		return 0
	}

	switch moment {
	case "enter":
		if entry >= 0 {
			return entry
		}

		return length / 3
	case "exit":
		if exit >= 0 {
			return exit
		}

		return 2 * length / 3
	case "hold":
		return length / 2
	}

	return 0
}

/* status is what the learning path is waiting on, in its own words. */
func (held *replay) status() string {
	if held.fragments == 0 {
		return "no tape"
	}

	if !held.space.Formed {
		return "forming the impulse map"
	}

	return "recognising precursors"
}

/*
markets is the grid as the learners are being shown it: one entry per instrument
the tape carries, with every quantity that instrument published and the regions
those quantities settled into.
*/
func (held *replay) markets(at int64) []*telemetry.LearningDevelopmentT {
	space := held.space
	markets := make([]*telemetry.LearningDevelopmentT, 0, len(space.Rows))

	for row, symbol := range space.Rows {
		development := &telemetry.LearningDevelopmentT{
			Symbol: symbol,
			AtNs:   at,
			FromNs: at,
			Status: held.status(),
		}
		activity, quality, err := space.Activity(symbol)

		if err != nil {
			continue
		}

		for column, identity := range space.Columns {
			coordinate := space.Coordinates[column]
			development.Quantities = append(
				development.Quantities, &telemetry.LearningQuantityT{
					Source:   identity[0],
					Label:    identity[1],
					X:        coordinate[0],
					Y:        coordinate[1],
					Value:    space.Values[row][column],
					Activity: activity[column],
					Quality:  quality[column],
					Present:  space.Present[row][column],
				},
			)
		}
		regions, version, err := space.Regions(symbol)

		if err != nil {
			continue
		}
		development.Decisions = version

		for _, region := range regions {
			development.Regions = append(development.Regions, &telemetry.LearningRegionT{
				Id:        region.ID,
				Condition: region.Condition,
				Level:     region.Level,
				Change:    region.Change,
				Strength:  region.Strength,
				Authority: region.Authority,
				Members:   int32(region.Members),
			})
			development.Context = append(
				development.Context, strconv.FormatUint(region.Condition, 16),
			)
		}
		development.Depth = int32(len(development.Context))
		markets = append(markets, development)
	}

	return markets
}

/* recognised is the precursor view: the grid, and what each learner holds. */
func (held *replay) recognised(
	at int64, frames uint64, learners []*telemetry.LearningLearnerT,
) *telemetry.LearningRecognitionT {
	return &telemetry.LearningRecognitionT{
		AtNs:      at,
		Fragments: int32(held.fragments),
		Frames:    frames,
		Grid:      held.layout(),
		Learners:  learners,
	}
}

/* layout is the settled map, reported once rather than per instrument. */
func (held *replay) layout() *telemetry.LearningGridT {
	space := held.space
	symbol := space.UpdatedLabel
	layout := &telemetry.LearningGridT{
		Symbol:  symbol,
		Formed:  space.Formed,
		Columns: int32(len(space.Columns)),
		Version: space.Version,
	}

	if symbol == "" {
		return layout
	}
	regions, version, err := space.Regions(symbol)

	if err != nil {
		return layout
	}
	layout.Version = version

	for _, region := range regions {
		layout.Regions = append(layout.Regions, &telemetry.LearningActiveT{
			Id:        region.ID,
			Condition: region.Condition,
			Strength:  region.Strength,
			Authority: region.Authority,
			Members:   int32(region.Members),
		})
	}

	return layout
}

/* reading is one learner in both shapes the dashboard reads it in. */
type reading struct {
	agent *telemetry.LearningAgentT
	held  *telemetry.LearningLearnerT
	links int32
}

/*
learner reads one memory: how much it holds, what moments it holds it under, and
what it answers when shown one situation from each of them.

The questions are the learner's own stored sequences, so what comes back is the
learner being asked about something it has actually seen rather than about a
situation invented to make it look decisive.
*/
func learner(index int, memory *store.Retained[*iradix.Tree[[]byte]]) reading {
	answer := reading{
		agent: &telemetry.LearningAgentT{Id: int32(index), Status: "recognising"},
		held:  &telemetry.LearningLearnerT{Id: int32(index)},
	}
	tree := memory.Read()

	if tree == nil {
		answer.agent.Status = "empty"

		return answer
	}
	answer.links = int32(tree.Len())
	answer.held.Links = answer.links
	answer.agent.Decisions = uint64(answer.links)
	counted := map[string]int32{}
	order := make([]string, 0, 4)
	sequences := map[string][]byte{}

	// The memory is read at the moment it was last written. An unrelated clock
	// either applies no decay, so the link seen most often long ago wins every
	// situation, or so much that every link collapses onto the prior and the
	// reading becomes a tie between things the learner does distinguish.
	step := uint64(0)
	iterator := tree.Root().Iterator()
	iterator.SeekPrefix([]byte("b/"))

	for key, value, more := iterator.Next(); more; key, value, more = iterator.Next() {
		moment, sequence, named := bytes.Cut(key[2:], []byte("/"))

		if !named {
			continue
		}

		if written := cognition.DecodeWeight(value).WriteStep; written > step {
			step = written
		}
		name := string(moment)

		if _, seen := counted[name]; !seen {
			order = append(order, name)
		}

		// The most specific situation the moment was learned in. A situation of
		// one region is one several moments genuinely share — the same region
		// lights up entering, holding and exiting — so asking about it is
		// asking a question the tape never answered, and the reading would say
		// more about which moment was seen most than about recognition.
		if len(sequence) > len(sequences[name]) {
			sequences[name] = bytes.Clone(sequence)
		}
		counted[name]++
	}

	for _, name := range order {
		answer.held.Moments = append(answer.held.Moments, &telemetry.LearningMomentT{
			Name: name, Links: counted[name],
		})
		given := ask(tree, name, sequences[name], step)

		if given == nil {
			continue
		}
		answer.held.Answers = append(answer.held.Answers, given)

		// The dashboard reads the leading answer as the learner's last call, so
		// the strongest one it currently gives stands there rather than nothing.
		if answer.agent.Last == nil || given.Confidence > answer.agent.Last.Tape {
			answer.agent.Last = &telemetry.LearningDecisionT{
				Id:      uint64(index),
				Agent:   int32(index),
				Symbol:  "",
				Context: named(sequences[name]),
				Action:  &telemetry.LearningActionT{Kind: given.Answered},
				Tape:    given.Confidence,
				HasTape: true,
			}
		}
	}

	return answer
}

/* named lists the regions a stored sequence is made of, in their own order. */
func named(sequence []byte) []string {
	regions := make([]string, 0, len(sequence)/8)

	for at := 0; at+8 <= len(sequence); at += 8 {
		regions = append(regions, strconv.FormatUint(
			binary.BigEndian.Uint64(sequence[at:at+8]), 16,
		))
	}

	return regions
}

/*
ask puts one stored sequence back to the memory that holds it and reports what
came back, read at the clock the learner has actually reached.

Reading at zero would apply no decay at all, so a link observed a great many
times long ago outweighs a recent one no matter what either was graded — the
reading would be a count, not a belief. A winner at zero contrast is a tie rather than a decision, and it is
reported as it stands rather than dressed up as one.
*/
func ask(
	tree *iradix.Tree[[]byte], moment string, sequence []byte, step uint64,
) *telemetry.LearningAnswerT {
	if len(sequence) == 0 {
		return nil
	}
	given, err := transport.Evaluate(associative.NewRecall(store.NewRetained(tree)), transport.Values(cognition.Evaluation{
		Context: sequence, Config: cognition.DefaultConfig(), Step: step,
	}))
	if err != nil {
		return nil
	}

	return &telemetry.LearningAnswerT{
		Asked:      moment,
		Answered:   given.WinnerClass,
		RunnerUp:   given.RunnerUp,
		Confidence: given.Confidence,
		Contrast:   given.Contrast,
		Ambiguity:  given.Ambiguity,
		Support:    given.Support,
	}
}
