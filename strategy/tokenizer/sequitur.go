package tokenizer

import (
	"slices"
	"strconv"
	"strings"
	"sync"
)

const rulePrefix = "\x00rule:"

/*
Symbol is one terminal string (e.g. R08) or an internal rule reference (#rule:N).
*/
type Symbol string

func ruleSymbol(id int) Symbol {
	return Symbol(rulePrefix + strconv.Itoa(id))
}

func ruleID(sym Symbol) (int, bool) {
	raw := string(sym)
	if !strings.HasPrefix(raw, rulePrefix) {
		return 0, false
	}

	id, err := strconv.Atoi(raw[len(rulePrefix):])
	if err != nil || id <= 0 {
		return 0, false
	}

	return id, true
}

/*
RuleRef is the display form of a rule symbol for external consumption:
M1, M2, … The frontend's ruleName and the grammar viz both use this form.
*/
func RuleRef(id int) string {
	if id <= 0 {
		return "S"
	}
	return "M" + strconv.Itoa(id)
}

/*
Node is a doubly-linked element in the Sequitur sequence.
*/
type Node struct {
	Val        Symbol
	Prev, Next *Node
}

/*
Rule is a grammar production discovered by Sequitur.
Head is a circular sentinel whose body nodes are Head.Next … until Head again.
*/
type Rule struct {
	ID    int
	Count int
	Head  *Node
}

/*
Sequitur discovers hierarchical structure in a stream of string tokens via
online digram replacement. Repeated digrams are promoted to new rules.
The grammar operates on the flat token stream: each tick's regions are
individual terminals that the grammar may group into rules.
*/
type Sequitur struct {
	mu           sync.Mutex
	nextID       int
	rules        map[int]*Rule
	digrams      map[[2]Symbol]*Node
	ruleByDigram map[[2]Symbol]int
	sentinel     *Node
	length       int
	pendingLen   int
	lastRule     *Rule
	// lastKind is create, reuse, or empty when the last Analyze only appended.
	lastKind string
}

func NewSequitur() *Sequitur {
	sentinel := &Node{}
	sentinel.Next = sentinel
	sentinel.Prev = sentinel

	return &Sequitur{
		nextID:       1,
		rules:        make(map[int]*Rule),
		digrams:      make(map[[2]Symbol]*Node),
		ruleByDigram: make(map[[2]Symbol]int),
		sentinel:     sentinel,
	}
}

/*
Analyze appends tokens to the grammar's root sequence, runs digram
replacement after each one, and records the most recently created rule.
Returns the created rule (or nil) and whether a structural event occurred.
*/
func (sequitur *Sequitur) Analyze(tokens []string) (*Rule, bool) {
	sequitur.mu.Lock()
	defer sequitur.mu.Unlock()

	sequitur.lastKind = ""
	var created *Rule

	for _, token := range tokens {
		if token == "" {
			continue
		}

		newNode := &Node{Val: Symbol(token)}
		sequitur.appendNode(newNode)
		sequitur.length++
		sequitur.pendingLen++

		if rule := sequitur.check(newNode.Prev); rule != nil {
			created = rule
			sequitur.lastRule = rule
			sequitur.pendingLen = 0
		}
	}

	return created, created != nil
}

/*
Tail returns the last n symbols in the root sequence as display strings.
A rule reference is rendered as its RuleRef (M1, M2, …); a terminal is
itself. This is what the Sequencer feeds to MDL after each Analyze call.
*/
func (sequitur *Sequitur) Tail(n int) []string {
	sequitur.mu.Lock()
	defer sequitur.mu.Unlock()

	if n <= 0 || sequitur.length == 0 {
		return nil
	}
	if n > sequitur.length {
		n = sequitur.length
	}

	// Walk back n positions from the tail.
	type pos struct {
		node *Node
		idx  int
	}

	var collect []pos
	current := sequitur.sentinel.Prev
	for i := 0; i < n && current != sequitur.sentinel && current != nil; i++ {
		collect = append(collect, pos{current, i})
		current = current.Prev
	}

	// Reverse to get oldest-first order.
	out := make([]string, 0, len(collect))
	for i := len(collect) - 1; i >= 0; i-- {
		sym := collect[i].node.Val
		if id, ok := ruleID(sym); ok {
			out = append(out, RuleRef(id))
		} else {
			out = append(out, string(sym))
		}
	}

	return out
}

/*
LastRule is the most recently created production, or nil.
*/
func (sequitur *Sequitur) LastRule() *Rule {
	sequitur.mu.Lock()
	defer sequitur.mu.Unlock()
	return sequitur.lastRule
}

