package numeric

type ProjectOut func(out float64, values []float64) []float64

type Project struct {
	project ProjectOut
}

func NewProject(project ProjectOut) *Project {
	return &Project{project: project}
}

func (project *Project) Next(out float64, values ...float64) (float64, error) {
	selected := project.project(out, values)

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
