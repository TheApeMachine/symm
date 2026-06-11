#import "solver_private.h"

@implementation ManifoldSolver (CoherencePrivate)

- (id<MTLComputePipelineState>)pipelineNamed:(NSString *)name error:(NSError **)error {
    id<MTLFunction> function = [self.library newFunctionWithName:name];

    if (function == nil) {
        if (error != nil) {
            *error = [NSError errorWithDomain:@"manifold" code:1 userInfo:@{NSLocalizedDescriptionKey: [NSString stringWithFormat:@"kernel %@ not found", name]}];
        }

        return nil;
    }

    return [self.device newComputePipelineStateWithFunction:function error:error];
}

- (void)configureCoherenceParams {
    CoherenceParamsHost *params = (CoherenceParamsHost *)self.coherenceParams.contents;
    float ampStats[4];

    [self runReduceFloatStats:self.oscAmp length:self.numOsc statsOut:ampStats];

    params->num_osc = self.numOsc;
    params->max_carriers = self.config.max_carriers;
    params->num_carriers = self.numOsc;
    params->dt = self.config.dt;
    params->coupling_scale = self.config.coupling_scale;
    params->carrier_reg = 0.0f;
    params->rng_seed = 1;
    params->conflict_threshold = 0.5f;
    params->offender_weight_floor = 1e-6f;
    params->gate_width_min = self.config.gate_width_min;
    params->gate_width_max = self.config.gate_width_max;
    params->ema_alpha = ampStats[2] / (ampStats[0] + ampStats[2] + self.config.rho_min);
    params->recenter_alpha = params->ema_alpha * 0.1f;
    params->mode = 0;
    params->anchor_random_eps = 0.0f;
    params->stable_amp_threshold = 0.5f;
    params->crystallize_amp_threshold = 0.8f;
    params->crystallize_conflict_threshold = 0.2f;
    params->crystallize_age = 8;
    params->crystallized_coupling_boost = 1.0f;
    params->volatile_decay_mul = 1.0f;
    params->stable_decay_mul = 1.0f;
    params->crystallized_decay_mul = 1.0f;
    params->topdown_phase_scale = 0.0f;
    params->topdown_energy_scale = 0.0f;
    params->topdown_random_energy_eps = 0.0f;
    params->repulsion_scale = 0.0f;
    params->domain_x = self.config.domain_x;
    params->domain_y = self.config.domain_y;
    params->domain_z = self.config.domain_z;
    params->spatial_sigma = self.config.domain_x / (float)self.config.grid_x;
    params->metabolic_rate = self.config.metabolic_rate;
}

- (void)configureGPEParams {
    GPEParamsHost *params = (GPEParamsHost *)self.gpeParams.contents;
    float dOmega = 2.0f * (float)M_PI / fmaxf((float)self.numOsc, 1.0f);

    params->dt = self.config.dt;
    params->hbar_eff = self.config.hbar_eff;
    params->mass_eff = 1.0f;
    params->g_interaction = self.config.g_interaction;
    params->energy_decay = self.config.energy_decay;
    params->chemical_potential = 0.0f;
    params->inv_domega2 = 1.0f / (dOmega * dOmega);
    params->anchors = 8;
    params->rng_seed = 1;
    params->anchor_eps = 0.0f;
}

- (void)clearAccumulators {
    [self runClearCarrierAccums:self.numOsc];
}

- (uint32_t)deriveNumBins {
    BinParamsHost *binParams = (BinParamsHost *)self.binParams.contents;
    float *omegaData = (float *)self.modeOmega.contents;
    uint32_t maxBin = 0;

    for (uint32_t index = 0; index < self.numOsc; index++) {
        float binFloat = (omegaData[index] - binParams->omega_min) * binParams->inv_bin_width;
        int binIndex = (int)floorf(binFloat);

        if (binIndex < 0) {
            continue;
        }

        if ((uint32_t)binIndex > maxBin) {
            maxBin = (uint32_t)binIndex;
        }
    }

    return maxBin + 1u;
}

- (BOOL)runExclusiveScan:(NSString **)error {
    return [self runExclusiveScanU32:self.binCounts
                              output:self.binStarts
                              length:self.numBins
                      writeTotalSlot:NO
                               error:error];
}

