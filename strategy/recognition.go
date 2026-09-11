package strategy

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strconv"
	"time"

	flatbuffers "github.com/google/flatbuffers/go"
	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
	"github.com/theapemachine/symm/nomagique/store"
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
	training.mu.Lock()
	at := time.Now().UTC().UnixNano()
	held := training.snapshot()
	frames := training.seen.Load()
	training.mu.Unlock()

	rec := held.state(at, frames)

	if held.fragments == 0 && held.loading {
		rec.state.Status = "reading the record"

		if rec.state.Rehearsal != nil {
			rec.state.Rehearsal.Status = "reading the record"
		}
	}

	return rec
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
		var individual *Agent

		if index < len(held.cohort) {
			individual = held.cohort[index]
		}
		learned := learner(index, memory, individual)

		if index == 0 && held.mainAgent != nil {
			mainTel := held.mainAgent.AgentTelemetry()
			state.Agents = append(state.Agents, mainTel)
		}

		if index > 0 || held.mainAgent == nil {
			learned.agent.Id = int32(index)
			learned.agent.Status = "learning"
			state.Agents = append(state.Agents, learned.agent)
		}

		state.Decisions += uint64(learned.links)
		learners = append(learners, learned.held)
	}

	var totalResolved uint64

	if held.fragments > 0 {
		totalResolved = uint64(held.fragments)
	}

	if held.mainAgent != nil {
		totalResolved += held.mainAgent.Graded()
	}
	state.Resolved = totalResolved

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
	var profitable, declining, quiet uint64

	for _, leg := range held.tape {
		if len(leg) < 2 {
			quiet++
			continue
		}
		valStart, okStart := frameValue(leg[0])
		valEnd, okEnd := frameValue(leg[len(leg)-1])

		if !okStart || !okEnd {
			quiet++
			continue
		}

		if valEnd > valStart {
			profitable++
		}

		if valEnd < valStart {
			declining++
		}

		if valEnd == valStart {
			quiet++
		}
	}

	observations := held.observations

	if observations == 0 && held.fragments > 0 {
		observations = frames
	}
	runs := int32(held.runs)

	if runs == 0 && held.fragments > 0 {
		runs = 1
	}

	rehearsal := &telemetry.LearningRehearsalT{
		Status:       held.status(),
		Workers:      int32(len(learners)),
		Episodes:     uint64(held.fragments),
		Trained:      uint64(held.fragments),
		Profitable:   profitable,
		Declining:    declining,
		Quiet:        quiet,
		Observations: observations,
		Budget:       held.budget,
		Runs:         runs,
		Tracks:       make([]*telemetry.LearningTrackT, 0, len(learners)),
	}
	for index, learner := range learners {
		steps, symbol, entry, exit := held.tapeSteps(index)
		rehearsal.Tracks = append(
			rehearsal.Tracks, learnerTrack(index, learner, steps, symbol, entry, exit, frames),
		)
	}

	return rehearsal
}

