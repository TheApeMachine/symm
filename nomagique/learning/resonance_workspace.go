package learning

import "gonum.org/v1/gonum/mat"

type resonanceWorkspace struct {
	xCol *mat.VecDense
	yCol *mat.VecDense

	predictions  []*mat.VecDense
	errors       []*mat.VecDense
	weightedErr  []*mat.VecDense
	belowSignal  []*mat.VecDense
	localSignal  []*mat.VecDense
	weightUpdate []*mat.Dense

	recProposal []*mat.VecDense
	recError    []*mat.VecDense
	recSignal   []*mat.VecDense
	recUpdate   []*mat.Dense

	// Multi-timescale temporal buffers per latent layer
	temporalPriors       []*mat.VecDense
	temporalErrors       []*mat.VecDense
	temporalSignals      []*mat.VecDense
	temporalWeightedErrs []*mat.VecDense
	temporalUpdates      []*mat.Dense
	prevLatents          []*mat.VecDense
	temporalSVDs         []mat.SVD
	svdValues            []float64   // Top-layer SVD values slice for test/backwards-compatibility
	layerSVDValues       [][]float64 // Per-layer SVD values

	// Top-layer prior alias for test/backwards-compatibility
	topPrior *mat.VecDense

	// Inference settling buffers
	grads       []*mat.VecDense
	bottomUp    []*mat.VecDense
	topDown     []*mat.VecDense
	savedStates []*mat.VecDense
	stepBuf     []*mat.VecDense
	correction  []*mat.VecDense

	// Diagnostics & reconstruction
	reconPred *mat.VecDense
	reconDiff *mat.VecDense

	// Multi-layer & innovation readout buffers
	readoutBuf *mat.VecDense
	taskPred   *mat.VecDense
	taskError  *mat.VecDense
	taskSignal *mat.VecDense
}

func newResonanceWorkspace(arch []int, targetDim int) *resonanceWorkspace {
	numLinks := len(arch) - 1
	numLatents := len(arch) - 1
	topDim := arch[len(arch)-1]

	totalLatentDim := 0
	for _, dim := range arch[1:] {
		totalLatentDim += dim
	}
	totalErrorDim := 0
	for _, dim := range arch[:len(arch)-1] {
		totalErrorDim += dim
	}
	readoutDim := totalLatentDim + totalErrorDim

	workspace := &resonanceWorkspace{
		xCol:                 mat.NewVecDense(arch[0], nil),
		predictions:          make([]*mat.VecDense, numLinks),
		errors:               make([]*mat.VecDense, numLinks),
		weightedErr:          make([]*mat.VecDense, numLinks),
		belowSignal:          make([]*mat.VecDense, numLinks),
		localSignal:          make([]*mat.VecDense, numLinks),
		weightUpdate:         make([]*mat.Dense, numLinks),
		recProposal:          make([]*mat.VecDense, numLinks),
		recError:             make([]*mat.VecDense, numLinks),
		recSignal:            make([]*mat.VecDense, numLinks),
		recUpdate:            make([]*mat.Dense, numLinks),
		temporalPriors:       make([]*mat.VecDense, numLatents),
		temporalErrors:       make([]*mat.VecDense, numLatents),
		temporalSignals:      make([]*mat.VecDense, numLatents),
		temporalWeightedErrs: make([]*mat.VecDense, numLatents),
		temporalUpdates:      make([]*mat.Dense, numLatents),
		prevLatents:          make([]*mat.VecDense, numLatents),
		temporalSVDs:         make([]mat.SVD, numLatents),
		layerSVDValues:       make([][]float64, numLatents),
		svdValues:            make([]float64, topDim),
		bottomUp:             make([]*mat.VecDense, len(arch)),
		topDown:              make([]*mat.VecDense, len(arch)),
		savedStates:          make([]*mat.VecDense, len(arch)),
		stepBuf:              make([]*mat.VecDense, len(arch)),
		correction:           make([]*mat.VecDense, len(arch)),
		grads:                make([]*mat.VecDense, len(arch)),
		reconPred:            mat.NewVecDense(arch[0], nil),
		reconDiff:            mat.NewVecDense(arch[0], nil),
		readoutBuf:           mat.NewVecDense(readoutDim, nil),
	}

	for layerIndex, layerDim := range arch {
		workspace.bottomUp[layerIndex] = mat.NewVecDense(layerDim, nil)
		workspace.topDown[layerIndex] = mat.NewVecDense(layerDim, nil)
		workspace.savedStates[layerIndex] = mat.NewVecDense(layerDim, nil)
		workspace.stepBuf[layerIndex] = mat.NewVecDense(layerDim, nil)
		workspace.correction[layerIndex] = mat.NewVecDense(layerDim, nil)

		if layerIndex > 0 {
			workspace.grads[layerIndex] = mat.NewVecDense(layerDim, nil)
			latentIndex := layerIndex - 1
			workspace.temporalPriors[latentIndex] = mat.NewVecDense(layerDim, nil)
			workspace.temporalErrors[latentIndex] = mat.NewVecDense(layerDim, nil)
			workspace.temporalSignals[latentIndex] = mat.NewVecDense(layerDim, nil)
			workspace.temporalWeightedErrs[latentIndex] = mat.NewVecDense(layerDim, nil)
			workspace.temporalUpdates[latentIndex] = mat.NewDense(layerDim, layerDim, nil)
			workspace.prevLatents[latentIndex] = mat.NewVecDense(layerDim, nil)
			workspace.layerSVDValues[latentIndex] = make([]float64, layerDim)
		}
	}

	workspace.topPrior = workspace.temporalPriors[numLatents-1]

	for linkIndex := range numLinks {
		rowDim := arch[linkIndex]
		colDim := arch[linkIndex+1]

		workspace.predictions[linkIndex] = mat.NewVecDense(rowDim, nil)
		workspace.errors[linkIndex] = mat.NewVecDense(rowDim, nil)
		workspace.weightedErr[linkIndex] = mat.NewVecDense(rowDim, nil)
		workspace.belowSignal[linkIndex] = mat.NewVecDense(rowDim, nil)
		workspace.localSignal[linkIndex] = mat.NewVecDense(rowDim, nil)
		workspace.weightUpdate[linkIndex] = mat.NewDense(rowDim, colDim, nil)
		workspace.recProposal[linkIndex] = mat.NewVecDense(colDim, nil)
		workspace.recError[linkIndex] = mat.NewVecDense(colDim, nil)
		workspace.recSignal[linkIndex] = mat.NewVecDense(colDim, nil)
		workspace.recUpdate[linkIndex] = mat.NewDense(colDim, rowDim, nil)
	}

	if targetDim > 0 {
		workspace.yCol = mat.NewVecDense(targetDim, nil)
		workspace.taskPred = mat.NewVecDense(targetDim, nil)
		workspace.taskError = mat.NewVecDense(targetDim, nil)
		workspace.taskSignal = mat.NewVecDense(targetDim, nil)
	}

	return workspace
}
