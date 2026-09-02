package runtime

import (
	"context"
	"errors"
	"slices"
)

/* Ingress accepts values from a streaming producer under an explicit lifecycle. */
type Ingress[T any] interface {
	Push(T)
	Status() *Status
}

/*
Workspace is a ring whose staged Nodes may themselves be Workload rings.

The outer disruptor supplies concurrency and stage barriers. A nested Workload
does not need an attachment, forwarding graph, or scheduler: it is simply a
Node in the Workspace stage declaration.
*/
type Workspace[T any] struct {
	*Workload[T]
	ingress  []*Workload[T]
	children []*Workload[T]
	err      error
}

/* NewWorkspace constructs the outer ring from the declared Node stages. */
func NewWorkspace[T any](
	ctx context.Context,
	name string,
	stages [][]Node[T],
) *Workspace[T] {
	workspace := &Workspace[T]{}

	if len(stages) == 0 || len(stages[0]) == 0 {
		workspace.err = errors.New("runtime: workspace requires at least one ingress node")

		return workspace
	}

	if len(stages[0]) > 255 {
		workspace.err = errors.New("runtime: workspace exceeds 255 concurrent writers")

		return workspace
	}

	seen := map[*Workload[T]]bool{}

	for stageIndex, stage := range stages {
		if len(stage) == 0 {
			workspace.err = errors.Join(
				workspace.err,
				errors.New("runtime: workspace stage is empty"),
			)

			continue
		}

		for _, node := range stage {
			workload, nested := node.(*Workload[T])

			if !nested && stageIndex == 0 {
				workspace.err = errors.Join(
					workspace.err,
					errors.New("runtime: workspace ingress node must be a workload"),
				)

				continue
			}

			if !nested {
				continue
			}

			if seen[workload] {
				workspace.err = errors.Join(
					workspace.err,
					errors.New("runtime: workload appears more than once"),
				)

				continue
			}

			seen[workload] = true
			workspace.children = append(workspace.children, workload)
			workspace.err = errors.Join(workspace.err, workload.Error())

			if stageIndex == 0 {
				workspace.ingress = append(workspace.ingress, workload)
			}
		}
	}

	// stages[0] holds the ingress Workloads, which are this ring's writers
	// rather than its handler groups — so the outer ring is built from the
	// remainder. A Composed node declared directly in a Workspace stage
	// therefore reports a stage index one lower than its position in the
	// declaration above; the ordering between such nodes is unaffected, which
	// is all a stage index is read for.
	workspace.Workload = newWorkload(ctx, name, stages[1:], uint8(len(stages[0])))
	workspace.err = errors.Join(workspace.err, workspace.Workload.Error())

	for _, ingress := range workspace.ingress {
		ingress.connect(workspace.Workload)
	}

	return workspace
}

/* Admit opens every inner ring before opening the outer ingress ring. */
func (workspace *Workspace[T]) Admit() {
	if workspace == nil || workspace.err != nil {
		return
	}

	workspace.Workload.admit()

	for _, child := range workspace.children {
		child.admit()
	}
}

func (workspace *Workspace[T]) Close() error {
	if workspace == nil {
		return nil
	}

	err := workspace.err

	for _, ingress := range workspace.ingress {
		err = errors.Join(err, ingress.Close())
	}

	if workspace.Workload != nil {
		err = errors.Join(err, workspace.Workload.Close())
	}

	for _, child := range workspace.children {
		if slices.Contains(workspace.ingress, child) {
			continue
		}

		err = errors.Join(err, child.Close())
	}

	return err
}

func (workspace *Workspace[T]) Error() error {
	if workspace == nil {
		return errors.New("runtime: workspace is nil")
	}

	return workspace.err
}
