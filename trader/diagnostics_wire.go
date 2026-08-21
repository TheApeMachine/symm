package trader

import wire "github.com/theapemachine/symm/telemetry/generated/telemetry"

/*
Wire projects the diagnostics snapshot into its explicit binary schema.
*/
func (diagnostics StreamDiagnostics) Wire() *wire.DiagnosticsFrameT {
	stages := make([]*wire.DiagnosticClockT, len(diagnostics.Stages))

	for index, stage := range diagnostics.Stages {
		stages[index] = &wire.DiagnosticClockT{
			Name: stage.Name, Count: stage.Count, TotalNs: stage.TotalNs,
			LastNs: stage.LastNs, MaxNs: stage.MaxNs, LastAtNs: stage.LastAtNs,
			Active: stage.Active, StartedNs: stage.StartedNs,
		}
	}

	hops := make([]*wire.DiagnosticHopT, len(diagnostics.Hops))

	for index, hop := range diagnostics.Hops {
		hops[index] = &wire.DiagnosticHopT{
			From: hop.From, To: hop.To, Count: hop.Count, TotalNs: hop.TotalNs,
			LastNs: hop.LastNs, MaxNs: hop.MaxNs,
		}
	}

	queues := make([]*wire.DiagnosticQueueT, len(diagnostics.Queues))

	for index, queue := range diagnostics.Queues {
		queues[index] = &wire.DiagnosticQueueT{
			Name: queue.Name, Kind: queue.Kind, Writers: queue.Writers,
			Readers: queue.Readers, Depth: queue.Depth, Capacity: queue.Cap,
			HighWater: queue.HighWater, Symbols: queue.Symbols,
		}
	}

	errors := make([]*wire.DiagnosticErrorT, len(diagnostics.Errors))

	for index, diagnosticErr := range diagnostics.Errors {
		errors[index] = &wire.DiagnosticErrorT{
			Source: diagnosticErr.Source, Message: diagnosticErr.Message,
			Caller: diagnosticErr.Caller, AtNs: diagnosticErr.AtNs,
		}
	}

	goroutines := make([]*wire.DiagnosticGoroutineT, len(diagnostics.Goroutines))

	for index, goroutine := range diagnostics.Goroutines {
		goroutines[index] = &wire.DiagnosticGoroutineT{
			Owner: goroutine.Owner, Count: goroutine.Count, State: goroutine.State,
		}
	}

	return &wire.DiagnosticsFrameT{
		Status:    diagnostics.Status,
		Enabled:   diagnostics.Enabled,
		AtNs:      diagnostics.AtNs,
		StartedNs: diagnostics.StartedNs,
		Stages:    stages,
		Hops:      hops,
		Queues:    queues,
		Errors:    errors,
		Pass: &wire.DiagnosticPassT{
			State: diagnostics.Pass.State, InFlightNs: diagnostics.Pass.InFlightNs,
			LastPassNs:  diagnostics.Pass.LastPassNs,
			SinceLastNs: diagnostics.Pass.SinceLastNs,
		},
		Goroutines: goroutines,
	}
}
