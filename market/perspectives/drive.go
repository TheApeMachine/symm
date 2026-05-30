package perspectives

// driveEntrySNR is the noise-floor multiple a drive reading must clear to enter:
// the SNR is now a real z-score, so 1.5 means one-and-a-half of the signal's own
// noise sigma above its baseline.
const driveEntrySNR = 1.5

/*
DrivePerspective is the simplest tradable playbook: enter when executed order flow
shows a strong, one-sided drive (CVD's AggressiveDrive) or hidden accumulation
(HiddenAbsorption) clearing the noise floor. It only authorizes entries; the trade
desk owns the exit via its protective stop and take-profit, so this perspective
needs no observation-gated branches (which Decide cannot supply anyway).
*/
type DrivePerspective struct {
	Tree *Tree
}

/*
NewDrivePerspective builds the drive entry playbook.
*/
func NewDrivePerspective() *DrivePerspective {
	return &DrivePerspective{
		Tree: &Tree{
			Branches: []Branch{
				{
					Category:  CategoryAggressiveDrive,
					Unit:      UnitSNR,
					Condition: ConditionIsGreaterThan,
					Value:     driveEntrySNR,
					Action:    ActionEnter,
				},
				{
					Category:  CategoryHiddenAbsorption,
					Unit:      UnitSNR,
					Condition: ConditionIsGreaterThan,
					Value:     driveEntrySNR,
					Action:    ActionEnter,
				},
			},
		},
	}
}

func (drive *DrivePerspective) Walk(measurements []Measurement) Perspective {
	if drive.Tree.Walk(measurements, nil) == nil {
		return nil
	}

	return drive
}

/*
Decide returns ActionEnter when a drive reading clears the floor, else nil.
*/
func (drive *DrivePerspective) Decide(measurements []Measurement) *ActionType {
	return drive.Tree.Walk(measurements, nil)
}

func (drive *DrivePerspective) Regime() Regime {
	return RegimeTrending
}

func (drive *DrivePerspective) Confidence() float64 {
	return 0.0
}
