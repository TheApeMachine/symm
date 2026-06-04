package reasoning

import (
	"context"
	"sort"

	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/optimizer/replay"
)

// capitalBlockWeight is how hard the score discounts a profitable forest for tying
// up the wallet: at most this fraction is shaved when every tick blocks a wanting
// entry. It prices the opportunity cost of capital without overriding genuine edge.
const capitalBlockWeight = 0.5

/*
SearchConfig tunes the beam search. Zero fields fall back to defaults, so a caller
can set only what it cares about.
*/
type SearchConfig struct {
	BeamWidth     int // forests carried between expansion rounds
	MaxRounds     int // hard cap on rounds
	Patience      int // stop after this many rounds with no new best
	MaxNodes      int // forests larger than this are not explored (bounds runaway depth)
	MinRoundTrips int // a profitable forest closing fewer trades than this is discounted toward 0
}

func (config SearchConfig) withDefaults() SearchConfig {
	if config.BeamWidth <= 0 {
		config.BeamWidth = 8
	}

	if config.MaxRounds <= 0 {
		config.MaxRounds = 20
	}

	if config.Patience <= 0 {
		config.Patience = 4
	}

	if config.MaxNodes <= 0 {
		config.MaxNodes = 24
	}

	return config
}

/*
Candidate is a scored reasoning forest. Score is the value the search maximises:
realized return, discounted when it rests on too few trades. Depth is NOT penalised
in the score — a deeper tree with the same return ranks equal and is kept for
further growth, with node count used only as a tie-break (prefer the simpler of two
equals) and a hard MaxNodes cap to stop runaway bloat. This is what lets the search
explore the temporal chains the playbook is meant to express.
*/
type Candidate struct {
	Forest []perspectives.Thought
	Score  float64
	Return float64 // raw realized return from the replay
	Trades int
	Nodes  int
}

// Result is the outcome of a search: the best forest found and how much was tried.
type Result struct {
	Best      Candidate
	Evaluated int
}

/*
Search grows reasoning forests from the data and returns the best one it can score
on the replay tape. It is deterministic: the vocabulary, seeds, and neighbours are
all generated in a fixed order, so the same rows and config yield the same forest.
*/
func Search(
	ctx context.Context,
	rows []perspectives.Measurement,
	costs replay.ReplayCosts,
	config SearchConfig,
) Result {
	config = config.withDefaults()
	vocab := DeriveVocabulary(rows)
	tape := replay.PrecompileTape(rows)

	evaluated := 0
	seen := make(map[string]bool)

	score := func(forest []perspectives.Thought, nodes int) Candidate {
		result := replay.NewThoughtSimulation(ctx, forest, tape, costs).Result()

		credited := result.Score

		if credited > 0 {
			if config.MinRoundTrips > 0 && result.ClosedTrades < config.MinRoundTrips {
				credited *= float64(result.ClosedTrades) / float64(config.MinRoundTrips)
			}

			// Capital opportunity cost: a forest that camps in one position blocks
			// entries its other branches wanted. Discount the profit by how often
			// the wallet was too locked to fund a wanting entry — rewarding yield per
			// unit of capital, not absolute return, so the search prefers strategies
			// that keep the account working over ones that tie it up.
			if tape.Len() > 0 && result.FundBlocked > 0 {
				blockRate := float64(result.FundBlocked) / float64(tape.Len())

				if blockRate > 1 {
					blockRate = 1
				}

				credited *= 1 - capitalBlockWeight*blockRate
			}
		}

		return Candidate{
			Forest: forest,
			Score:  credited,
			Return: result.Score,
			Trades: result.ClosedTrades,
			Nodes:  nodes,
		}
	}

	consider := func(forest []perspectives.Thought) (Candidate, bool) {
		nodes := countNodes(forest)
		if nodes > config.MaxNodes {
			return Candidate{}, false // past the size cap; do not explore further
		}

		key := keyOf(forest)
		if seen[key] {
			return Candidate{}, false
		}

		seen[key] = true
		evaluated++

		return score(forest, nodes), true
	}

	beam := make([]Candidate, 0, config.BeamWidth)

	for _, seed := range Seeds(vocab) {
		if candidate, fresh := consider(seed); fresh {
			beam = append(beam, candidate)
		}
	}

	beam = topCandidates(beam, config.BeamWidth)

	if len(beam) == 0 {
		return Result{Evaluated: evaluated}
	}

	best := beam[0]

	stagnation := 0

	for round := 0; round < config.MaxRounds; round++ {
		grown := make([]Candidate, 0, len(beam)*16)

		for _, member := range beam {
			for _, neighbor := range Neighbors(member.Forest, vocab) {
				if candidate, fresh := consider(neighbor); fresh {
					grown = append(grown, candidate)
				}
			}
		}

		if len(grown) == 0 {
			break
		}

		beam = topCandidates(append(beam, grown...), config.BeamWidth)

		if beam[0].Score > best.Score {
			best = beam[0]
			stagnation = 0

			continue
		}

		stagnation++
		if stagnation >= config.Patience {
			break
		}
	}

	return Result{Best: best, Evaluated: evaluated}
}

// topCandidates sorts by score (then simpler, then stable by encoding) and keeps the
// best width.
func topCandidates(candidates []Candidate, width int) []Candidate {
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}

		return candidates[i].Nodes < candidates[j].Nodes
	})

	if len(candidates) > width {
		candidates = candidates[:width]
	}

	return candidates
}

func countNodes(forest []perspectives.Thought) int {
	total := 0

	var walk func(nodes []perspectives.Thought)
	walk = func(nodes []perspectives.Thought) {
		for index := range nodes {
			total++
			walk(nodes[index].Then)
		}
	}

	walk(forest)

	return total
}

// keyOf is the dedup identity of a forest: its serialized playbook. Two forests
// that write the same YAML are the same candidate and are scored once.
func keyOf(forest []perspectives.Thought) string {
	encoded, err := perspectives.MarshalThoughts(forest, 2)
	if err != nil {
		return ""
	}

	return string(encoded)
}
