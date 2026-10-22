package tokenizer

import (
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
*/
type Sequitur struct {
	mu         sync.Mutex
	nextID     int
	rules      map[int]*Rule
	digrams    map[[2]Symbol]*Node
	sentinel   *Node
	length     int
	pendingLen int
	lastRule   *Rule
}

/*
NewSequitur creates a new Sequitur.
*/
func NewSequitur() *Sequitur {
	sentinel := &Node{}

	sentinel.Next = sentinel
	sentinel.Prev = sentinel

	return &Sequitur{
		nextID:   1,
		rules:    make(map[int]*Rule),
		digrams:  make(map[[2]Symbol]*Node),
		sentinel: sentinel,
	}
}

/*
Analyze appends tokens to the grammar, runs digram replacement after each one,
and reports whether a new rule was created on this call. When a rule is created,
LastRule returns it until the next creation.
*/
func (sequitur *Sequitur) Analyze(tokens []string) (*Rule, bool) {
	sequitur.mu.Lock()
	defer sequitur.mu.Unlock()

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
LastRule is the most recently created production, or nil.
*/
func (sequitur *Sequitur) LastRule() *Rule {
	sequitur.mu.Lock()
	defer sequitur.mu.Unlock()

	return sequitur.lastRule
}

/*
Symbols is how many distinct symbols the grammar holds, root included.
*/
func (sequitur *Sequitur) Symbols() int {
	sequitur.mu.Lock()
	defer sequitur.mu.Unlock()

	return len(sequitur.rules) + 1
}

/*
Rules lists every production with its right-hand side as symbol strings:
the grammar snapshot the hub streams so the dashboard renders the live
rulebook.
*/
func (sequitur *Sequitur) Rules() [][]string {
	sequitur.mu.Lock()
	defer sequitur.mu.Unlock()

	productions := make([][]string, 0, len(sequitur.rules))

	for _, rule := range sequitur.rules {
		symbols := make([]string, 0)

		for node := rule.Head.Next; node != nil && node != rule.Head; node = node.Next {
			symbols = append(symbols, string(node.Val))
		}

		productions = append(productions, symbols)
	}

	return productions
}

/*
TerminalNames flattens rule into terminal token strings (e.g. R08/R07).
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
Flush reports whether there are tokens since the last boundary.
The caller should treat true as a flush boundary and emit their buffer.
*/
func (sequitur *Sequitur) Flush() bool {
	sequitur.mu.Lock()
	defer sequitur.mu.Unlock()

	hasPending := sequitur.pendingLen > 0
	sequitur.pendingLen = 0

	return hasPending
}

/*
RuleCount returns the number of discovered grammar rules.
*/
func (sequitur *Sequitur) RuleCount() int {
	sequitur.mu.Lock()
	defer sequitur.mu.Unlock()

	return len(sequitur.rules)
}

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
	ruleID := sequitur.nextID
	sequitur.nextID++

	head := &Node{}
	n1 := &Node{Val: d[0], Prev: head}
	n2 := &Node{Val: d[1], Prev: n1, Next: head}
	head.Next = n1
	head.Prev = n2
	n1.Next = n2

	rule := &Rule{
		ID:    ruleID,
		Count: 2,
		Head:  head,
	}

	sequitur.rules[ruleID] = rule

	sequitur.substitute(firstOccur, ruleID)
	sequitur.substitute(newOccur, ruleID)

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
