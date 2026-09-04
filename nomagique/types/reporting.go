package types

/*
Reading is one metric a node publishes about itself: the label it is known by
and the value it currently holds.

Unit and Timescale travel with it because a number without them is not a
measurement, and the node that computed the quantity is the only place that
knows what the quantity is.
*/
type Reading struct {
	Label     string
	Unit      string
	Timescale string
	Value     Scalar

	// Defined marks a quantity the node cannot state yet — a return with no
	// prior price to move from. An undefined reading is omitted rather than
	// published as a zero that would read as a real measurement of no change.
	Defined bool
}

/*
Reporter is implemented by any node that publishes readings about its own
state. A terminal Projection harvests them by walking the composition, so a
node states what it measured and nothing outside it needs to know how.

Naming is the node's own responsibility: a primitive reports the general
quantity it computes, and a composition that wants domain names wraps it in a
Labelled node rather than reaching inside.
*/
type Reporter interface {
	Readings() []Reading
}

/*
Labelled renames the readings of the node it wraps, so one primitive can
appear several times in a composition under the names its consumer knows them
by, without the primitive itself carrying any domain vocabulary.

Names maps a reported label to the label it should be published under. A
reading whose label is absent from the map keeps its own name; an entry
mapping to the empty string suppresses it.
*/
type Labelled struct {
	Node  Node
	Names map[string]string

	// Prefix, when set, is prepended to every reading this node publishes
	// that the Names map did not already rename.
	Prefix string
}

func (labelled *Labelled) Step(x Scalar) Scalar {
	if labelled.Node == nil {
		return x
	}

	return labelled.Node.Step(x)
}

func (labelled *Labelled) Readings() []Reading {
	reporter, ok := labelled.Node.(Reporter)

	if !ok {
		return nil
	}

	source := reporter.Readings()
	renamed := make([]Reading, 0, len(source))

	for _, reading := range source {
		name, mapped := labelled.Names[reading.Label]

		if mapped && name == "" {
			continue
		}

		if !mapped {
			name = labelled.Prefix + reading.Label
		}

		reading.Label = name
		renamed = append(renamed, reading)
	}

	return renamed
}

/*
Composite is implemented by a node that holds other nodes in slots. Walk uses
it to reach the whole composition, so a node with slots is traversable without
the walk enumerating every type that exists.
*/
type Composite interface {
	Slots() []Node
}

/*
WalkInner visits the structure beneath a node without visiting that node's own
reporting surface, so a wrapper that republishes its child's readings is the
only place those readings are seen.
*/
func WalkInner(root Node, visit func(Node)) {
	switch node := root.(type) {
	case *Chain:
		Walk(node.A, visit)
		Walk(node.B, visit)
		Walk(node.C, visit)
		Walk(node.D, visit)
	case *Split:
		Walk(node.A, visit)
		Walk(node.B, visit)
		Walk(node.C, visit)
		Walk(node.D, visit)
	}
}

/*
Walk visits every node reachable from the root, depth first, so a terminal
stage can harvest the readings of everything upstream of it without the
composition being taken apart by hand.
*/
func Walk(root Node, visit func(Node)) {
	if root == nil {
		return
	}

	visit(root)

	switch node := root.(type) {
	case *Chain:
		Walk(node.A, visit)
		Walk(node.B, visit)
		Walk(node.C, visit)
		Walk(node.D, visit)
	case *Split:
		Walk(node.A, visit)
		Walk(node.B, visit)
		Walk(node.C, visit)
		Walk(node.D, visit)
	case *Labelled:
		// The wrapped node is reached through the Labelled, whose Readings
		// already republish it under the names the composition uses. Visiting
		// the child directly would leak its generic label into the
		// measurement and let a reference bind past the rename.
		WalkInner(node.Node, visit)
	case *Keyed:
		Walk(node.Active(), visit)
	default:
		// Every other node reaches its structure through its slots. A node
		// that holds slots reports them, so a reference nested anywhere in a
		// composition is reachable without this walk knowing every type.
		if composite, ok := root.(Composite); ok {
			for _, slot := range composite.Slots() {
				Walk(slot, visit)
			}
		}
	}
}

