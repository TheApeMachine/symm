package catalog

/*
Port is one wired connection of a primitive: a stream it reads, or the stream it produces.
*/
type Port struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

/*
Schema is the catalog entry for a single primitive behavior.
*/
type Schema struct {
	Kind        string `json:"kind"`
	Category    string `json:"category"`
	Op          string `json:"op"`
	Name        string `json:"name"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Package     string `json:"package"`
	Builder     string `json:"builder"`
	Inputs         []Port `json:"inputs"`
	Outputs        []Port `json:"outputs"`
	ParamCount     int    `json:"paramCount,omitempty"`
	TypeParamCount int    `json:"typeParamCount,omitempty"`
}
