package store

import "gonum.org/v1/gonum/mat"

type resonanceWorkspace struct {
	xCol *mat.VecDense

	predictions  []*mat.VecDense
	errors       []*mat.VecDense
	belowSignal  []*mat.VecDense
	localSignal  []*mat.VecDense
	weightUpdate []*mat.Dense

	recProposal []*mat.VecDense
	recError    []*mat.VecDense
	recSignal   []*mat.VecDense
	recUpdate   []*mat.Dense

	// Multi-timescale temporal buffers per latent layer
	temporalPriors  []*mat.VecDense
	temporalErrors  []*mat.VecDense
	temporalSignals []*mat.VecDense
	temporalUpdates []*mat.Dense
	prevLatents     []*mat.VecDense

	// Inference settling buffers
	grads       []*mat.VecDense
	bottomUp    []*mat.VecDense
	savedStates []*mat.VecDense
	stepBuf     []*mat.VecDense
	correction  []*mat.VecDense

	// Diagnostics & reconstruction
	reconPred *mat.VecDense
	reconDiff *mat.VecDense

	// Multi-layer & innovation readout buffers
	readoutBuf *mat.VecDense
	taskPred   *mat.VecDense
}

/*
newResonanceWorkspace sizes every scratch buffer the settle loop reuses.

readout must be the same mode the manifold harvests with: the readout buffer
and the task weights are multiplied together, so a workspace sized for a
different mode is a dimension mismatch at the first prediction.
*/
func newResonanceWorkspace(
	arch []int,
	taskRows int,
	readout resonanceReadout,
) *resonanceWorkspace {
	numLinks := len(arch) - 1
	numLatents := len(arch) - 1

	totalLatentDim := 0
	for _, dim := range arch[1:] {
		totalLatentDim += dim
	}
	totalErrorDim := 0
	for _, dim := range arch[:len(arch)-1] {
		totalErrorDim += dim
	}
	readoutDim := 0

	switch readout {
	case readoutAll:
		readoutDim = totalLatentDim + totalErrorDim
	case readoutLatents:
		readoutDim = totalLatentDim
	case readoutInnovations:
		readoutDim = totalErrorDim
	}

	workspace := &resonanceWorkspace{
		xCol:            mat.NewVecDense(arch[0], nil),
		predictions:     make([]*mat.VecDense, numLinks),
		errors:          make([]*mat.VecDense, numLinks),
		belowSignal:     make([]*mat.VecDense, numLinks),
		localSignal:     make([]*mat.VecDense, numLinks),
		weightUpdate:    make([]*mat.Dense, numLinks),
		recProposal:     make([]*mat.VecDense, numLinks),
		recError:        make([]*mat.VecDense, numLinks),
		recSignal:       make([]*mat.VecDense, numLinks),
		recUpdate:       make([]*mat.Dense, numLinks),
		temporalPriors:  make([]*mat.VecDense, numLatents),
		temporalErrors:  make([]*mat.VecDense, numLatents),
		temporalSignals: make([]*mat.VecDense, numLatents),
		temporalUpdates: make([]*mat.Dense, numLatents),
		prevLatents:     make([]*mat.VecDense, numLatents),
		bottomUp:        make([]*mat.VecDense, len(arch)),
		savedStates:     make([]*mat.VecDense, len(arch)),
		stepBuf:         make([]*mat.VecDense, len(arch)),
		correction:      make([]*mat.VecDense, len(arch)),
		grads:           make([]*mat.VecDense, len(arch)),
		reconPred:       mat.NewVecDense(arch[0], nil),
		reconDiff:       mat.NewVecDense(arch[0], nil),
		readoutBuf:      mat.NewVecDense(readoutDim, nil),
	}

	for layerIndex, layerDim := range arch {
		workspace.bottomUp[layerIndex] = mat.NewVecDense(layerDim, nil)
		workspace.savedStates[layerIndex] = mat.NewVecDense(layerDim, nil)
		workspace.stepBuf[layerIndex] = mat.NewVecDense(layerDim, nil)
		workspace.correction[layerIndex] = mat.NewVecDense(layerDim, nil)

		if layerIndex > 0 {
			workspace.grads[layerIndex] = mat.NewVecDense(layerDim, nil)
			latentIndex := layerIndex - 1
			workspace.temporalPriors[latentIndex] = mat.NewVecDense(layerDim, nil)
			workspace.temporalErrors[latentIndex] = mat.NewVecDense(layerDim, nil)
			workspace.temporalSignals[latentIndex] = mat.NewVecDense(layerDim, nil)
			workspace.temporalUpdates[latentIndex] = mat.NewDense(layerDim, layerDim, nil)
			workspace.prevLatents[latentIndex] = mat.NewVecDense(layerDim, nil)
		}
	}

	for linkIndex := range numLinks {
		rowDim := arch[linkIndex]
		colDim := arch[linkIndex+1]

		workspace.predictions[linkIndex] = mat.NewVecDense(rowDim, nil)
		workspace.errors[linkIndex] = mat.NewVecDense(rowDim, nil)
		workspace.belowSignal[linkIndex] = mat.NewVecDense(rowDim, nil)
		workspace.localSignal[linkIndex] = mat.NewVecDense(rowDim, nil)
		workspace.weightUpdate[linkIndex] = mat.NewDense(rowDim, colDim, nil)
		workspace.recProposal[linkIndex] = mat.NewVecDense(colDim, nil)
		workspace.recError[linkIndex] = mat.NewVecDense(colDim, nil)
		workspace.recSignal[linkIndex] = mat.NewVecDense(colDim, nil)
		workspace.recUpdate[linkIndex] = mat.NewDense(colDim, rowDim, nil)
	}

	if taskRows > 0 {
		workspace.taskPred = mat.NewVecDense(taskRows, nil)
	}

	return workspace
}
