package perspectives

/*
DrivePerspective is the simplest tradable playbook: enter when executed order flow
shows a one-sided drive (CVD's AggressiveDrive) or hidden accumulation
(HiddenAbsorption) clearing that signal's own noise floor. It only authorizes
entries; the trade desk owns the exit via the same global perspective pass with
ObservationHolding.
*/
type DrivePerspective struct {
	Tree *Tree
}

/*
NewDrivePerspective builds the drive entry playbook.
*/
func NewDrivePerspective() *DrivePerspective {
	return &DrivePerspective{
		Tree: &Tree{Branches: []Branch{
			snrBranch(CategoryAggressiveDrive, ActionEnter),
			snrBranch(CategoryHiddenAbsorption, ActionEnter),
		}},
	}
}

func (drive *DrivePerspective) Walk(measurements []Measurement) Perspective {
	if drive.Tree.Walk(measurements, nil) == nil {
		return nil
	}

	return drive
}

/*
Decide returns ActionEnter when a drive reading clears the floor, else nil. The
drive playbook has no observation-gated leaves, so observations do not change its
verdict; the parameter exists to satisfy the unified entry/exit Perspective contract.
*/
func (drive *DrivePerspective) Decide(
	measurements []Measurement,
	observations []ObservationType,
) *ActionType {
	return drive.Tree.Walk(measurements, observations)
}

func (drive *DrivePerspective) Regime() Regime {
	return RegimeTrending
}

func (drive *DrivePerspective) Confidence() float64 {
	return 0.0
}
