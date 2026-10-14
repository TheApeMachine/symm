package strategy

import (
	"fmt"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/store"
)

type Tokenizer struct {
	Depth     int     `json:"depth"`
	Regions   int     `json:"regions"`
	Threshold float64 `json:"threshold"`
}

const tokenizerKey = "tokenizer.json"

/*
Token is kept for hindsight/audit compatibility.
*/
func (tokenizer *Tokenizer) Token(regions []*store.Region) (string, float64) {
	count := max(min(tokenizer.Regions, len(regions)), 1)
	keys := make([]string, count)

	for idx, region := range regions[:count] {
		keys[idx] = fmt.Sprintf("R%02d", region.Key)
	}

	return strings.Join(keys, "-"), regions[count-1].Brightness
}

/*
Active returns all individual region names ("R00".."R14") on this tick
whose brightness clears Threshold, ordered brightest first.
*/
func (tokenizer *Tokenizer) Active(regions []*store.Region) ([]string, float64) {
	if len(regions) == 0 {
		return nil, 0
	}

	var active []string

	for _, region := range regions {
		if region.Brightness < tokenizer.Threshold {
			break
		}

		active = append(active, fmt.Sprintf("R%02d", region.Key))
	}

	return active, regions[0].Brightness
}

/*
StepGrammar feeds newly active regions into a symbol's Sequitur Grammar
and returns the latest induced rule's terminal sequence (if a rule completed).
*/
func (tokenizer *Tokenizer) StepGrammar(
	grammar *Grammar,
	lastSymbol *string,
	active []string,
) ([]string, bool) {
	var induced []string
	var moved bool

	for _, sym := range active {
		if sym == *lastSymbol {
			continue
		}

		*lastSymbol = sym
		rule := grammar.Append(sym)

		if rule == nil {
			continue
		}

		terms := rule.TerminalNames(grammar)

		if tokenizer.Depth > 0 && len(terms) > tokenizer.Depth {
			terms = terms[len(terms)-tokenizer.Depth:]
		}

		if len(terms) >= 2 {
			induced = terms
			moved = true
		}
	}

	return induced, moved
}

/*
Advance appends new active regions onto a live rolling path (deduplicating
consecutive identical regions) and trims to Depth.
*/
func (tokenizer *Tokenizer) Advance(path []string, token string, strength float64) ([]string, bool) {
	if strength < tokenizer.Threshold {
		return path, false
	}

	if len(path) > 0 && path[len(path)-1] == token {
		return path, false
	}

	kept := path[max(len(path)-tokenizer.Depth+1, 0):]
	next := make([]string, 0, len(kept)+1)
	return append(append(next, kept...), token), true
}

/*
MatchSuffix finds the longest Sequitur rule in actions that matches the
tail of the live token stream.
*/
func MatchSuffix(tokens []string, actions map[string]string) (path string, action string) {
	for start := range tokens {
		if len(tokens)-start < 2 {
			break
		}

		candidate := strings.Join(tokens[start:], "/")

		if act := actions[candidate]; act != "" {
			return candidate, act
		}
	}

	return "", ""
}

func (tokenizer *Tokenizer) validate() error {
	if tokenizer.Depth < 1 || tokenizer.Regions < 1 || tokenizer.Regions > store.RegionCount {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf(
				"[tokenizer] depth %d must be at least 1 and regions %d within [1, %d]",
				tokenizer.Depth, tokenizer.Regions, store.RegionCount,
			),
			nil,
		))
	}

	return nil
}
