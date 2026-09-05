package vector

/*
Reading is the unpacked result of one classification: which class won, how
confidently, how ambiguous the distribution was, and the mass by class label.
*/
type Reading struct {
	Ready         bool
	WinnerIndex   int
	WinnerLabel   string
	Confidence    float64
	Ambiguity     float64
	Sharpness     float64
	Maturity      float64
	Probabilities map[string]float64
	// Unscored names the declared classes this observation carried incomplete
	// evidence for, which therefore took no part in the comparison. A reading
	// drawn from two of five classes is a different statement from one drawn
	// from all five, and the probabilities alone cannot say which it is.
	Unscored []string
}

/*
Read unpacks the current distribution into a labelled reading. An unready
distribution yields an unready Reading, so an absent classification is never
mistaken for a uniform one.
*/
func (classifier *Classifier) Read() Reading {
	distribution := classifier.Distribution()

	if !distribution.Ready() {
		return Reading{}
	}

	groups := classifier.Groups()

	// Only the classes that were scored appear in the distribution, so a
	// position in it is not a position in the declared class list. This is the
	// mapping back.
	indices := classifier.logits.ReadyIndices()

	unscored := make([]string, 0)

	for index, group := range groups {
		scored := false

		for _, ready := range indices {
			if ready == index {
				scored = true

				break
			}
		}

		if !scored {
			unscored = append(unscored, group.Label)
		}
	}

	reading := Reading{
		Unscored:      unscored,
		Ready:         true,
		WinnerIndex:   distribution.Winner(),
		Confidence:    float64(distribution.Confidence()),
		Ambiguity:     float64(distribution.Ambiguity()),
		Sharpness:     float64(distribution.Sharpness()),
		Maturity:      float64(classifier.Maturity()),
		Probabilities: make(map[string]float64, len(groups)),
	}

	if reading.WinnerIndex >= 0 && reading.WinnerIndex < len(indices) {
		reading.WinnerIndex = indices[reading.WinnerIndex]
	}

	if reading.WinnerIndex >= 0 && reading.WinnerIndex < len(groups) {
		reading.WinnerLabel = groups[reading.WinnerIndex].Label
	}

	for position, index := range indices {
		if index < 0 || index >= len(groups) {
			continue
		}

		reading.Probabilities[groups[index].Label] = float64(
			distribution.Probability(position),
		)
	}

	return reading
}

/*
Probability returns the mass assigned to one class label.
*/
func (reading Reading) Probability(label string) (float64, bool) {
	value, found := reading.Probabilities[label]

	return value, found
}
