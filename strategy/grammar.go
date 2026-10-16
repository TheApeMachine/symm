package strategy

import (
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/theapemachine/errnie"
)

/*
digramKey identifies a pair of adjacent symbols in the grammar.
leftID and rightID encode whether each symbol is terminal or non-terminal
using the high bit (1 << 63) for non-terminal rule references.
*/
type digramKey struct {
	leftID  uint64
	rightID uint64
}

/*
Symbol represents one node in a rule's doubly-linked circular symbol list.
It represents either a terminal (value > 0, rule == nil) or a non-terminal
rule reference (rule != nil). Sentinel guard nodes have isGuard = true.
*/
type Symbol struct {
	prev    *Symbol
	next    *Symbol
	owner   *Rule
	rule    *Rule
	value   uint32
	isGuard bool
	prevRef *Symbol
	nextRef *Symbol
}

/*
Rule represents one grammar production rule R_id -> Symbol...
*/
type Rule struct {
	id      uint32
	guard   *Symbol
	count   int
	refHead *Symbol
}

/*
Grammar implements Nevill-Manning & Witten (1997) Sequitur algorithm:
streaming hierarchical rule induction preserving Digram Uniqueness and Rule Utility.
Zero floating point, zero magic constants, purely deterministic.
*/
type Grammar struct {
	startRule   *Rule
	rules       map[uint32]*Rule
	digrams     map[digramKey]*Symbol
	names       map[uint32]string
	symbolCodes map[string]uint32
	nextRuleID  uint32
	nextCodeID  uint32
	trace       bool
	events      []GrammarEvent
}

/*
Grammar event kinds, in the order Sequitur performs them on one Append:
the terminal joins R0, then each digram repeat is resolved either by creating
a rule for it or by reusing the rule whose whole body it is, and a rule left
with one reference is inlined back into its parent (rule utility).
*/
const (
	EventAppend = "append"
	EventCreate = "create"
	EventReuse  = "reuse"
	EventInline = "inline"
)

/*
GrammarEvent is one structural step Sequitur took. Rule is the rule created,
reused or inlined; Into is the rule the step rewrote (the owner of the
substituted occurrence, or the parent an inlined rule dissolved into); Digram
is the repeated pair, labelled as in RuleLabel.
*/
type GrammarEvent struct {
	Kind   string    `json:"kind"`
	Token  string    `json:"token,omitempty"`
	Rule   uint32    `json:"rule"`
	Into   uint32    `json:"into"`
	From   uint32    `json:"from"`
	Digram [2]string `json:"digram"`
}

/*
Trace makes the grammar record every GrammarEvent until Drain takes them. It
is off by default so batch training never accumulates events nobody reads.
*/
func (grammar *Grammar) Trace() *Grammar {
	grammar.trace = true
	return grammar
}

/*
Drain returns the events recorded since the previous Drain.
*/
func (grammar *Grammar) Drain() []GrammarEvent {
	events := grammar.events
	grammar.events = nil
	return events
}

func (grammar *Grammar) record(event GrammarEvent) {
	if grammar.trace {
		grammar.events = append(grammar.events, event)
	}
}

/*
RuleLabel names a rule reference M<id>; terminals keep their own names (the
region tokens are R<nn>, so rules cannot reuse the R prefix).
*/
func RuleLabel(id uint32) string {
	return fmt.Sprintf("M%d", id)
}

/*
label is the display name of one symbol: RuleLabel for a rule reference, the
terminal name otherwise.
*/
func (grammar *Grammar) label(symbol *Symbol) string {
	if symbol.rule != nil {
		return RuleLabel(symbol.rule.id)
	}

	return grammar.Name(symbol.value)
}

/*
Symbols labels the right-hand side of rule in order.
*/
func (grammar *Grammar) Symbols(rule *Rule) []string {
	labels := make([]string, 0)

	for current := rule.guard.next; current != rule.guard && current != nil; current = current.next {
		labels = append(labels, grammar.label(current))
	}

	return labels
}

