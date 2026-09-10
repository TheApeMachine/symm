package strategy

import (
	"bytes"
	"encoding/binary"
	"strconv"
	"time"

	flatbuffers "github.com/google/flatbuffers/go"
	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/learning/associative"
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

	return held.state(at)
}

/* state is what one mounted tape and its learners currently amount to. */
func (held *replay) state(at int64) *Recognition {
	state := &telemetry.LearningStateT{
		AtNs:     at,
		Steps:    held.frames,
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
	state.Recognition = held.recognised(at, learners)

	return &Recognition{state: state}
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
	at int64, learners []*telemetry.LearningLearnerT,
) *telemetry.LearningRecognitionT {
	return &telemetry.LearningRecognitionT{
		AtNs:      at,
		Fragments: int32(held.fragments),
		Frames:    held.frames,
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
func learner(index int, memory core.Primitive) reading {
	answer := reading{
		agent: &telemetry.LearningAgentT{Id: int32(index), Status: "recognising"},
		held:  &telemetry.LearningLearnerT{Id: int32(index)},
	}
	tree := core.To[*iradix.Tree[[]byte]](transport.NewApply(memory, nil).Next(nil))

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
	recall := associative.NewRecall(
		transport.NewIO(core.From(tree)),
		transport.NewIO(core.From(cognition.Evaluation{
			Context: sequence, Config: cognition.DefaultConfig(), Step: step,
		})),
	)
	var given cognition.Evaluation

	for answered := recall.Next(nil); answered != nil; answered = recall.Next(nil) {
		given = core.To[cognition.Evaluation](answered)
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
