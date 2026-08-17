package learning

import "gonum.org/v1/gonum/mat"

type resonanceWorkspace struct {
	xCol                *mat.VecDense
	yCol                *mat.VecDense
	predictions         []*mat.VecDense
	errors              []*mat.VecDense
	grads               []*mat.VecDense
	topPrior            *mat.VecDense
	temporalError       *mat.VecDense
	temporalSignal      *mat.VecDense
	temporalWeightedErr *mat.VecDense
	taskPred            *mat.VecDense
	taskError           *mat.VecDense
	taskSignal          *mat.VecDense
	weightedErr         []*mat.VecDense
	belowSignal         []*mat.VecDense
	correction          []*mat.VecDense
	localSignal         []*mat.VecDense
	weightUpdate        []*mat.Dense
	recProposal         []*mat.VecDense
	recError            []*mat.VecDense
	recSignal           []*mat.VecDense
	recUpdate           []*mat.Dense
	temporalUpdate      *mat.Dense
	bottomUp            []*mat.VecDense
	topDown             []*mat.VecDense
	savedStates         []*mat.VecDense
	stepBuf             []*mat.VecDense
	reconPred           *mat.VecDense
	reconDiff           *mat.VecDense
	temporalSVD         mat.SVD
	svdValues           []float64
}

func newResonanceWorkspace(arch []int, targetDim int) *resonanceWorkspace {
	numLinks := len(arch) - 1
	topDim := arch[len(arch)-1]

	workspace := &resonanceWorkspace{
		xCol:                mat.NewVecDense(arch[0], nil),
		predictions:         make([]*mat.VecDense, numLinks),
		errors:              make([]*mat.VecDense, numLinks),
		grads:               make([]*mat.VecDense, len(arch)),
		topPrior:            mat.NewVecDense(topDim, nil),
		temporalError:       mat.NewVecDense(topDim, nil),
		temporalSignal:      mat.NewVecDense(topDim, nil),
		temporalWeightedErr: mat.NewVecDense(topDim, nil),
		bottomUp:            make([]*mat.VecDense, len(arch)),
		topDown:             make([]*mat.VecDense, len(arch)),
		savedStates:         make([]*mat.VecDense, len(arch)),
		stepBuf:             make([]*mat.VecDense, len(arch)),
		weightedErr:         make([]*mat.VecDense, numLinks),
		belowSignal:         make([]*mat.VecDense, numLinks),
		correction:          make([]*mat.VecDense, len(arch)),
		localSignal:         make([]*mat.VecDense, numLinks),
		weightUpdate:        make([]*mat.Dense, numLinks),
		recProposal:         make([]*mat.VecDense, numLinks),
		recError:            make([]*mat.VecDense, numLinks),
		recSignal:           make([]*mat.VecDense, numLinks),
		recUpdate:           make([]*mat.Dense, numLinks),
		reconPred:           mat.NewVecDense(arch[0], nil),
		reconDiff:           mat.NewVecDense(arch[0], nil),
		temporalUpdate:      mat.NewDense(topDim, topDim, nil),
		svdValues:           make([]float64, topDim),
	}

	for layerIndex, layerDim := range arch {
		workspace.bottomUp[layerIndex] = mat.NewVecDense(layerDim, nil)
		workspace.topDown[layerIndex] = mat.NewVecDense(layerDim, nil)
		workspace.savedStates[layerIndex] = mat.NewVecDense(layerDim, nil)
		workspace.stepBuf[layerIndex] = mat.NewVecDense(layerDim, nil)
		workspace.correction[layerIndex] = mat.NewVecDense(layerDim, nil)

		if layerIndex > 0 {
			workspace.grads[layerIndex] = mat.NewVecDense(layerDim, nil)
		}
	}

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
