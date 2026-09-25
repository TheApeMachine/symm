package store

import "bytes"

// receiveBoundary binds provenance to this input delivery. A later socket
// observation must never be used to timestamp an earlier metric publication.
func (server *GridServer) receiveBoundary(args Grid_write_Params) error {
	run, err := args.Run()

	if err != nil || run == "" {
		return boundaryError("grid: stamped input requires a run", err)
	}

	receipt, err := args.Receipt()

	if err != nil || len(receipt) == 0 {
		return boundaryError("grid: stamped input requires its source receipt", err)
	}

	inputs, err := args.Data()

	if err != nil {
		return boundaryError("grid: read stamped inputs", err)
	}

	count := 0

	for index := range inputs.Len() {
		payload, err := inputs.At(index)

		if err != nil {
			return boundaryError("grid: read stamped input", err)
		}

		if len(payload) > 0 {
			count++
		}
	}

	if count != 1 {
		return boundaryError("grid: one stamped input is required per boundary", nil)
	}

	server.run = run
	server.sequence = args.Sequence()
	server.receipt = bytes.Clone(receipt)
	return nil
}