/*
Check verifies Sequitur's two invariants over the whole grammar and returns
every violation: digram uniqueness (no digram occurs twice, except the
overlapping pair in a run like "a a a") and rule utility (every rule other
than R0 is referenced at least twice).
*/
func (grammar *Grammar) Check() []string {
	violations := make([]string, 0)
	seen := make(map[digramKey]*Symbol)

	for _, rule := range append([]*Rule{grammar.startRule}, grammar.Rules()...) {
		for current := rule.guard.next; current != rule.guard && current != nil; current = current.next {
			if current.next == nil || current.next.isGuard {
				continue
			}

			key := makeDigramKey(current, current.next)

			if prior, found := seen[key]; found && prior.next != current {
				violations = append(violations, fmt.Sprintf(
					"digram uniqueness: %s %s occurs in %s and %s",
					grammar.label(current), grammar.label(current.next),
					RuleLabel(prior.owner.id), RuleLabel(current.owner.id),
				))
				continue
			}

			seen[key] = current
		}

		if rule.id > 0 && rule.count < 2 {
			violations = append(violations, fmt.Sprintf(
				"rule utility: %s is referenced %d time(s)", RuleLabel(rule.id), rule.count,
			))
		}
	}

	return violations
}

/*
NewGrammar instantiates a new empty Sequitur grammar with root rule R0.
*/
func NewGrammar() *Grammar {
	grammar := &Grammar{
		rules:       make(map[uint32]*Rule),
		digrams:     make(map[digramKey]*Symbol),
		names:       make(map[uint32]string),
		symbolCodes: make(map[string]uint32),
		nextRuleID:  1,
		nextCodeID:  1,
	}

	startGuard := &Symbol{
		isGuard: true,
	}

	startGuard.prev = startGuard
	startGuard.next = startGuard

	startRule := &Rule{
		id:    0,
		guard: startGuard,
	}

	startGuard.owner = startRule

	grammar.startRule = startRule
	grammar.rules[0] = startRule

	return grammar
}

/*
Append adds a named terminal symbol to the sequence and induces any repeating
rules on the fly. Returns the deepest rule that just completed, or nil.
*/
func (grammar *Grammar) Append(name string) *Rule {
	code := grammar.codeForName(name)
	return grammar.AppendCode(code)
}

/*
AppendCode adds an integer terminal symbol to the sequence and induces rules.
*/
func (grammar *Grammar) AppendCode(code uint32) *Rule {
	newSymbol := &Symbol{
		owner: grammar.startRule,
		value: code,
	}

	grammar.record(GrammarEvent{Kind: EventAppend, Token: grammar.Name(code)})

	lastSymbol := grammar.startRule.guard.prev
	grammar.join(lastSymbol, newSymbol)
	grammar.join(newSymbol, grammar.startRule.guard)

	grammar.checkDigram(lastSymbol)

	activeSymbol := grammar.startRule.guard.prev

	if activeSymbol == nil || activeSymbol.isGuard {
		return nil
	}

	if activeSymbol.rule != nil {
		return activeSymbol.rule
	}

	return nil
}

/*
Rules returns all induced rules (id > 0) in ascending rule ID order.
*/
func (grammar *Grammar) Rules() []*Rule {
	ruleList := make([]*Rule, 0, len(grammar.rules))

	for ruleID, rule := range grammar.rules {
		if ruleID == 0 {
			continue
		}

		ruleList = append(ruleList, rule)
	}

	slices.SortFunc(ruleList, func(leftRule, rightRule *Rule) int {
		return cmp.Compare(leftRule.id, rightRule.id)
	})

	return ruleList
}

/*
Rule returns the rule with the given ID, or nil if not found.
*/
func (grammar *Grammar) Rule(ruleID uint32) *Rule {
	return grammar.rules[ruleID]
}

/*
StartRule returns the root sequence rule R0.
*/
func (grammar *Grammar) StartRule() *Rule {
	return grammar.startRule
}

/*
Name returns the human-readable label for a terminal code.
*/
func (grammar *Grammar) Name(code uint32) string {
	if label, found := grammar.names[code]; found {
		return label
	}

	return fmt.Sprintf("T%d", code)
}

/*
String formats the entire grammar into readable productions.
*/
func (grammar *Grammar) String() string {
	lines := make([]string, 0, len(grammar.rules))
	lines = append(lines, grammar.startRule.String(grammar))

	for _, rule := range grammar.Rules() {
		lines = append(lines, rule.String(grammar))
	}

	return strings.Join(lines, "\n")
}

/*
ID returns the production rule ID.
*/
func (rule *Rule) ID() uint32 {
	return rule.id
}

