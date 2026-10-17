package strategy

import (
	"fmt"
	"testing"
)

func feedCodes(codes []uint32) *Grammar {
	grammar := NewGrammar()
	for _, code := range codes {
		grammar.AppendCode(code)
	}
	return grammar
}

func nonOverlappingDigrams(grammar *Grammar) []string {
	type occ struct {
		rule uint32
		left *Symbol
	}

	found := map[string][]occ{}

	for _, rule := range grammar.rules {
		for current := rule.guard.next; current != rule.guard && current != nil && current.next != nil && !current.next.isGuard; current = current.next {
			label := grammar.Name(current.value)
			if current.rule != nil {
				label = fmt.Sprintf("R%d", current.rule.id)
			}
			next := grammar.Name(current.next.value)
			if current.next.rule != nil {
				next = fmt.Sprintf("R%d", current.next.rule.id)
			}
			key := label + " " + next
			found[key] = append(found[key], occ{rule: rule.id, left: current})
		}
	}

	var violations []string
	for label, occs := range found {
		for i := 0; i < len(occs); i++ {
			for j := i + 1; j < len(occs); j++ {
				if occs[i].left.next == occs[j].left || occs[j].left.next == occs[i].left {
					continue
				}
				violations = append(violations, fmt.Sprintf("%s in R%d and R%d", label, occs[i].rule, occs[j].rule))
			}
		}
	}
	return violations
}

func grammarProblems(grammar *Grammar) []string {
	var problems []string

	for id, rule := range grammar.rules {
		if id == 0 {
			continue
		}
		refs := 0
		for ref := rule.refHead.nextRef; ref != nil && ref != rule.refHead && refs <= rule.count+2; ref = ref.nextRef {
			refs++
		}
		if rule.count < 2 || refs != rule.count {
			problems = append(problems, fmt.Sprintf("R%d count %d refs %d", id, rule.count, refs))
		}
	}

	for _, rule := range grammar.rules {
		for current := rule.guard.next; current != rule.guard && current != nil && current.next != nil && !current.next.isGuard; current = current.next {
			key := makeDigramKey(current, current.next)
			indexed, found := grammar.digrams[key]
			if !found {
				problems = append(problems, "unindexed digram")
				continue
			}
			if indexed != current && indexed.next != current {
				problems = append(problems, "digram index misses this occurrence")
			}
		}
	}

	return problems
}

func grammarExpands(grammar *Grammar, codes []uint32) bool {
	expanded := grammar.StartRule().Terminals()
	if len(expanded) != len(codes) {
		return false
	}
	for i := range codes {
		if expanded[i] != codes[i] {
			return false
		}
	}
	return true
}

func TestRepeatedDigramStaysUnique(t *testing.T) {
	input := []uint32{2, 2, 1, 1, 1, 2, 2, 1, 2, 1, 1, 1}
	grammar := feedCodes(input)

	if !grammarExpands(grammar, input) {
		t.Fatalf("expanded %v", grammar.StartRule().Terminals())
	}

	if violations := nonOverlappingDigrams(grammar); len(violations) > 0 {
		t.Fatalf("%s\n%v", grammar.String(), violations)
	}
	if problems := grammarProblems(grammar); len(problems) > 0 {
		t.Fatalf("%s\n%v", grammar.String(), problems)
	}
}

func TestBinaryGrammarsKeepDigramUniqueness(t *testing.T) {
	violators := 0
	for length := 1; length <= 14; length++ {
		for mask := 0; mask < 1<<length; mask++ {
			codes := make([]uint32, length)
			for i := 0; i < length; i++ {
				codes[i] = uint32(((mask >> (length - 1 - i)) & 1) + 1)
			}
			grammar := feedCodes(codes)
			if !grammarExpands(grammar, codes) {
				t.Fatalf("expansion %v from %v\n%s", grammar.StartRule().Terminals(), codes, grammar.String())
			}
			if len(nonOverlappingDigrams(grammar)) > 0 || len(grammarProblems(grammar)) > 0 {
				violators++
			}
		}
	}
	if violators != 0 {
		t.Fatalf("%d grammars repeated a digram", violators)
	}
}
