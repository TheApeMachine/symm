package transport

import (
	"io"

	"github.com/theapemachine/errnie"
)

/*
Feedback implements a bidirectional data flow mechanism that allows simultaneous forward
and backward streaming of data in a workflow pipeline. It wraps io.ReadWriter and io.Writer
interfaces to create a tee-based streaming pattern.

The type is particularly useful in scenarios where:
- Data needs to be processed in a forward direction while maintaining a copy
- Responses need to be captured and sent backwards in the pipeline
- LLM (Language Learning Model) responses need to be stored in an agent's context

Structure:
- forward: Primary data channel that implements both reading and writing
- backward: Secondary channel for writing copies of the data
- tee: A TeeReader that automatically copies data from forward to backward during reads

```go

	pipeline := workflow.NewPipeline(
	    agent,                // Input source
	    workflow.NewFeedback(
	        algo,         // Forward stream (algo)
	        data,         // Backward stream (data)
	    ),
	    converter,        // Output destination
	)

	// When data flows through this pipeline:
	// 1. Algo sends data to the data
	// 2. Data streams forward to the algo
	// 3. Simultaneously, the data is fed back to the algo
	// 4. Algo processes the data for algo

```
*/
type Feedback struct {
	forward  io.ReadWriter
	backward io.Writer
	tee      io.Reader
}

/*
NewFeedback creates a new Feedback instance that manages bidirectional data flow.

Parameters:
  - forward: Primary ReadWriter for the main data flow
  - backward: Writer for the copy/feedback stream

Returns:
  - *Feedback: Configured Feedback instance with tee reading set up
*/
func NewFeedback(forward io.ReadWriter, backward io.Writer) *Feedback {
	return &Feedback{
		forward:  forward,
		backward: backward,
		tee:      io.TeeReader(forward, backward),
	}
}

/*
Read implements io.Reader. It reads from the tee reader, which automatically
copies all read data to the backward writer while returning it from the
forward reader.

Parameters:
  - p: Byte slice to read data into

Returns:
  - n: Number of bytes read
  - err: Any error that occurred during reading
*/
func (feedback *Feedback) Read(p []byte) (n int, err error) {
	return feedback.tee.Read(p)
}

/*
Write implements io.Writer. It writes data to the forward writer and updates
the tee reader to reflect the new content.

Parameters:
  - p: Byte slice containing data to write

Returns:
  - n: Number of bytes written
  - err: Any error that occurred during writing
*/
func (feedback *Feedback) Write(p []byte) (n int, err error) {
	// Reset the tee with the updated forward component after writing
	if n, err = feedback.forward.Write(p); err != nil {
		return n, errnie.Error(err)
	}

	return n, nil
}

/*
Close implements io.Closer. It attempts to close both forward and backward
components if they implement io.Closer.

Returns:
  - error: Any error that occurred while closing either component
*/
func (feedback *Feedback) Close() error {
	// Close the forward component if it implements io.Closer
	if closer, ok := feedback.forward.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			return errnie.Error(err)
		}
	}

	// Close the backward component if it implements io.Closer
	if closer, ok := feedback.backward.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			return errnie.Error(err)
		}
	}

	return nil
}
