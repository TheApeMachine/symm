package sensorium

import _ "embed"

// PhysicsMonitorHTML is served at /physics by the actual Hub integration.
//
//go:embed viewer/physics.html
var PhysicsMonitorHTML string
