package manifold

type fieldSolver interface {
	ReadRhoProjection() ([][]float64, error)
}

/*
FieldSnapshot is the raw post-step scalar field exported by the solver.
*/
type FieldSnapshot struct {
	Rho [][]float64 `json:"rho"`
}

/*
Read captures the solver-owned density projection without remapping it.
*/
func (field *FieldSnapshot) Read(solver fieldSolver) error {
	rho, err := solver.ReadRhoProjection()

	if err != nil {
		return err
	}

	field.Rho = rho
	return nil
}