- (BOOL)runCoherenceBinning:(NSString **)error {
    *(uint32_t *)self.omegaMinKey.contents = 0xFFFFFFFFu;
    *(uint32_t *)self.omegaMaxKey.contents = 0u;
    [self runClearU32:self.binCounts count:self.config.max_carriers];

    [self dispatchGridKernel:self.reduceOmegaMinMax
                     buffers:@[self.modeOmega, self.numCarriers, self.omegaMinKey, self.omegaMaxKey]
                 threadCount:self.numOsc];

    [self dispatchGridKernel:self.computeBinParams
                     buffers:@[self.omegaMinKey, self.omegaMaxKey, self.numCarriers, self.binParams, self.gateWidthMaxBuf]
                 threadCount:1];

    self.numBins = [self deriveNumBins];

    if (self.numBins == 0) {
        if (error != nil) {
            *error = @"coherence binning produced zero bins";
        }

        return NO;
    }

    if (self.numBins > self.config.max_carriers) {
        if (error != nil) {
            *error = @"coherence bin count exceeds max_carriers";
        }

        return NO;
    }

    *(uint32_t *)self.numBinsBuf.contents = self.numBins;

    [self dispatchGridKernel:self.binCountCarriers
                     buffers:@[self.modeOmega, self.numCarriers, self.binCounts, self.binParams, self.numBinsBuf]
                 threadCount:self.numOsc];

    if (![self runExclusiveScan:error]) {
        return NO;
    }

    [self runCopyU32:self.binStarts dst:self.binOffsets count:self.numBins];

    [self dispatchGridKernel:self.binScatterCarriers
                     buffers:@[self.modeOmega, self.numCarriers, self.binOffsets, self.binParams, self.numBinsBuf, self.carrierBinnedIdx]
                 threadCount:self.numOsc];

    return YES;
}

- (BOOL)runCoherenceStep:(NSString **)error {
    if (self.numOsc == 0) {
        return YES;
    }

    CoherenceParamsHost *params = (CoherenceParamsHost *)self.coherenceParams.contents;

    [self configureCoherenceParams];
    [self configureGPEParams];
    [self clearAccumulators];

    if (![self runCoherenceBinning:error]) {
        return NO;
    }

    *(uint32_t *)self.numCarriers.contents = self.numOsc;

    [self dispatchGridKernel:self.precomputeCarrierAnchorPositions
                     buffers:@[self.particlePos, self.modeAnchorIdx, self.modeAnchorPos, self.modeAnchorWeight, self.numCarriers]
                 threadCount:(NSUInteger)self.numOsc * kModeAnchors];

    [self dispatchGridKernel:self.prepOscillatorCoupling
                     buffers:@[
                         self.oscPhase, self.oscOmega, self.oscAmp, self.oscHeat,
                         self.oscCouplingPrep, self.coherenceParams, self.binParams, self.numBinsBuf
                     ]
                 threadCount:self.numOsc];

    NSUInteger tgWidth = manifold_simd_threadgroup_width(
        self.numOsc,
        self.simdWidth,
        self.maxThreadsPerThreadgroup
    );
    NSUInteger tgCount = 1;

    if (self.numOsc > tgWidth) {
        tgCount = (self.numOsc + tgWidth - 1) / tgWidth;
    }

    for (uint32_t tileBase = 0; tileBase < self.numOsc; tileBase += kCarrierTileSize) {
        uint32_t tileCount = self.numOsc - tileBase;

        if (tileCount > kCarrierTileSize) {
            tileCount = kCarrierTileSize;
        }

        params->carrier_tile_base = tileBase;
        params->carrier_tile_count = tileCount;

        NSUInteger tgAccumBytes = (NSUInteger)tileCount * kCarrierAccumThreadgroupBytes;

        [self dispatchThreadgroupKernel:self.accumulateForces
                                buffers:@[
                                    self.oscOmega, self.particlePos,
                                    self.modeOmega, self.modeGate, self.modeAnchorWeight,
                                    self.accums, self.coherenceParams, self.numCarriers,
                                    self.binStarts, self.carrierBinnedIdx, self.numBinsBuf,
                                    self.modeAnchorPos, self.oscCouplingPrep
                                ]
                          threadgroupSize:tgWidth
                         threadgroupCount:tgCount
                 threadgroupMemoryLength:tgAccumBytes];
    }

    [self dispatchGridKernel:self.gpeStep
                     buffers:@[
                         self.oscPhase, self.oscOmega, self.oscAmp,
                         self.modeReal, self.modeImag,
                         self.modeOmega, self.modeGate,
                         self.modeAnchorIdx, self.modeAnchorWeight,
                         self.accums, self.numCarriers, self.particlePos,
                         self.coherenceParams, self.gpeParams
                     ]
                 threadCount:self.numOsc];

    [self dispatchGridKernel:self.updatePhases
                     buffers:@[
                         self.oscPhase, self.oscOmega, self.oscAmp,
                         self.modeReal, self.modeImag, self.modeOmega, self.modeGate,
                         self.modeAnchorWeight,
                         self.numCarriers, self.coherenceParams,
                         self.binStarts, self.carrierBinnedIdx, self.binParams, self.numBinsBuf,
                         self.particlePos, self.modeAnchorPos, self.oscCouplingPrep
                     ]
                 threadCount:self.numOsc];

    return YES;
}

@end
