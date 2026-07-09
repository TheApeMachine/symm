package strategy

/*
PostMortem manages the process of evaluating a Thesis after the fact.
This is the final stage in the Thesis life-cycle and can only be performed
once the underlying trade has been successfuly exited.
*/
type PostMortem struct {
	thesis *Thesis
}

/*
NewPostMortem wraps a Thesis and ground truth so they can be used for analysis.
*/
func NewPostMortem(thesis *Thesis) *PostMortem {
	return &PostMortem{
		thesis: thesis,
	}
}

/*
Analyze the performance of the Thesis compared to ground truth to derive what
held up, what did not, and which adjustments should be made to sharpen the
decision making process.
*/
func (pm *PostMortem) Analyze() *PostMortem {
	return pm
}