var (
	_ Node     = (*Labelled)(nil)
	_ Reporter = (*Labelled)(nil)
)

/*
Evidence is implemented by a node whose estimate carries its own confidence:
how many samples back it, how far the current observation sits from the
estimate, and the variance of the noise it is measured against.

A terminal Projection declares these facts rather than a signal-to-noise
ratio, so the one validated derivation stays in one place.
*/
type Evidence interface {
	Support() float64
	Divergence() Scalar
	NoiseVariance() Scalar
}

/*
Rejector is implemented by a node that can declare an observation unmeasurable.
A terminal Projection publishes the failure instead of readings derived from
input the composition could not use.
*/
type Rejector interface {
	Rejected() bool
}

/*
Report publishes the value of the node it wraps under a declared name, so a
derived quantity — a ratio, a difference, a product — becomes part of the
measurement without the primitive that computed it carrying any vocabulary.

It observes rather than advances: Step evaluates the wrapped node and passes
the carrier through unchanged, so wrapping a stage in a Report never alters
what the composition computes.

Defined, when set, gates publication on another node: a quantity that is not
yet stateable is absent rather than published as a zero that would read as a
real measurement.
*/
type Report struct {
	Label     string
	Unit      string
	Timescale string
	Value     Node
	Defined   Node

	reading Reading
}

func (report *Report) Step(x Scalar) Scalar {
	report.reading = Reading{
		Label:     report.Label,
		Unit:      report.Unit,
		Timescale: report.Timescale,
		Defined:   true,
	}

	if report.Value != nil {
		report.reading.Value = report.Value.Step(x)
	}

	if report.Defined != nil && report.Defined.Step(x) == 0 {
		report.reading.Defined = false
	}

	return x
}

func (report *Report) Readings() []Reading {
	return []Reading{report.reading}
}

// Slots exposes the nodes this report is composed of.
func (report *Report) Slots() []Node { return []Node{report.Value, report.Defined} }

var (
	_ Node     = (*Report)(nil)
	_ Reporter = (*Report)(nil)
)

/*
Require declares that the composition can only measure this observation when
the node in its When slot emits a non-zero value. A terminal Projection reads
the rejection and publishes the failure instead of readings derived from input
the composition could not use.

Degenerate behavior: an omitted When requires nothing, so a Require with no
condition never rejects.
*/
type Require struct {
	When Node

	rejected bool
}

func (require *Require) Step(x Scalar) Scalar {
	require.rejected = require.When != nil && require.When.Step(x) == 0

	return x
}

// Rejected reports whether the most recent observation failed the requirement.
func (require *Require) Rejected() bool { return require.rejected }

// Slots exposes the nodes this requirement is composed of.
func (require *Require) Slots() []Node { return []Node{require.When} }

var (
	_ Node     = (*Require)(nil)
	_ Rejector = (*Require)(nil)
)

/*
Ref reads a quantity another part of the same composition published under a
name, so a derived value can be expressed where it belongs without a local
variable carrying a pointer between the two places.

It resolves against the composition at construction time, the same walk that
binds a terminal projection, and it observes rather than advances: the node it
names is stepped by whoever owns it, and Ref only reads what that step
produced. A quantity is therefore counted exactly once no matter how many
places refer to it.

Degenerate behavior: a name nothing publishes resolves to nothing and the node
emits zero, which is the additive identity every operation degenerates to.
*/
type Ref struct {
	Name string

	source Reporter
}

/*
Resolve binds this reference to the Report publishing its name. The builder
calls it; a composition never does.
*/
func (ref *Ref) Resolve(root Node) {
	Walk(root, func(node Node) {
		reporter, ok := node.(Reporter)

		if !ok || node == Node(ref) {
			return
		}

		for _, reading := range reporter.Readings() {
			if reading.Label == ref.Name {
				ref.source = reporter

				return
			}
		}
	})
}

func (ref *Ref) Step(Scalar) Scalar {
	if ref.source == nil {
		return 0
	}

	for _, reading := range ref.source.Readings() {
		if reading.Label == ref.Name {
			return reading.Value
		}
	}

	return 0
}

var _ Node = (*Ref)(nil)
