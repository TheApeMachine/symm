package signal

/*
MetricSemantics is one metric's declared identity.
*/
type MetricSemantics struct {
	Identity     string `json:"identity"`
	Source       string `json:"source"`
	Metric       string `json:"metric"`
	Class        string `json:"class,omitempty"`
	Role         string `json:"role,omitempty"`
	Purpose      string `json:"purpose,omitempty"`
	Definedness  string `json:"definedness,omitempty"`
	Destinations string `json:"destinations,omitempty"`
	Forbidden    string `json:"forbidden,omitempty"`
	Status       string `json:"status,omitempty"`
}

/*
SignalSemantics is one signal family's declared identity.
*/
type SignalSemantics struct {
	Source  string `json:"source"`
	Purpose string `json:"purpose"`
}

/*
MetricMap is the decoded map keyed by declared identity ("source/metric").
*/
type MetricMap struct {
	BaselineCommit string                     `json:"baselineCommit"`
	Metrics        map[string]MetricSemantics `json:"metrics"`
	Signals        map[string]SignalSemantics `json:"signals"`
}

/*
Semantics returns the declared semantic map.
*/
func Semantics() MetricMap {
	return MetricMap{
		Metrics: make(map[string]MetricSemantics),
		Signals: make(map[string]SignalSemantics),
	}
}