/*
Count returns the reference count of this rule across the grammar.
*/
func (rule *Rule) Count() int {
	return rule.count
}

/*
Length returns the number of symbols on this rule's right-hand side.
*/
func (rule *Rule) Length() int {
	symbolCount := 0

	for currentSymbol := rule.guard.next; currentSymbol != rule.guard && currentSymbol != nil; currentSymbol = currentSymbol.next {
		symbolCount++
	}

	return symbolCount
}

/*
Depth returns the tree depth of this rule (1 for terminal-only rules).
*/
func (rule *Rule) Depth() int {
	maxChildDepth := 0

	for currentSymbol := rule.guard.next; currentSymbol != rule.guard && currentSymbol != nil; currentSymbol = currentSymbol.next {
		if currentSymbol.rule == nil {
			continue
		}

		childDepth := currentSymbol.rule.Depth()

		if childDepth > maxChildDepth {
			maxChildDepth = childDepth
		}
	}

	return maxChildDepth + 1
}

/*
Terminals flattens the rule into its terminal symbol codes.
*/
func (rule *Rule) Terminals() []uint32 {
	terminals := make([]uint32, 0)
	rule.collectTerminals(&terminals)
	return terminals
}

func (rule *Rule) collectTerminals(terminals *[]uint32) {
	for currentSymbol := rule.guard.next; currentSymbol != rule.guard && currentSymbol != nil; currentSymbol = currentSymbol.next {
		if currentSymbol.rule != nil {
			currentSymbol.rule.collectTerminals(terminals)
			continue
		}

		*terminals = append(*terminals, currentSymbol.value)
	}
}

/*
TerminalNames flattens the rule into its terminal string names.
*/
func (rule *Rule) TerminalNames(grammar *Grammar) []string {
	names := make([]string, 0)
	rule.collectTerminalNames(grammar, &names)
	return names
}

func (rule *Rule) collectTerminalNames(grammar *Grammar, names *[]string) {
	for currentSymbol := rule.guard.next; currentSymbol != rule.guard && currentSymbol != nil; currentSymbol = currentSymbol.next {
		if currentSymbol.rule != nil {
			currentSymbol.rule.collectTerminalNames(grammar, names)
			continue
		}

		*names = append(*names, grammar.Name(currentSymbol.value))
	}
}

/*
String formats the rule as R_id -> [sym1, sym2, ...].
*/
func (rule *Rule) String(grammar *Grammar) string {
	parts := make([]string, 0)

	for currentSymbol := rule.guard.next; currentSymbol != rule.guard && currentSymbol != nil; currentSymbol = currentSymbol.next {
		if currentSymbol.rule != nil {
			parts = append(parts, fmt.Sprintf("R%d", currentSymbol.rule.id))
			continue
		}

		parts = append(parts, grammar.Name(currentSymbol.value))
	}

	return fmt.Sprintf("R%d -> [%s]", rule.id, strings.Join(parts, ", "))
}

func (grammar *Grammar) codeForName(name string) uint32 {
	if code, found := grammar.symbolCodes[name]; found {
		return code
	}

	code := grammar.nextCodeID
	grammar.nextCodeID++
	grammar.symbolCodes[name] = code
	grammar.names[code] = name
	return code
}

func (grammar *Grammar) createRule() *Rule {
	ruleID := grammar.nextRuleID
	grammar.nextRuleID++

	guard := &Symbol{
		isGuard: true,
	}
	guard.prev = guard
	guard.next = guard

	refHead := &Symbol{
		isGuard: true,
	}
	refHead.prevRef = refHead
	refHead.nextRef = refHead

	rule := &Rule{
		id:      ruleID,
		guard:   guard,
		refHead: refHead,
	}
	guard.owner = rule

	grammar.rules[ruleID] = rule
	return rule
}

func (grammar *Grammar) join(leftSymbol, rightSymbol *Symbol) {
	if leftSymbol.next != nil {
		grammar.deleteDigram(leftSymbol)
	}

	if rightSymbol.prev != nil && rightSymbol.prev != leftSymbol {
		grammar.deleteDigram(rightSymbol.prev)
	}

	leftSymbol.next = rightSymbol
	rightSymbol.prev = leftSymbol
}

