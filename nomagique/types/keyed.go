package types

/*
Keyed routes the carrier to one sub-composition per key, constructing a fresh
one the first time a key is seen.

A stream that interleaves independent series — one per instrument, per venue,
per cohort member — is still one stream. Without this node every consumer
grows its own map of sub-states and its own construction branch, which puts
state and lifecycle back into the consumer and defeats the point of composing
a single pipeline.

Build supplies a new sub-composition for a key. Select names the key for the
current observation; it is a function rather than a slot because a key is an
identity, not a quantity, and the carrier only carries quantities.

Active() reads the sub-composition the last Step routed to, so a projection
reads the same branch that just advanced.

Degenerate behavior: an omitted Build or Select cannot route, and the node is
the identity on the carrier.
*/
type Keyed struct {
	Build  func() Node
	Select func() string

	tick *Tick

	branches map[string]Node
	active   Node
}

func (keyed *Keyed) Step(x Scalar) Scalar {
	keyed.active = nil

	if keyed.Build == nil || keyed.Select == nil {
		return x
	}

	key := keyed.Select()

	if keyed.branches == nil {
		keyed.branches = make(map[string]Node)
	}

	branch, found := keyed.branches[key]

	if !found {
		branch = keyed.Build()

		// A branch is a complete composition in its own right, built after the
		// enclosing pipeline was. Bind and resolve it here so a terminal
		// projection and any in-graph reference inside the branch are wired
		// against the branch they actually belong to.
		Bind(branch, keyed.tick)

		keyed.branches[key] = branch
	}

	keyed.active = branch

	return branch.Step(x)
}

/*
Bind attaches the observation counter every branch this node builds is
measured against.
*/
func (keyed *Keyed) Bind(tick *Tick) { keyed.tick = tick }

// Active returns the sub-composition the most recent Step routed to.
func (keyed *Keyed) Active() Node { return keyed.active }

// Len reports how many keys the node has constructed a branch for.
func (keyed *Keyed) Len() int { return len(keyed.branches) }

/*
Bind wires the deferred parts of one composition: a terminal stage learns the
graph it terminates, and an in-graph reference finds the quantity it names.

The top-level builder calls it for the composition it is given; Keyed calls it
for each branch it constructs, since those come into being later.
*/
func Bind(root Node, tick *Tick) {
	Walk(root, func(node Node) {
		if binder, ok := node.(interface{ Bind(Node) }); ok {
			binder.Bind(root)
		}

		if resolver, ok := node.(interface{ Resolve(Node) }); ok {
			resolver.Resolve(root)
		}

		// A stateful node reached by several paths of one graph must advance
		// once per observation, not once per path. Attaching the composition's
		// observation counter is what lets it tell the difference.
		if guarded, ok := node.(interface{ Bind(*Tick) }); ok {
			guarded.Bind(tick)
		}
	})
}

var _ Node = (*Keyed)(nil)
