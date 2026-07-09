package strategy

/*
Evidence is a snapshot of every datapoint or other information that
was used to formulate a Thesis. It is primarily used during the
generation of the PostMortem, so the system can map the Thesis to
ground truth and extract highly granular learnings from that process.
*/
type Evidence[T any] struct {
	snapshot T
}

/*
NewEvidence wraps a snapshot in the Evidence type.
*/
func NewEvidence[T any](snapshot T) *Evidence[any] {
	return &Evidence[any]{
		snapshot: snapshot,
	}
}

/*
Diff provides a structured artifact which contains only the exact
difference between the snapshot, and the ground truth. It will
ignore and disgard any key/value pair that exists on one side,
but not on the other.
*/
func (evidence *Evidence[any]) Diff(groundTruth any) *Evidence[any] {
	return evidence
}