func (grammar *Grammar) deleteDigram(symbol *Symbol) {
	if symbol == nil || symbol.isGuard || symbol.next == nil || symbol.next.isGuard {
		return
	}

	key := makeDigramKey(symbol, symbol.next)
	matchSymbol, found := grammar.digrams[key]

	if found && matchSymbol == symbol {
		delete(grammar.digrams, key)
	}
}

func (grammar *Grammar) checkDigram(symbol *Symbol) bool {
	if symbol == nil || symbol.isGuard || symbol.next == nil || symbol.next.isGuard {
		return false
	}

	key := makeDigramKey(symbol, symbol.next)
	matchSymbol, found := grammar.digrams[key]

	if !found {
		grammar.digrams[key] = symbol
		return false
	}

	if matchSymbol == symbol {
		return false
	}

	if matchSymbol.next == symbol {
		return false
	}

	grammar.resolveRepeat(symbol, matchSymbol)
	return true
}

func (grammar *Grammar) resolveRepeat(symbol, matchSymbol *Symbol) {
	digram := [2]string{grammar.label(symbol), grammar.label(symbol.next)}

	if matchSymbol.owner.id != 0 && matchSymbol.prev.isGuard && matchSymbol.next.next.isGuard {
		grammar.record(GrammarEvent{
			Kind: EventReuse, Rule: matchSymbol.owner.id, Into: symbol.owner.id, Digram: digram,
		})
		grammar.substitute(symbol, matchSymbol.owner)
		return
	}

	newRule := grammar.createRule()
	grammar.record(GrammarEvent{
		Kind: EventCreate, Rule: newRule.id, Into: symbol.owner.id, From: matchSymbol.owner.id, Digram: digram,
	})

	firstCopy := grammar.cloneSymbol(matchSymbol, newRule)
	secondCopy := grammar.cloneSymbol(matchSymbol.next, newRule)

	grammar.join(newRule.guard, firstCopy)
	grammar.join(firstCopy, secondCopy)
	grammar.join(secondCopy, newRule.guard)

	key := makeDigramKey(firstCopy, secondCopy)
	grammar.digrams[key] = firstCopy

	grammar.substitute(matchSymbol, newRule)
	grammar.substitute(symbol, newRule)
}

func (grammar *Grammar) substitute(symbol *Symbol, targetRule *Rule) *Symbol {
	prevSymbol := symbol.prev
	nextSymbol := symbol.next.next

	grammar.deleteDigram(prevSymbol)
	grammar.deleteDigram(symbol)
	grammar.deleteDigram(symbol.next)

	firstTarget := grammar.detachRuleRef(symbol)
	secondTarget := grammar.detachRuleRef(symbol.next)

	replacement := &Symbol{
		owner: symbol.owner,
	}
	grammar.attachRuleRef(replacement, targetRule)

	grammar.join(prevSymbol, replacement)
	grammar.join(replacement, nextSymbol)

	if firstTarget != nil && firstTarget.id > 0 && firstTarget.count == 1 {
		grammar.inlineRule(firstTarget)
	}

	if secondTarget != nil && secondTarget != firstTarget && secondTarget.id > 0 && secondTarget.count == 1 {
		grammar.inlineRule(secondTarget)
	}

	reduced := grammar.checkDigram(prevSymbol)

	if !reduced && replacement.next != nil && !replacement.next.isGuard {
		grammar.checkDigram(replacement)
	}

	return replacement
}

