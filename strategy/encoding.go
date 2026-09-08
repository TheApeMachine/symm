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
		Status:    "learning",
		Restored:  learner.Restored,
	}

	if learner.Err != nil {
		state.Status = learner.Err.Error()
	}

	for index, trader := range learner.Traders {
		member := learner.Population.Agents[index]
		entry := trader.Wire(member, focus)
		entry.Id = int32(index)
		state.Agents = append(state.Agents, entry)
	}
	space := learner.Population.Grid

	for row, symbol := range space.Rows {
		development := learner.Developments[symbol]
		activity, quality, err := space.Activity(symbol)

		if err != nil {
			panic(err)
		} // Rows and Activity share the same grid identity dictionary.

		entry := &wire.LearningDevelopmentT{
			Symbol:    symbol,
			AtNs:      development.At.UnixNano(),
			FromNs:    development.From.UnixNano(),
			Status:    "learning",
			Decisions: development.Decisions,
		}

		if focus != "" && focus != symbol {
			state.Markets = append(state.Markets, entry)
			continue
		}

		for _, context := range development.History {
			for _, condition := range context.Conditions {
				entry.Context = append(entry.Context, strconv.FormatUint(condition, 10))
			}
		}

		for _, region := range development.Regions {
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
		last := activation.Decision
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
		for _, choice := range activation.Choices {
			reading := choice.Prior
			state.Alternatives = append(state.Alternatives, &wire.LearningActionT{
				Kind:   choice.Action.Kind,
				Power:  int32(choice.Action.Power),
				Reduce: choice.Action.Reduce,
				Prior: &wire.LearningPriorT{
					Defined:           reading.Defined,
					Mean:              reading.Mean,
					Variance:          reading.Variance,
					VarianceDefined:   reading.VarianceDefined,
					Samples:           reading.Samples,
					Support:           reading.Support,
					Authority:         reading.Authority,
					Provisional:       reading.Provisional,
					Maturity:          reading.Maturity,
					EvidenceAuthority: reading.EvidenceAuthority,
					Depth:             int32(reading.Depth),
					ContextLength:     int32(reading.ContextLength),
					Pending:           reading.Pending,
					Memory:            reading.Memory,
				},
			})
		}
	}
	if evaluation := trader.LastEvaluation; evaluation != nil {
		state.Outcome = &wire.LearningDecisionT{Id: evaluation.ID, Agent: int32(trader.ID), Symbol: evaluation.Label, AtNs: evaluation.At.UnixNano(), ThroughNs: evaluation.Through.UnixNano(), Action: &wire.LearningActionT{Kind: evaluation.Action.Kind, Power: int32(evaluation.Action.Power), Reduce: evaluation.Action.Reduce}, Tape: evaluation.Value, HasTape: true, Quantity: evaluation.Quantity.String()}
	}
	return state
}
