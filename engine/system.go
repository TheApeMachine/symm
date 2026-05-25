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