/* tapeSteps projects a mounted fragment into one scalar per frame. */
func (held *replay) tapeSteps(index int) (
	[]*telemetry.LearningStepT, string, int32, int32,
) {
	if len(held.tape) == 0 {
		return nil, "", -1, -1
	}
	leg := held.tape[index%len(held.tape)]
	steps := make([]*telemetry.LearningStepT, 0, len(leg))
	symbol := ""
	entry, exit := int32(-1), int32(-1)

	for index, frame := range leg {
		value, defined := frameValue(frame)

		if symbol == "" && len(frame) > 0 {
			symbol = frame[0].Label
		}
		moment := frameMoment(frame)

		if (moment == "enter" || moment == "enter_long" || moment == "enter_short") && entry < 0 {
			entry = int32(index)
		}

		if moment == "exit" || moment == "exit_long" || moment == "exit_short" {
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
	case "enter", "enter_long", "enter_short":
		if entry >= 0 {
			return entry
		}

		return length / 3
	case "exit", "exit_long", "exit_short":
		if exit >= 0 {
			return exit
		}

		return 2 * length / 3
	case "hold", "hold_long", "hold_short":
		return length / 2
	}

	return 0
}

/* status is what the learning path is waiting on, in its own words. */
func (held *replay) status() string {
	if held.mainAgent != nil && held.mainAgent.Status() == "trading" {
		return "trading"
	}

	if held.loading && held.fragments == 0 {
		return "reading the record"
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

	for _, symbol := range space.Rows {
		development := &telemetry.LearningDevelopmentT{
			Symbol: symbol,
			AtNs:   at,
			FromNs: at,
			Status: held.status(),
		}
		quantities, regions, version, err := space.MarketSnapshot(symbol)

		if err != nil {
			continue
		}

		for _, q := range quantities {
			development.Quantities = append(
				development.Quantities, &telemetry.LearningQuantityT{
					Source:   q.Source,
					Label:    q.Label,
					X:        q.X,
					Y:        q.Y,
					Value:    q.Value,
					Activity: q.Activity,
					Quality:  q.Quality,
					Present:  q.Present,
				},
			)
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
func learner(
	index int,
	memory *store.Retained[*iradix.Tree[[]byte]],
	individual *Agent,
) reading {
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
	answer.agent.Reading = &telemetry.LearningPriorT{
		Defined:  answer.links > 0,
		Samples:  uint64(answer.links),
		Support:  float64(answer.links),
		Maturity: float64(answer.links) / 100.0,
	}
	counted := map[string]int32{}
	order := make([]string, 0, 4)

	if individual != nil && individual.Engine() != nil {
		classCounts := individual.Engine().ClassCounts()
		for name, count := range classCounts {
			counted[name] = count
			order = append(order, name)
		}
	} else {
		step := uint64(0)
		iterator := tree.Root().Iterator()
		iterator.SeekPrefix([]byte("b/"))

		for key, value, more := iterator.Next(); more; key, value, more = iterator.Next() {
			if !bytes.HasPrefix(key, []byte("b/")) {
				break
			}
			class, _, named := cognition.ParseBasinKey(key)

			if !named {
				continue
			}

			if written := cognition.DecodeWeight(value).WriteStep; written > step {
				step = written
			}
			name := string(class)

			if _, seen := counted[name]; !seen {
				order = append(order, name)
			}
			counted[name]++
		}
	}

	for _, name := range order {
		answer.held.Moments = append(answer.held.Moments, &telemetry.LearningMomentT{
			Name: name, Links: counted[name],
		})
	}

	if individual != nil {
		answer.held.Answers = individual.Answers()
	}

	if len(answer.held.Answers) > 0 {
		last := answer.held.Answers[len(answer.held.Answers)-1]
		answer.agent.Last = &telemetry.LearningDecisionT{
			Id:      uint64(index),
			Agent:   int32(index),
			Symbol:  "",
			Action:  &telemetry.LearningActionT{Kind: last.Answered},
			Tape:    last.Confidence,
			HasTape: true,
		}
	}

	if answer.links > 0 {
		branches := make([]*telemetry.CognitionBranchT, 0, 256)
		branches = append(branches, &telemetry.CognitionBranchT{
			Id:          0,
			ParentId:    -1,
			Token:       "•",
			Prefix:      "",
			Key:         "",
			Depth:       0,
			Probability: 1.0,
			Count:       uint64(answer.links),
		})

		nodeByKey := make(map[string]int64, 256)
		it := tree.Root().Iterator()
		it.SeekPrefix([]byte("b/"))

		for key, value, more := it.Next(); more && len(branches) < 256; key, value, more = it.Next() {
			if !bytes.HasPrefix(key, []byte("b/")) {
				break
			}
			class, sequence, named := cognition.ParseBasinKey(key)

			if !named || len(sequence) == 0 {
				continue
			}

			weight := cognition.DecodeWeight(value)
			parentID := int64(0)
			prefix := ""
			depth := int64(1)

			for at := 0; at+8 <= len(sequence) && len(branches) < 256; at += 8 {
				tokenVal := binary.BigEndian.Uint64(sequence[at : at+8])
				token := formatRegionToken(tokenVal)
				pathKey := prefix + "/" + token

				if prefix == "" {
					prefix = token
				}

				if prefix != token {
					prefix = prefix + " → " + token
				}

				nodeID, exists := nodeByKey[pathKey]

				if !exists {
					nodeID = int64(len(branches))
					nodeByKey[pathKey] = nodeID
					branches = append(branches, &telemetry.CognitionBranchT{
						Id:          nodeID,
						ParentId:    parentID,
						Token:       token,
						Prefix:      prefix,
						Key:         pathKey,
						Depth:       depth,
						Probability: 1.0,
						Count:       weight.Count,
					})
				}
				parentID = nodeID
				depth++
			}

			if len(branches) < 256 {
				actionToken := string(class)
				actionPathKey := prefix + "/action:" + actionToken
				actionPrefix := prefix + " → " + actionToken

				prob := weight.Probability

				if prob <= 0 && counted[actionToken] > 0 {
					prob = float64(weight.Count) / float64(counted[actionToken])
				}

				leafID, exists := nodeByKey[actionPathKey]

				if !exists {
					leafID = int64(len(branches))
					nodeByKey[actionPathKey] = leafID
					branches = append(branches, &telemetry.CognitionBranchT{
						Id:          leafID,
						ParentId:    parentID,
						Token:       actionToken,
						Prefix:      actionPrefix,
						Key:         actionPathKey,
						Depth:       depth,
						Probability: prob,
						Count:       weight.Count,
					})
				}
			}
		}
		answer.held.Branches = branches
	}

	return answer
}

/* formatRegionToken formats a condition token into human-readable region and ternary directions. */
func formatRegionToken(tokenVal uint64) string {
	regionID := grid.ConditionQuantity(tokenVal)
	levelState := tokenVal & 0x3
	changeState := (tokenVal >> 2) & 0x3

	levelStr := "="

	if levelState == 1 {
		levelStr = "+"
	}

	if levelState == 2 {
		levelStr = "-"
	}
	changeStr := "—"

	if changeState == 1 {
		changeStr = "▲"
	}

	if changeState == 2 {
		changeStr = "▼"
	}

	return fmt.Sprintf("R%d %s%s", regionID, levelStr, changeStr)
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