/*
Symbols is how many distinct symbols the grammar holds, rules included.
*/
func (sequitur *Sequitur) Symbols() int {
	sequitur.mu.Lock()
	defer sequitur.mu.Unlock()
	return len(sequitur.rules) + 1
}

/*
Rules lists every production with its right-hand side as display strings.
The dashboard's rulebook and the SequiturFrame's rules field use this.
*/
func (sequitur *Sequitur) Rules() [][]string {
	sequitur.mu.Lock()
	defer sequitur.mu.Unlock()

	productions := make([][]string, 0, len(sequitur.rules))

	for _, rule := range sequitur.rules {
		symbols := make([]string, 0)
		for node := rule.Head.Next; node != nil && node != rule.Head; node = node.Next {
			if id, ok := ruleID(node.Val); ok {
				symbols = append(symbols, RuleRef(id))
			} else {
				symbols = append(symbols, string(node.Val))
			}
		}
		productions = append(productions, symbols)
	}

	return productions
}

/*
RootSequence returns the full root as display strings, oldest first.
Used for the SequiturFrame's `start` field.
*/
func (sequitur *Sequitur) RootSequence() []string {
	sequitur.mu.Lock()
	defer sequitur.mu.Unlock()

	out := make([]string, 0, sequitur.length)
	for node := sequitur.sentinel.Next; node != nil && node != sequitur.sentinel; node = node.Next {
		if id, ok := ruleID(node.Val); ok {
			out = append(out, RuleRef(id))
		} else {
			out = append(out, string(node.Val))
		}
	}

	return out
}

/*
RootLength is the number of symbols currently in the root sequence.
*/
func (sequitur *Sequitur) RootLength() int {
	sequitur.mu.Lock()
	defer sequitur.mu.Unlock()
	return sequitur.length
}

/*
TerminalNames flattens rule into terminal token strings (e.g. R08, R07).
*/
func (sequitur *Sequitur) TerminalNames(rule *Rule) []string {
	if sequitur == nil || rule == nil || rule.Head == nil {
		return nil
	}

	sequitur.mu.Lock()
	defer sequitur.mu.Unlock()

	names := make([]string, 0, rule.Count)
	sequitur.collectTerminalNames(rule, &names)

	return names
}

func (sequitur *Sequitur) collectTerminalNames(rule *Rule, names *[]string) {
	for node := rule.Head.Next; node != rule.Head; node = node.Next {
		if id, ok := ruleID(node.Val); ok {
			if nested := sequitur.rules[id]; nested != nil {
				sequitur.collectTerminalNames(nested, names)
			}
			continue
		}
		*names = append(*names, string(node.Val))
	}
}

/*
Flush reports whether there are tokens since the last boundary and resets
the pending counter.
*/
func (sequitur *Sequitur) Flush() bool {
	sequitur.mu.Lock()
	defer sequitur.mu.Unlock()

	hasPending := sequitur.pendingLen > 0
	sequitur.pendingLen = 0
	return hasPending
}

/*
LastKind reports what the last Analyze did: "create" when a new rule was
minted, "reuse" when an existing rule absorbed the digram, and "append"
when the tick only extended the root.
*/
func (sequitur *Sequitur) LastKind() string {
	sequitur.mu.Lock()
	defer sequitur.mu.Unlock()

	if sequitur.lastKind == "" {
		return "append"
	}

	return sequitur.lastKind
}

/*
RuleView is one production as the dashboard publishes it. Depth is the
longest chain of rule references, terminals counting as 1. Count is how
many times the digram has been replaced by this rule.
*/
type RuleView struct {
	ID        int
	Count     int
	Depth     int
	Symbols   []string
	Terminals []string
}

/*
RuleViews lists every production with its real id, use count, and depth.
*/
func (sequitur *Sequitur) RuleViews() []RuleView {
	sequitur.mu.Lock()
	defer sequitur.mu.Unlock()

	views := make([]RuleView, 0, len(sequitur.rules))

	for _, rule := range sequitur.rules {
		symbols := make([]string, 0)
		for node := rule.Head.Next; node != nil && node != rule.Head; node = node.Next {
			if id, ok := ruleID(node.Val); ok {
				symbols = append(symbols, RuleRef(id))
			} else {
				symbols = append(symbols, string(node.Val))
			}
		}

		terminals := make([]string, 0, rule.Count)
		sequitur.collectTerminalNames(rule, &terminals)

		views = append(views, RuleView{
			ID:        rule.ID,
			Count:     rule.Count,
			Depth:     sequitur.ruleDepth(rule.ID, map[int]bool{}),
			Symbols:   symbols,
			Terminals: terminals,
		})
	}

	slices.SortFunc(views, func(a, b RuleView) int {
		return a.ID - b.ID
	})

	return views
}

