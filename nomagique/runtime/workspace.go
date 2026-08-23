package runtime

/*
Workspace is a runtime object that provides an ergonomic way to
handle highly concurrent workloads.
*/
type Workspace struct {
}

func NewWorkspace() *Workspace {
	return &Workspace{}
}
