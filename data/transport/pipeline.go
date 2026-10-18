package transport

import (
	"io"

	"github.com/theapemachine/errnie"
)

/*
Pipeline manages a chain of io.ReadWriteCloser components.

It connects components together so data flows through all components in sequence.
Each component can produce data independently.
*/
type Pipeline struct {
	components []io.ReadWriteCloser
	processed  bool
}

/*
NewPipeline creates a pipeline connecting io.ReadWriteCloser components.

It connects components together so data written to the pipeline flows through
all components in sequence.

Example:

	// Simple pipeline
	p := workflow.NewPipeline(message, agent, provider)
	io.Copy(os.Stdout, p)

	// Nested pipelines
	p1 := workflow.NewPipeline(message, agent, provider)
	p2 := workflow.NewPipeline(message, agent, provider, p1)
	io.Copy(os.Stdout, p2)
*/
func NewPipeline(components ...io.ReadWriteCloser) io.ReadWriteCloser {
	return &Pipeline{components: components}
}

/*
Read copies each stage into the next one, then reads the last stage.
An empty frame ends the pass. The next Write starts a new one.
*/
func (pipeline *Pipeline) Read(p []byte) (n int, err error) {
	if len(pipeline.components) == 0 {
		return 0, io.EOF
	}

	if !pipeline.processed {
		for i := range len(pipeline.components) - 1 {
			n, err = Copy(pipeline.components[i+1], pipeline.components[i])

			if err == io.EOF || (err == nil && n == 0) {
				return 0, io.EOF
			}

			if err != nil {
				return 0, errnie.Error(err)
			}
		}

		pipeline.processed = true
	}

	n, err = pipeline.components[len(pipeline.components)-1].Read(p)

	if err == io.EOF || (err == nil && n == 0) {
		pipeline.processed = false

		if err == nil {
			return 0, io.EOF
		}

		return n, err
	}

	if err != nil {
		return n, errnie.Error(err)
	}

	return n, nil
}

/*
Write implements the io.Writer interface.

It writes data to the first component in the pipeline.
Note that writing is optional - components can produce data independently.
*/
func (pipeline *Pipeline) Write(p []byte) (n int, err error) {
	if len(pipeline.components) == 0 {
		return len(p), nil
	}

	pipeline.processed = false
	return pipeline.components[0].Write(p)
}

/*
Close closes every stage in the pipeline.
*/
func (pipeline *Pipeline) Close() error {
	for _, component := range pipeline.components {
		if err := component.Close(); err != nil {
			return errnie.Error(err)
		}
	}

	return nil
}
