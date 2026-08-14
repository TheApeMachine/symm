package types

type Group struct {
	ID           string        `json:"id,omitempty"`
	Name         string        `json:"name,omitempty"`
	Measurements []Measurement `json:"measurements,omitempty"`
}