func (sequitur *Sequitur) ruleDepth(id int, stack map[int]bool) int {
	if stack[id] {
		return 1
	}

	rule := sequitur.rules[id]
	if rule == nil || rule.Head == nil {
		return 1
	}

	stack[id] = true
	depth := 1

	for node := rule.Head.Next; node != nil && node != rule.Head; node = node.Next {
		child, ok := ruleID(node.Val)
		if !ok {
			continue
		}

		if nested := 1 + sequitur.ruleDepth(child, stack); nested > depth {
			depth = nested
		}
	}

	delete(stack, id)

	return depth
}

/*
RuleCount returns the number of discovered grammar rules.
*/
func (sequitur *Sequitur) RuleCount() int {
	sequitur.mu.Lock()
	defer sequitur.mu.Unlock()
	return len(sequitur.rules)
}

// --- internal (caller must hold mu) ---

func (sequitur *Sequitur) appendNode(n *Node) {
	last := sequitur.sentinel.Prev

	n.Next = sequitur.sentinel
	n.Prev = last
	last.Next = n
	sequitur.sentinel.Prev = n
}

func (sequitur *Sequitur) isSentinel(n *Node) bool {
	return n == nil || n == sequitur.sentinel
}

/*
check tests whether the digram ending at n forms a repeated pair.
If a match is found (non-overlapping), reduce fires. Returns the new rule.
*/
func (sequitur *Sequitur) check(n *Node) *Rule {
	if sequitur.isSentinel(n) || sequitur.isSentinel(n.Next) {
		return nil
	}

	digram := [2]Symbol{n.Val, n.Next.Val}

	if match, exists := sequitur.digrams[digram]; exists {
		if match.Next != nil && match.Val == digram[0] && match.Next.Val == digram[1] {
			if match == n || match.Next == n || match.Next == n.Next {
				return nil
			}
			return sequitur.reduce(n, match, digram)
		}
	}

	sequitur.digrams[digram] = n
	return nil
}

func (sequitur *Sequitur) reduce(
	newOccur, firstOccur *Node, d [2]Symbol,
) *Rule {
	if id, ok := sequitur.ruleByDigram[d]; ok {
		if rule := sequitur.rules[id]; rule != nil {
			rule.Count += 2
			sequitur.substitute(firstOccur, id)
			sequitur.substitute(newOccur, id)
			delete(sequitur.digrams, d)
			if sequitur.lastKind != "create" {
				sequitur.lastKind = "reuse"
			}
			sequitur.lastRule = rule
			sequitur.checkAdjacencies(firstOccur, newOccur)
			return rule
		}
	}

	id := sequitur.nextID
	sequitur.nextID++

	head := &Node{}
	n1 := &Node{Val: d[0], Prev: head}
	n2 := &Node{Val: d[1], Prev: n1, Next: head}
	head.Next = n1
	head.Prev = n2
	n1.Next = n2

	rule := &Rule{
		ID:    id,
		Count: 2,
		Head:  head,
	}

	sequitur.rules[id] = rule
	sequitur.ruleByDigram[d] = id
	sequitur.lastKind = "create"
	sequitur.lastRule = rule

	sequitur.substitute(firstOccur, id)
	sequitur.substitute(newOccur, id)

	delete(sequitur.digrams, d)

	sequitur.checkAdjacencies(firstOccur, newOccur)

	return rule
}

func (sequitur *Sequitur) substitute(n *Node, id int) {
	if n == nil || n.Next == nil || n.Next.Next == nil {
		return
	}

	victim := n.Next

	n.Val = ruleSymbol(id)
	n.Next = victim.Next
	victim.Next.Prev = n

	victim.Prev = nil
	victim.Next = nil
}

func (sequitur *Sequitur) checkAdjacencies(a, b *Node) {
	if !sequitur.isSentinel(a) && !sequitur.isSentinel(a.Prev) {
		sequitur.check(a.Prev)
	}
	if !sequitur.isSentinel(a) {
		sequitur.check(a)
	}
	if !sequitur.isSentinel(b) && b != a && !sequitur.isSentinel(b.Prev) {
		sequitur.check(b.Prev)
	}
	if !sequitur.isSentinel(b) && b != a {
		sequitur.check(b)
	}
}
