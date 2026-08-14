package hawkes

/*
Readiness separates directly observed arrival evidence from identified model
state. A successful fit is not forecast evidence until residual and
out-of-sample validation exists, while ModelUpdated identifies the events that
actually established a new parameter epoch.
*/
type Readiness struct {
	Observation  bool
	Intensity    bool
	HawkesFit    bool
	ModelUpdated bool
	Forecast     bool
	Reason       string
}
