package strategy

/*
Thesis is the structure used by the trader to functionally reason
over the compressed and enriched raw market data. It drives the
ultimate decision the trader will make regarding how to enter,
and exit a trade. If at any point during the process the Thesis
breaks down, it is instantly disgarded and no more time is wasted
on it. Just because a Thesis is successfully created, does not
mean it will result in a trade. For that to happen, one or more
of the following needs to be true:

  - A slot must be available for it on the broker.Desk, or it needs to outweigh any
    current occupant of the broker.Desk slots to push a rotation.
  - Failing that, a reserve slow needs to be available, and the Thesis must be eligable
    by clearing the "opportunity" barrier.
  - It needs to outweigh any other Thesis created in the same iteration, or place second
    if multiple slots are available.
*/
type Thesis struct {
	graph    *Graph
	evidence map[string]*Evidence[any]
}

func NewThesis() *Thesis {
	return &Thesis{
		evidence: make(map[string]*Evidence[any]),
	}
}

/*
AddEvidence records a stage's snapshot on the Thesis under key.
*/
func (thesis *Thesis) AddEvidence(key string, snapshot any) {
	if thesis.evidence == nil {
		thesis.evidence = make(map[string]*Evidence[any])
	}

	thesis.evidence[key] = NewEvidence(snapshot)
}

/*
Evidence returns the snapshot recorded under key, and whether it was present.
*/
func (thesis *Thesis) Evidence(key string) (any, bool) {
	evidence, ok := thesis.evidence[key]

	if !ok {
		return nil, false
	}

	return evidence.snapshot, true
}

func (thesis *Thesis) Update() *Thesis {
	return thesis
}

func (thesis *Thesis) Clone() *Thesis {
	cloned := NewThesis()
	if thesis.graph != nil {
		cloned.graph = thesis.graph // Might need deep copy if graph is mutated later
	}
	for key, ev := range thesis.evidence {
		cloned.evidence[key] = NewEvidence(ev.snapshot)
	}
	return cloned
}