func (grammar *Grammar) inlineRule(rule *Rule) {
	if rule.id == 0 || rule.count != 1 {
		return
	}

	refSymbol := rule.refHead.nextRef

	if refSymbol == nil || refSymbol == rule.refHead {
		return
	}

	parentRule := refSymbol.owner
	grammar.record(GrammarEvent{Kind: EventInline, Rule: rule.id, Into: parentRule.id})
	refPrev := refSymbol.prev
	refNext := refSymbol.next

	grammar.deleteDigram(refPrev)
	grammar.deleteDigram(refSymbol)

	firstSymbol := rule.guard.next
	lastSymbol := rule.guard.prev

	rule.guard.next = rule.guard
	rule.guard.prev = rule.guard

	childRules := make([]*Rule, 0)

	for currentSymbol := firstSymbol; currentSymbol != nil; currentSymbol = currentSymbol.next {
		currentSymbol.owner = parentRule

		if currentSymbol.rule != nil {
			childRules = append(childRules, currentSymbol.rule)
		}

		if currentSymbol == lastSymbol {
			break
		}
	}

	firstSymbol.prev = nil
	lastSymbol.next = nil

	refSymbol.prevRef.nextRef = refSymbol.nextRef
	refSymbol.nextRef.prevRef = refSymbol.prevRef
	refSymbol.prevRef = nil
	refSymbol.nextRef = nil
	refSymbol.rule = nil
	rule.count--

	grammar.join(refPrev, firstSymbol)
	grammar.join(lastSymbol, refNext)

	delete(grammar.rules, rule.id)

	for _, childRule := range childRules {
		if childRule.id > 0 && childRule.count == 1 {
			grammar.inlineRule(childRule)
		}
	}

	reduced := grammar.checkDigram(refPrev)

	if !reduced && lastSymbol.next != nil && !lastSymbol.next.isGuard {
		grammar.checkDigram(lastSymbol)
	}
}

func (grammar *Grammar) cloneSymbol(sourceSymbol *Symbol, ownerRule *Rule) *Symbol {
	cloned := &Symbol{
		owner: ownerRule,
		value: sourceSymbol.value,
	}

	if sourceSymbol.rule != nil {
		grammar.attachRuleRef(cloned, sourceSymbol.rule)
	}

	return cloned
}

func (grammar *Grammar) attachRuleRef(symbol *Symbol, targetRule *Rule) {
	symbol.rule = targetRule
	symbol.prevRef = targetRule.refHead
	symbol.nextRef = targetRule.refHead.nextRef
	targetRule.refHead.nextRef.prevRef = symbol
	targetRule.refHead.nextRef = symbol
	targetRule.count++
}

func (grammar *Grammar) detachRuleRef(symbol *Symbol) *Rule {
	targetRule := symbol.rule

	if targetRule == nil {
		return nil
	}

	symbol.prevRef.nextRef = symbol.nextRef
	symbol.nextRef.prevRef = symbol.prevRef
	symbol.prevRef = nil
	symbol.nextRef = nil
	symbol.rule = nil
	targetRule.count--
	return targetRule
}

func makeSymbolID(symbol *Symbol) uint64 {
	if symbol.rule != nil {
		return (uint64(1) << 63) | uint64(symbol.rule.id)
	}

	return uint64(symbol.value)
}

func makeDigramKey(leftSymbol, rightSymbol *Symbol) digramKey {
	return digramKey{
		leftID:  makeSymbolID(leftSymbol),
		rightID: makeSymbolID(rightSymbol),
	}
}

/*
serializedSymbol represents JSON-serializable symbol state.
*/
type serializedSymbol struct {
	IsRule bool   `json:"is_rule"`
	RuleID uint32 `json:"rule_id,omitempty"`
	Value  uint32 `json:"value,omitempty"`
}

/*
serializedRule represents JSON-serializable rule state.
*/
type serializedRule struct {
	ID      uint32             `json:"id"`
	Symbols []serializedSymbol `json:"symbols"`
}

/*
serializedGrammar represents full JSON-serializable grammar snapshot.
*/
type serializedGrammar struct {
	NextRuleID uint32             `json:"next_rule_id"`
	NextCodeID uint32             `json:"next_code_id"`
	Names      map[uint32]string  `json:"names"`
	Rules      []serializedRule   `json:"rules"`
	Start      []serializedSymbol `json:"start"`
}

/*
MarshalJSON serializes the grammar into JSON.
*/
func (grammar *Grammar) MarshalJSON() ([]byte, error) {
	snapshot := serializedGrammar{
		NextRuleID: grammar.nextRuleID,
		NextCodeID: grammar.nextCodeID,
		Names:      grammar.names,
		Rules:      make([]serializedRule, 0, len(grammar.rules)),
		Start:      serializeSymbols(grammar.startRule),
	}

	for _, rule := range grammar.Rules() {
		snapshot.Rules = append(snapshot.Rules, serializedRule{
			ID:      rule.id,
			Symbols: serializeSymbols(rule),
		})
	}

	body, err := json.Marshal(snapshot)

	if err != nil {
		return nil, errnie.Error(errnie.Err(errnie.Internal, "[grammar] serialization failed", err))
	}

	return body, nil
}

