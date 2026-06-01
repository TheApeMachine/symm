package numeric

/*
ProjectOut remaps the observation vector between pipeline stages. Prefer
NewProjectScalar when the stage emits one fused value — it avoids a heap
allocation per Push.
*/
type ProjectOut func(out float64, values []float64) []float64

/*
ProjectScalar remaps observations into one scalar without allocating a slice.
*/
type ProjectScalar func(out float64, values []float64) float64

type Project struct {
	vector ProjectOut
	scalar ProjectScalar
}

func NewProject(project ProjectOut) *Project {
	return &Project{vector: project}
}

/*
NewProjectScalar builds a zero-alloc projection stage for single-output fusions.
*/
func NewProjectScalar(scalar ProjectScalar) *Project {
	return &Project{scalar: scalar}
}

func (project *Project) Next(out float64, values ...float64) (float64, error) {
	if project.vector == nil {
		return 0, nil
	}

	selected := project.vector(out, values)

	if len(selected) == 0 {
		return 0, nil
	}

	result := selected[0]

	for _, value := range selected[1:] {
		result *= value
	}

	return result, nil
}

func (project *Project) Reset() error {
	return nil
}
