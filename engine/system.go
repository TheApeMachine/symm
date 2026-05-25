package engine

/*
System is one schedulable unit in the booter loop.
Start opens resources, Tick drains only what the system cares about, Close releases them.
*/
type System interface {
	Start() error
	State() State
	Tick() error
	Close() error
}

/*
Passive is embedded by systems that only work when another system drives their Measure path.
*/
type Passive struct{}

func (passive *Passive) Start() error {
	return nil
}

func (passive *Passive) State() State {
	return READY
}

func (passive *Passive) Close() error {
	return nil
}

func (passive *Passive) Tick() error {
	return nil
}