func serializeSymbols(rule *Rule) []serializedSymbol {
	symbols := make([]serializedSymbol, 0)

	for currentSymbol := rule.guard.next; currentSymbol != rule.guard && currentSymbol != nil; currentSymbol = currentSymbol.next {
		entry := serializedSymbol{}

		if currentSymbol.rule != nil {
			entry.IsRule = true
			entry.RuleID = currentSymbol.rule.id
		}

		if currentSymbol.rule == nil {
			entry.Value = currentSymbol.value
		}

		symbols = append(symbols, entry)
	}

	return symbols
}

/*
UnmarshalJSON deserializes a JSON grammar snapshot into this Grammar.
*/
func (grammar *Grammar) UnmarshalJSON(buf []byte) error {
	snapshot := serializedGrammar{}

	if err := json.Unmarshal(buf, &snapshot); err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "[grammar] deserialization failed", err))
	}

	grammar.rules = make(map[uint32]*Rule)
	grammar.digrams = make(map[digramKey]*Symbol)
	grammar.names = snapshot.Names

	if grammar.names == nil {
		grammar.names = make(map[uint32]string)
	}

	grammar.symbolCodes = make(map[string]uint32, len(grammar.names))

	for code, label := range grammar.names {
		grammar.symbolCodes[label] = code
	}

	grammar.nextRuleID = snapshot.NextRuleID
	grammar.nextCodeID = snapshot.NextCodeID

	grammar.startRule = &Rule{
		id: 0,
		guard: &Symbol{
			isGuard: true,
		},
	}
	grammar.startRule.guard.prev = grammar.startRule.guard
	grammar.startRule.guard.next = grammar.startRule.guard
	grammar.startRule.guard.owner = grammar.startRule
	grammar.rules[0] = grammar.startRule

	for _, ruleData := range snapshot.Rules {
		guard := &Symbol{
			isGuard: true,
		}
		guard.prev = guard
		guard.next = guard

		refHead := &Symbol{
			isGuard: true,
		}
		refHead.prevRef = refHead
		refHead.nextRef = refHead

		rule := &Rule{
			id:      ruleData.ID,
			guard:   guard,
			refHead: refHead,
		}
		guard.owner = rule
		grammar.rules[ruleData.ID] = rule
	}

	for _, ruleData := range snapshot.Rules {
		targetRule := grammar.rules[ruleData.ID]

		for _, symbolData := range ruleData.Symbols {
			newSymbol := &Symbol{
				owner: targetRule,
				value: symbolData.Value,
			}

			if symbolData.IsRule {
				referencedRule := grammar.rules[symbolData.RuleID]

				if referencedRule == nil {
					return errnie.Error(errnie.Err(
						errnie.Validation,
						fmt.Sprintf("[grammar] unknown referenced rule ID %d", symbolData.RuleID),
						nil,
					))
				}

				grammar.attachRuleRef(newSymbol, referencedRule)
			}

			lastSymbol := targetRule.guard.prev
			grammar.join(lastSymbol, newSymbol)
			grammar.join(newSymbol, targetRule.guard)
		}
	}

	for _, symbolData := range snapshot.Start {
		newSymbol := &Symbol{
			owner: grammar.startRule,
			value: symbolData.Value,
		}

		if symbolData.IsRule {
			referencedRule := grammar.rules[symbolData.RuleID]

			if referencedRule == nil {
				return errnie.Error(errnie.Err(
					errnie.Validation,
					fmt.Sprintf("[grammar] unknown referenced rule ID %d in start rule", symbolData.RuleID),
					nil,
				))
			}

			grammar.attachRuleRef(newSymbol, referencedRule)
		}

		lastSymbol := grammar.startRule.guard.prev
		grammar.join(lastSymbol, newSymbol)
		grammar.join(newSymbol, grammar.startRule.guard)
	}

	for _, rule := range grammar.rules {
		for currentSymbol := rule.guard.next; currentSymbol != rule.guard && currentSymbol != nil; currentSymbol = currentSymbol.next {
			if currentSymbol.next != rule.guard && currentSymbol.next != nil {
				key := makeDigramKey(currentSymbol, currentSymbol.next)
				grammar.digrams[key] = currentSymbol
			}
		}
	}

	return nil
}
