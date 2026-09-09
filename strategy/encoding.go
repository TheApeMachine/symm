package strategy

import (
	"sort"
	"strconv"

	flatbuffers "github.com/google/flatbuffers/go"
	"github.com/theapemachine/symm/nomagique/learning/associative/agent"

	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
)

/*
	MarshalFlatbuffer encodes original learning and trading owners under their

state lock. It performs no learning, valuation or statistical calculation.
*/
func (learner *Learner) MarshalFlatbuffer(focus string) []byte {
	learner.mutex.Lock()
	defer learner.mutex.Unlock()

	state := &wire.LearningStateT{
		AtNs:      learner.At.UnixNano(),
		Steps:     learner.Steps,
		Decisions: learner.Decisions,
		Resolved:  learner.Resolved,
		Status:    learner.Traders[0].Status,
		Restored:  learner.Restored,
	}

	if learner.Rehearsal != nil {
		state.Rehearsal = learner.Rehearsal.Wire()
	}

	if learner.Err != nil {
		state.Status = learner.Err.Error()
	}

	for index, trader := range learner.Traders {
		member := learner.Agent
		entry := trader.Wire(member, focus)
		entry.Id = int32(index)
		state.Agents = append(state.Agents, entry)
	}
	learner.Grid.Mutex.Lock()
	space := learner.Grid.Space
	func() {

		for row, symbol := range space.Rows {
			impulse := learner.Grid.Latest[symbol]
			activity, quality, err := space.Activity(symbol)

			if err != nil {
				panic(err)
			} // Rows and Activity share the same grid identity dictionary.

			entry := &wire.LearningDevelopmentT{
				Symbol:    symbol,
				AtNs:      impulse.At.UnixNano(),
				FromNs:    impulse.From.UnixNano(),
				Status:    "learning",
				Decisions: 0,
				// Retained state transitions, not tokens: Context below flattens
				// each transition's conditions into one list.
				Depth: 0,
			}

			if focus != "" && focus != symbol {
				state.Markets = append(state.Markets, entry)
				continue
			}

			if !impulse.Ready {
				entry.Status = "waiting for regions"
			}
			if activation := learner.Agent.Activations[symbol]; activation != nil {
				entry.Depth = int32(len(activation.Context))
				for _, condition := range activation.Context {
					entry.Context = append(entry.Context, strconv.FormatUint(condition, 10))
				}
			}

			for _, region := range impulse.Regions {
				entry.Regions = append(entry.Regions, &wire.LearningRegionT{
					Id:        region.ID,
					Condition: region.Condition,
					Level:     region.Level,
					Change:    region.Change,
					Strength:  region.Strength,
					Authority: region.Authority,
					Members:   int32(region.Members),
				})
			}

			for column, identity := range space.Columns {
				entry.Quantities = append(entry.Quantities, &wire.LearningQuantityT{
					Source:   identity[0],
					Label:    identity[1],
					X:        space.Coordinates[column][0],
					Y:        space.Coordinates[column][1],
					Value:    space.Values[row][column],
					Present:  space.Present[row][column],
					Activity: activity[column],
					Quality:  quality[column],
				})
			}

			state.Markets = append(state.Markets, entry)
		}

	}()
	learner.Grid.Mutex.Unlock()

	builder := flatbuffers.NewBuilder(0)
	builder.Finish(state.Pack(builder))
	return builder.FinishedBytes()
}

/* Wire uses the same Balance, Holding and associative readings used to act. */
func (trader *Trader) Wire(member *agent.Agent[Action], focus string) *wire.LearningAgentT {
	reading := member.Reading

	state := &wire.LearningAgentT{
		Initial:    trader.Initial.String(),
		Fees:       trader.Fees.String(),
		Cash:       trader.Balance.Cash().String(),
		Equity:     trader.Equity.String(),
		Profit:     trader.Profit.String(),
		Realized:   trader.Realized.String(),
		Unrealized: trader.Unrealized.String(),
		Wealth:     trader.Wealth,
		Decisions:  member.Decisions,
		Fills:      trader.Fills,
		Pending:    uint64(len(member.Pending)),
		Wins:       member.Positive,
		Losses:     member.Negative,
		Status:     trader.Status,
		Reward:     member.Reward.TotalReward,
		ElapsedNs:  int64(member.Reward.TotalElapsed),
		Open:       int32(trader.open()),
	}

	state.Reading = &wire.LearningPriorT{
		Defined:           reading.Defined,
		Mean:              reading.Mean,
		Variance:          reading.Variance,
		VarianceDefined:   reading.VarianceDefined,
		Samples:           reading.Samples,
		Support:           reading.Support,
		Authority:         reading.Authority,
		Maturity:          reading.Maturity,
		EvidenceAuthority: reading.EvidenceAuthority,
		Memory:            reading.Memory,
	}

	for _, regulator := range trader.Positions {
		state.Positions = append(state.Positions, regulator.Wire())
	}
	sort.Slice(state.Positions, func(left, right int) bool {
		return state.Positions[left].Holding.Symbol < state.Positions[right].Holding.Symbol
	})

	activation := member.Activations[focus]

	if activation != nil {
		last := activation
		state.Last = &wire.LearningDecisionT{
			Id:     last.ID,
			Symbol: last.Label,
			AtNs:   last.At.UnixNano(),
			Action: &wire.LearningActionT{
				Kind:   last.Action.Kind,
				Power:  int32(last.Action.Power),
				Reduce: last.Action.Reduce,
			},
		}

		for _, token := range last.Context {
			state.Last.Context = append(state.Last.Context, strconv.FormatUint(token, 10))
		}

		if last.Outcome != nil {
			state.Last.HasTape, state.Last.Tape = true, *last.Outcome
		}
	}

	if activation != nil {
		for _, action := range activation.Alternatives {
			state.Alternatives = append(state.Alternatives, &wire.LearningActionT{
				Kind: action.Kind, Power: int32(action.Power), Reduce: action.Reduce,
			})
		}
	}

	if evaluation := trader.LastEvaluation; evaluation != nil {
		state.Outcome = &wire.LearningDecisionT{Id: evaluation.ID, Agent: int32(trader.ID), Symbol: evaluation.Label, AtNs: evaluation.At.UnixNano(), ThroughNs: evaluation.Through.UnixNano(), Action: &wire.LearningActionT{Kind: evaluation.Action.Kind, Power: int32(evaluation.Action.Power), Reduce: evaluation.Action.Reduce}, Tape: evaluation.Value, HasTape: true, Quantity: evaluation.Quantity.String()}
	}
	return state
}

/*
Wire copies rehearsal counters and each worker's mounted track while the
workers continue independently. Tracks are gathered before the rehearsal lock
is taken: a worker takes its own lock first and the rehearsal lock second.
*/
func (rehearsal *Rehearsal) Wire() *wire.LearningRehearsalT {
	tracks := make([]*wire.LearningTrackT, 0, len(rehearsal.cursors))

	for index, cursor := range rehearsal.cursors {
		tracks = append(tracks, cursor.wire(int32(index)))
	}
	rehearsal.mutex.Lock()
	defer rehearsal.mutex.Unlock()
	state := rehearsal.progress
	state.Tracks = tracks

	if rehearsal.err != nil {
		state.Status = rehearsal.err.Error()
	}

	return &state
}
