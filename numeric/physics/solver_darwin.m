#import "solver_private.h"

@implementation ManifoldSolver

- (instancetype)initWithConfig:(const ManifoldConfig *)config
                 metallibBytes:(const void *)metallibBytes
                metallibLength:(size_t)metallibLength
                         error:(NSString **)error {
    self = [super init];

    if (self == nil) {
        return nil;
    }

    self.device = MTLCreateSystemDefaultDevice();

    if (self.device == nil) {
        if (error != nil) {
            *error = @"Metal device unavailable";
        }

        return nil;
    }

    self.queue = [self.device newCommandQueue];
    self.config = *config;
    self.numCells = config->grid_x * config->grid_y * config->grid_z;

    if (metallibBytes == NULL || metallibLength == 0) {
        if (error != nil) {
            *error = @"metallib payload is empty";
        }

        return nil;
    }

    NSError *metalError = nil;
    dispatch_data_t metallibData = dispatch_data_create(
        metallibBytes,
        metallibLength,
        dispatch_get_global_queue(QOS_CLASS_DEFAULT, 0),
        DISPATCH_DATA_DESTRUCTOR_DEFAULT
    );
    self.library = [self.device newLibraryWithData:metallibData error:&metalError];

    if (self.library == nil) {
        if (error != nil) {
            *error = metalError.localizedDescription ?: @"failed to load kernels.metallib";
        }

        return nil;
    }

    if (![self buildPipelines:error]) {
        return nil;
    }

    self.simdWidth = (uint32_t)self.accumulateForces.threadExecutionWidth;

    if (self.simdWidth == 0) {
        self.simdWidth = 32;
    }

    self.maxThreadsPerThreadgroup = self.device.maxThreadsPerThreadgroup.width;
    self.maxCarriersForTG = manifold_max_carriers_for_threadgroup(self.device);

    if (self.config.max_carriers > self.maxCarriersForTG) {
        if (error != nil) {
            *error = [NSString stringWithFormat:
                @"max_carriers %u exceeds device threadgroup capacity %u",
                self.config.max_carriers,
                self.maxCarriersForTG];
        }

        return nil;
    }

    if (![self allocateBuffers:error]) {
        return nil;
    }

    return self;
}

- (void)drainGPUQueue {
    id<MTLCommandBuffer> commandBuffer = [self.queue commandBuffer];
    [commandBuffer commit];
    [commandBuffer waitUntilCompleted];
}

- (void)dealloc {
    if (self.stepDispatchActive) {
        [self endStepDispatches];
    }

    [self drainGPUQueue];
}

- (BOOL)buildPipelines:(NSString **)error {
    NSError *pipelineError = nil;

    self.clearField = [self pipelineNamed:@"clear_field" error:&pipelineError];
    self.clearBufferU32 = [self pipelineNamed:@"clear_buffer_u32" error:&pipelineError];
    self.pipelineCopyBufferU32 = [self pipelineNamed:@"copy_buffer_u32" error:&pipelineError];
    self.pipelineCopyBufferFloat = [self pipelineNamed:@"copy_buffer_float" error:&pipelineError];
    self.pipelineCopyBitsToFloat = [self pipelineNamed:@"copy_bits_to_float" error:&pipelineError];
    self.scatterPrefixSeedLast = [self pipelineNamed:@"scatter_prefix_sum_seed_last" error:&pipelineError];
    self.clearCarrierAccums = [self pipelineNamed:@"clear_carrier_accums" error:&pipelineError];
    self.deriveMaxCarrierBin = [self pipelineNamed:@"derive_max_carrier_bin" error:&pipelineError];
    self.initOmegaScanKeys = [self pipelineNamed:@"init_omega_scan_keys" error:&pipelineError];
    self.gasComputePrimitives = [self pipelineNamed:@"gas_compute_primitives" error:&pipelineError];
    self.gasStage1 = [self pipelineNamed:@"gas_rk2_stage1" error:&pipelineError];
    self.gasStage2 = [self pipelineNamed:@"gas_rk2_stage2" error:&pipelineError];
    self.reduceOmegaMinMax = [self pipelineNamed:@"coherence_reduce_omega_minmax_keys" error:&pipelineError];
    self.computeBinParams = [self pipelineNamed:@"coherence_compute_bin_params" error:&pipelineError];
    self.binCountCarriers = [self pipelineNamed:@"coherence_bin_count_carriers" error:&pipelineError];
    self.binScatterCarriers = [self pipelineNamed:@"coherence_bin_scatter_carriers" error:&pipelineError];
    self.scanPass1 = [self pipelineNamed:@"exclusive_scan_u32_pass1" error:&pipelineError];
    self.scanAddBlockOffsets = [self pipelineNamed:@"exclusive_scan_u32_add_block_offsets" error:&pipelineError];
    self.scanFinalizeTotal = [self pipelineNamed:@"exclusive_scan_u32_finalize_total" error:&pipelineError];
    self.precomputeCarrierAnchorPositions = [self pipelineNamed:@"precompute_carrier_anchor_positions" error:&pipelineError];
    self.prepOscillatorCoupling = [self pipelineNamed:@"coherence_prep_oscillator_coupling" error:&pipelineError];
    self.accumulateForces = [self pipelineNamed:@"coherence_accumulate_forces" error:&pipelineError];
    self.gpeStep = [self pipelineNamed:@"coherence_gpe_step" error:&pipelineError];
    self.updatePhases = [self pipelineNamed:@"coherence_update_oscillator_phases" error:&pipelineError];
    self.scatterComputeCellIdx = [self pipelineNamed:@"scatter_compute_cell_idx" error:&pipelineError];
    self.scatterCountCells = [self pipelineNamed:@"scatter_count_cells" error:&pipelineError];
    self.scatterPrefixUpsweep = [self pipelineNamed:@"scatter_prefix_sum_upsweep" error:&pipelineError];
    self.scatterPrefixDownsweep = [self pipelineNamed:@"scatter_prefix_sum_downsweep" error:&pipelineError];
    self.scatterReorderParticles = [self pipelineNamed:@"scatter_reorder_particles" error:&pipelineError];
    self.scatterGatherCells = [self pipelineNamed:@"scatter_gather_cells" error:&pipelineError];
    self.picGatherUpdate = [self pipelineNamed:@"pic_gather_update_particles" error:&pipelineError];
    self.picGatherPilotWave = [self pipelineNamed:@"pic_gather_update_particles_pilot_wave" error:&pipelineError];
    self.projectModesToSpatialPsi = [self pipelineNamed:@"project_modes_to_spatial_psi" error:&pipelineError];
    self.particleInteractions = [self pipelineNamed:@"particle_interactions" error:&pipelineError];
    self.spatialHashAssign = [self pipelineNamed:@"spatial_hash_assign" error:&pipelineError];
    self.spatialHashScatter = [self pipelineNamed:@"spatial_hash_scatter" error:&pipelineError];
    self.spatialHashCollisions = [self pipelineNamed:@"spatial_hash_collisions" error:&pipelineError];
    self.reduceFloatStatsPass1 = [self pipelineNamed:@"reduce_float_stats_pass1" error:&pipelineError];
    self.reduceFloatStatsFinalize = [self pipelineNamed:@"reduce_float_stats_finalize" error:&pipelineError];
    self.generateParticlePositions = [self pipelineNamed:@"generate_particle_positions" error:&pipelineError];
    self.initializeParticleProperties = [self pipelineNamed:@"initialize_particle_properties" error:&pipelineError];
    self.coherenceFuseBinning = [self pipelineNamed:@"coherence_fuse_binning" error:&pipelineError];

    if (pipelineError != nil) {
        if (error != nil) {
            *error = pipelineError.localizedDescription;
        }

        return NO;
    }

    return YES;
}

- (BOOL)allocateBuffers:(NSString **)error {
    (void)error;
    size_t cellBytes = (size_t)self.numCells * sizeof(float);
    uint32_t maxModes = self.config.max_carriers;
    size_t gasPrimBytes = (size_t)self.numCells * 32;
    size_t particleCicBytes = (size_t)maxModes * 4 * sizeof(float);
    size_t gpuOnlyBytes = cellBytes * 4 + cellBytes + gasPrimBytes +
        (size_t)maxModes * kModeAnchors * 3 * sizeof(float) +
        (size_t)maxModes * 4 * sizeof(float) +
        particleCicBytes * 2;
    MTLHeapDescriptor *heapDescriptor = [[MTLHeapDescriptor alloc] init];
    heapDescriptor.size = gpuOnlyBytes + (4u << 20);
    heapDescriptor.storageMode = MTLStorageModePrivate;
    self.gpuHeap = [self.device newHeapWithDescriptor:heapDescriptor];

    self.momRho = [self newSharedBufferWithLength:cellBytes * 4];
    self.eInt = [self newSharedBufferWithLength:cellBytes];
    self.momRhoStage = [self newGPUBufferWithLength:cellBytes * 4];
    self.eStage = [self newGPUBufferWithLength:cellBytes];
    self.gasPrim = [self newGPUBufferWithLength:gasPrimBytes];
    self.particleCicA = [self newGPUBufferWithLength:particleCicBytes];
    self.particleCicB = [self newGPUBufferWithLength:particleCicBytes];
    self.gasParams = [self.device newBufferWithLength:sizeof(GasGridParamsHost) options:MTLResourceStorageModeShared];
    self.dbgCap = [self.device newBufferWithLength:sizeof(uint32_t) options:MTLResourceStorageModeShared];
    self.dbgHead = [self.device newBufferWithLength:sizeof(uint32_t) options:MTLResourceStorageModeShared];
    self.dbgWords = [self.device newBufferWithLength:sizeof(uint32_t) options:MTLResourceStorageModeShared];

    self.oscPhase = [self.device newBufferWithLength:(size_t)maxModes * sizeof(float) options:MTLResourceStorageModeShared];
    self.oscOmega = [self.device newBufferWithLength:(size_t)maxModes * sizeof(float) options:MTLResourceStorageModeShared];
    self.oscAmp = [self.device newBufferWithLength:(size_t)maxModes * sizeof(float) options:MTLResourceStorageModeShared];
    self.oscHeat = [self.device newBufferWithLength:(size_t)maxModes * sizeof(float) options:MTLResourceStorageModeShared];
    self.particlePos = [self.device newBufferWithLength:(size_t)maxModes * 3 * sizeof(float) options:MTLResourceStorageModeShared];
    self.particleVel = [self.device newBufferWithLength:(size_t)maxModes * 3 * sizeof(float) options:MTLResourceStorageModeShared];
    self.particleMass = [self.device newBufferWithLength:(size_t)maxModes * sizeof(float) options:MTLResourceStorageModeShared];
    self.particleEnergy = [self.device newBufferWithLength:(size_t)maxModes * sizeof(float) options:MTLResourceStorageModeShared];
    self.particlePosSorted = [self.device newBufferWithLength:(size_t)maxModes * 3 * sizeof(float) options:MTLResourceStorageModeShared];
    self.particleVelSorted = [self.device newBufferWithLength:(size_t)maxModes * 3 * sizeof(float) options:MTLResourceStorageModeShared];
    self.particleMassSorted = [self.device newBufferWithLength:(size_t)maxModes * sizeof(float) options:MTLResourceStorageModeShared];
    self.particleHeatSorted = [self.device newBufferWithLength:(size_t)maxModes * sizeof(float) options:MTLResourceStorageModeShared];
    self.particleEnergySorted = [self.device newBufferWithLength:(size_t)maxModes * sizeof(float) options:MTLResourceStorageModeShared];
    self.particleCellIdx = [self.device newBufferWithLength:(size_t)maxModes * sizeof(uint32_t) options:MTLResourceStorageModeShared];
    self.scatterCellCounts = [self.device newBufferWithLength:cellBytes options:MTLResourceStorageModeShared];
    self.scatterCellStarts = [self.device newBufferWithLength:cellBytes options:MTLResourceStorageModeShared];
    self.scatterCellOffsets = [self.device newBufferWithLength:cellBytes options:MTLResourceStorageModeShared];
    self.sortedOriginalIdx = [self.device newBufferWithLength:(size_t)maxModes * sizeof(uint32_t) options:MTLResourceStorageModeShared];
    self.rhoAtomic = [self.device newBufferWithLength:cellBytes options:MTLResourceStorageModeShared];
    self.momAtomic = [self.device newBufferWithLength:cellBytes * 3 options:MTLResourceStorageModeShared];
    self.eAtomic = [self.device newBufferWithLength:cellBytes options:MTLResourceStorageModeShared];
    self.gravityPotential = [self.device newBufferWithLength:cellBytes options:MTLResourceStorageModeShared];
    self.sortScatterParams = [self.device newBufferWithLength:sizeof(SortScatterParamsHost) options:MTLResourceStorageModeShared];
    self.picGatherParams = [self.device newBufferWithLength:sizeof(PicGatherParamsHost) options:MTLResourceStorageModeShared];
    self.particleExcitation = [self.device newBufferWithLength:(size_t)maxModes * sizeof(float) options:MTLResourceStorageModeShared];
    self.particleVelIn = [self.device newBufferWithLength:(size_t)maxModes * 3 * sizeof(float) options:MTLResourceStorageModeShared];
    self.particleHeatIn = [self.device newBufferWithLength:(size_t)maxModes * sizeof(float) options:MTLResourceStorageModeShared];
    self.hashCellCounts = [self.device newBufferWithLength:cellBytes options:MTLResourceStorageModeShared];
    self.hashCellStarts = [self.device newBufferWithLength:((size_t)self.numCells + 1u) * sizeof(uint32_t) options:MTLResourceStorageModeShared];
    self.hashCellOffsets = [self.device newBufferWithLength:cellBytes options:MTLResourceStorageModeShared];
    self.hashSortedIdx = [self.device newBufferWithLength:(size_t)maxModes * sizeof(uint32_t) options:MTLResourceStorageModeShared];
    self.hashParticleCellIdx = [self.device newBufferWithLength:(size_t)maxModes * sizeof(uint32_t) options:MTLResourceStorageModeShared];
    self.hashNumCellsBuf = [self.device newBufferWithLength:sizeof(uint32_t) options:MTLResourceStorageModeShared];
    self.hashNumParticlesBuf = [self.device newBufferWithLength:sizeof(uint32_t) options:MTLResourceStorageModeShared];
    *(uint32_t *)self.hashNumCellsBuf.contents = self.numCells;
    self.spatialHashParams = [self.device newBufferWithLength:sizeof(SpatialHashParamsHost) options:MTLResourceStorageModeShared];
    self.spatialCollisionParams = [self.device newBufferWithLength:sizeof(SpatialCollisionParamsHost) options:MTLResourceStorageModeShared];
    self.particleInteractionParams = [self.device newBufferWithLength:sizeof(ParticleInteractionParamsHost) options:MTLResourceStorageModeShared];
    self.psiReField = [self.device newBufferWithLength:cellBytes options:MTLResourceStorageModeShared];
    self.psiImField = [self.device newBufferWithLength:cellBytes options:MTLResourceStorageModeShared];
    self.psiReAtomic = [self.device newBufferWithLength:cellBytes options:MTLResourceStorageModeShared];
    self.psiImAtomic = [self.device newBufferWithLength:cellBytes options:MTLResourceStorageModeShared];
    self.modeProjectParams = [self.device newBufferWithLength:sizeof(ModeProjectParamsHost) options:MTLResourceStorageModeShared];
    self.pilotWaveParams = [self.device newBufferWithLength:sizeof(PilotWaveParamsHost) options:MTLResourceStorageModeShared];
    self.particleGenParams = [self.device newBufferWithLength:sizeof(ParticleGenParamsHost) options:MTLResourceStorageModeShared];
    self.particleRandomVals = [self.device newBufferWithLength:(size_t)maxModes * 4 * sizeof(float) options:MTLResourceStorageModeShared];
    self.reduceGroupStats = [self.device newBufferWithLength:(size_t)maxModes * 4 * sizeof(float) options:MTLResourceStorageModeShared];
    self.reduceStatsOut = [self.device newBufferWithLength:4 * sizeof(float) options:MTLResourceStorageModeShared];
    self.hashBlockSums = [self.device newBufferWithLength:(size_t)maxModes * sizeof(uint32_t) options:MTLResourceStorageModeShared];
    self.gravityReady = NO;
    self.modeReal = [self.device newBufferWithLength:(size_t)maxModes * sizeof(float) options:MTLResourceStorageModeShared];
    self.modeImag = [self.device newBufferWithLength:(size_t)maxModes * sizeof(float) options:MTLResourceStorageModeShared];
    self.modeOmega = [self.device newBufferWithLength:(size_t)maxModes * sizeof(float) options:MTLResourceStorageModeShared];
    self.modeGate = [self.device newBufferWithLength:(size_t)maxModes * sizeof(float) options:MTLResourceStorageModeShared];
    self.modeAnchorIdx = [self.device newBufferWithLength:(size_t)maxModes * 8 * sizeof(uint32_t) options:MTLResourceStorageModeShared];
    self.modeAnchorWeight = [self.device newBufferWithLength:(size_t)maxModes * 8 * sizeof(float) options:MTLResourceStorageModeShared];
    self.modeAnchorPos = [self newGPUBufferWithLength:(size_t)maxModes * kModeAnchors * 3 * sizeof(float)];
    self.oscCouplingPrep = [self newGPUBufferWithLength:(size_t)maxModes * 4 * sizeof(float)];
    self.accums = [self.device newBufferWithLength:(size_t)maxModes * sizeof(CarrierAccumHost) options:MTLResourceStorageModeShared];
    self.numCarriers = [self.device newBufferWithLength:sizeof(uint32_t) options:MTLResourceStorageModeShared];
    self.omegaMinKey = [self.device newBufferWithLength:sizeof(uint32_t) options:MTLResourceStorageModeShared];
    self.omegaMaxKey = [self.device newBufferWithLength:sizeof(uint32_t) options:MTLResourceStorageModeShared];
    self.binCounts = [self.device newBufferWithLength:(size_t)maxModes * sizeof(uint32_t) options:MTLResourceStorageModeShared];
    self.binStarts = [self.device newBufferWithLength:((size_t)maxModes + 1) * sizeof(uint32_t) options:MTLResourceStorageModeShared];
    self.binOffsets = [self.device newBufferWithLength:(size_t)maxModes * sizeof(uint32_t) options:MTLResourceStorageModeShared];
    self.carrierBinnedIdx = [self.device newBufferWithLength:(size_t)maxModes * sizeof(uint32_t) options:MTLResourceStorageModeShared];
    self.binParams = [self.device newBufferWithLength:sizeof(BinParamsHost) options:MTLResourceStorageModeShared];
    self.numBinsBuf = [self.device newBufferWithLength:sizeof(uint32_t) options:MTLResourceStorageModeShared];
    self.gateWidthMaxBuf = [self.device newBufferWithLength:sizeof(float) options:MTLResourceStorageModeShared];
    *(float *)self.gateWidthMaxBuf.contents = self.config.gate_width_max;
    self.scanBlockSums = [self.device newBufferWithLength:(size_t)maxModes * sizeof(uint32_t) options:MTLResourceStorageModeShared];
    self.scanBlockPrefix = [self.device newBufferWithLength:(size_t)maxModes * sizeof(uint32_t) options:MTLResourceStorageModeShared];
    self.scanBlockScratch = [self.device newBufferWithLength:(size_t)maxModes * sizeof(uint32_t) options:MTLResourceStorageModeShared];
    self.coherenceParams = [self.device newBufferWithLength:sizeof(CoherenceParamsHost) options:MTLResourceStorageModeShared];
    self.gpeParams = [self.device newBufferWithLength:sizeof(GPEParamsHost) options:MTLResourceStorageModeShared];

    [self resetDepositsInternal];
    *(uint32_t *)self.dbgCap.contents = 0;
    *(uint32_t *)self.dbgHead.contents = 0;

    return YES;
}

- (void)resetDepositsInternal {
    [self runClearField:self.momRho count:(self.numCells * 4)];
    [self runClearField:self.eInt count:self.numCells];
}

- (BOOL)depositCell:(uint32_t)cellX cellY:(uint32_t)cellY cellZ:(uint32_t)cellZ
                rho:(float)rho momX:(float)momX momY:(float)momY momZ:(float)momZ eInt:(float)eInt error:(NSString **)error {
    if (cellX >= self.config.grid_x || cellY >= self.config.grid_y || cellZ >= self.config.grid_z) {
        if (error != nil) {
            *error = @"deposit cell out of bounds";
        }

        return NO;
    }

    uint32_t index = manifold_cell_index(cellX, cellY, cellZ, self.config.grid_x, self.config.grid_y, self.config.grid_z);
    float *momRhoData = (float *)self.momRho.contents;
    float *eData = (float *)self.eInt.contents;

    uint32_t base = index * 4;
    momRhoData[base + 0] += momX;
    momRhoData[base + 1] += momY;
    momRhoData[base + 2] += momZ;
    momRhoData[base + 3] += rho;
    eData[index] += eInt;

    return YES;
}

- (BOOL)setOscillators:(const ManifoldOscillator *)oscillators count:(uint32_t)count error:(NSString **)error {
    if (oscillators == NULL || count == 0) {
        if (error != nil) {
            *error = @"oscillator list is empty";
        }

        return NO;
    }

    if (count > self.config.max_carriers) {
        if (error != nil) {
            *error = @"oscillator count exceeds max_carriers";
        }

        return NO;
    }

    self.numOsc = count;
    *(uint32_t *)self.numCarriers.contents = count;

    float *phaseData = (float *)self.oscPhase.contents;
    float *omegaData = (float *)self.oscOmega.contents;
    float *ampData = (float *)self.oscAmp.contents;
    float *heatData = (float *)self.oscHeat.contents;
    float *posData = (float *)self.particlePos.contents;
    float *modeRealData = (float *)self.modeReal.contents;
    float *modeImagData = (float *)self.modeImag.contents;
    float *modeOmegaData = (float *)self.modeOmega.contents;
    float *modeGateData = (float *)self.modeGate.contents;
    uint32_t *anchorIdx = (uint32_t *)self.modeAnchorIdx.contents;
    float *anchorWeight = (float *)self.modeAnchorWeight.contents;

    float *excitationData = (float *)self.particleExcitation.contents;

    BOOL needsGeneratedPositions = YES;

    for (uint32_t index = 0; index < count; index++) {
        if (oscillators[index].pos_x != 0.0f || oscillators[index].pos_y != 0.0f || oscillators[index].pos_z != 0.0f) {
            needsGeneratedPositions = NO;
            break;
        }
    }

    for (uint32_t index = 0; index < count; index++) {
        const ManifoldOscillator *oscillator = &oscillators[index];
        phaseData[index] = oscillator->phase;
        omegaData[index] = oscillator->omega;
        ampData[index] = oscillator->amplitude;
        heatData[index] = oscillator->heat;
        excitationData[index] = oscillator->omega;

        if (!needsGeneratedPositions) {
            posData[index * 3 + 0] = oscillator->pos_x;
            posData[index * 3 + 1] = oscillator->pos_y;
            posData[index * 3 + 2] = oscillator->pos_z;
        }
    }

    if (needsGeneratedPositions) {
        [self configureParticleGenParams];
        [self seedRandomValuesFromOscillators:oscillators count:count];
        [self dispatchGridKernel:self.generateParticlePositions
                         buffers:@[self.particlePos, self.particleRandomVals, self.particleGenParams]
                     threadCount:count];
    }

    [self runInitializeParticleProperties:oscillators count:count];

    float *velData = (float *)self.particleVel.contents;

    for (uint32_t index = 0; index < count; index++) {
        const ManifoldOscillator *oscillator = &oscillators[index];

        if (oscillator->vel_x == 0.0f && oscillator->vel_y == 0.0f && oscillator->vel_z == 0.0f) {
            continue;
        }

        velData[index * 3 + 0] = oscillator->vel_x;
        velData[index * 3 + 1] = oscillator->vel_y;
        velData[index * 3 + 2] = oscillator->vel_z;
    }

    for (uint32_t index = 0; index < count; index++) {
        const ManifoldOscillator *oscillator = &oscillators[index];
        modeRealData[index] = cosf(oscillator->phase) * oscillator->amplitude;
        modeImagData[index] = sinf(oscillator->phase) * oscillator->amplitude;
        modeOmegaData[index] = oscillator->omega;
        modeGateData[index] = fmaxf(self.config.gate_width_min, self.config.gate_width_max * 0.5f);
        anchorIdx[index * 8 + 0] = index;
        anchorWeight[index * 8 + 0] = 1.0f;

        for (uint32_t slot = 1; slot < 8; slot++) {
            anchorIdx[index * 8 + slot] = 0xFFFFFFFFu;
            anchorWeight[index * 8 + slot] = 0.0f;
        }
    }

    return YES;
}

- (void)configureGasParams {
    GasGridParamsHost *params = (GasGridParamsHost *)self.gasParams.contents;
    float dx = self.config.domain_x / (float)self.config.grid_x;

    params->num_cells = self.numCells;
    params->grid_x = self.config.grid_x;
    params->grid_y = self.config.grid_y;
    params->grid_z = self.config.grid_z;
    params->dx = dx;
    params->inv_dx = 1.0f / dx;
    params->inv_dx2 = params->inv_dx * params->inv_dx;
    params->dt = self.config.dt;
    params->gamma = self.config.gamma;
    params->c_v = self.config.c_v;
    float envelopeRho = self.config.gas_envelope_rho_min;

    if (!(envelopeRho > 0.0f)) {
        float carrierCount = (float)self.config.max_carriers;

        if (carrierCount < 1.0f) {
            carrierCount = 1.0f;
        }

        envelopeRho = self.config.rho_min / carrierCount;
    }

    float gasPMin = self.config.gas_p_min;

    if (!(gasPMin > 0.0f)) {
        float cellVolume = (self.config.domain_x / (float)self.config.grid_x) *
            (self.config.domain_y / (float)self.config.grid_y) *
            (self.config.domain_z / (float)self.config.grid_z);
        gasPMin = (self.config.gamma - 1.0f) * envelopeRho * cellVolume;
    }

    params->rho_min = envelopeRho;
    params->p_min = gasPMin;
    params->mu = 0.0f;
    params->k_thermal = self.config.k_thermal;
}

- (BOOL)runGasStep:(NSString **)error {
    (void)error;
    [self configureGasParams];
    *(uint32_t *)self.dbgCap.contents = 0;

    [self dispatchGasBrickKernel:self.gasComputePrimitives
                         buffers:@[
                             self.momRho, self.eInt,
                             self.gasPrim, self.gasParams
                         ]];

    [self dispatchGasBrickKernel:self.gasStage1
                         buffers:@[
                             self.momRho, self.eInt,
                             self.gasPrim,
                             self.momRhoStage, self.eStage,
                             self.gasParams, self.dbgHead, self.dbgWords, self.dbgCap
                         ]];

    [self dispatchGasBrickKernel:self.gasComputePrimitives
                         buffers:@[
                             self.momRhoStage, self.eStage,
                             self.gasPrim, self.gasParams
                         ]];

    [self dispatchGasBrickKernel:self.gasStage2
                         buffers:@[
                             self.momRho, self.eInt,
                             self.momRhoStage, self.eStage,
                             self.gasPrim,
                             self.momRho, self.eInt,
                             self.gasParams, self.dbgHead, self.dbgWords, self.dbgCap
                         ]];

    return YES;
}

- (BOOL)computeReading:(ManifoldReading *)reading error:(NSString **)error {
    (void)error;
    float *momRhoData = (float *)self.momRho.contents;
    float *eData = (float *)self.eInt.contents;
    float *modeRealData = (float *)self.modeReal.contents;
    float *modeImagData = (float *)self.modeImag.contents;
    uint32_t gx = self.config.grid_x;
    uint32_t gy = self.config.grid_y;
    uint32_t gz = self.config.grid_z;
    uint32_t cx = gx / 2;
    uint32_t cy = gy / 2;
    uint32_t cz = gz / 2;
    float dx = self.config.domain_x / (float)gx;
    uint32_t xm = (cx == 0) ? (gx - 1) : (cx - 1);
    uint32_t xp = (cx + 1 == gx) ? 0 : (cx + 1);
    uint32_t ym = (cy == 0) ? (gy - 1) : (cy - 1);
    uint32_t yp = (cy + 1 == gy) ? 0 : (cy + 1);
    uint32_t zm = (cz == 0) ? (gz - 1) : (cz - 1);
    uint32_t zp = (cz + 1 == gz) ? 0 : (cz + 1);

    float dpx = (manifold_pressure_at(eData, self.config.gamma, xp, cy, cz, gx, gy, gz) -
                 manifold_pressure_at(eData, self.config.gamma, xm, cy, cz, gx, gy, gz)) / (2.0f * dx);
    float dpy = (manifold_pressure_at(eData, self.config.gamma, cx, yp, cz, gx, gy, gz) -
                 manifold_pressure_at(eData, self.config.gamma, cx, ym, cz, gx, gy, gz)) / (2.0f * dx);
    float dpz = (manifold_pressure_at(eData, self.config.gamma, cx, cy, zp, gx, gy, gz) -
                 manifold_pressure_at(eData, self.config.gamma, cx, cy, zm, gx, gy, gz)) / (2.0f * dx);

    float ux_xm, uy_xm, uz_xm, ux_xp, uy_xp, uz_xp;
    float ux_ym, uy_ym, uz_ym, ux_yp, uy_yp, uz_yp;
    float ux_zm, uy_zm, uz_zm, ux_zp, uy_zp, uz_zp;
    manifold_velocity_at(momRhoData, xm, cy, cz, gx, gy, gz, &ux_xm, &uy_xm, &uz_xm);
    manifold_velocity_at(momRhoData, xp, cy, cz, gx, gy, gz, &ux_xp, &uy_xp, &uz_xp);
    manifold_velocity_at(momRhoData, cx, ym, cz, gx, gy, gz, &ux_ym, &uy_ym, &uz_ym);
    manifold_velocity_at(momRhoData, cx, yp, cz, gx, gy, gz, &ux_yp, &uy_yp, &uz_yp);
    manifold_velocity_at(momRhoData, cx, cy, zm, gx, gy, gz, &ux_zm, &uy_zm, &uz_zm);
    manifold_velocity_at(momRhoData, cx, cy, zp, gx, gy, gz, &ux_zp, &uy_zp, &uz_zp);

    float divergence = (ux_xp - ux_xm + uy_yp - uy_ym + uz_zp - uz_zm) / (2.0f * dx);
    float coherenceMag2 = 0.0f;
    float guidanceSpeed = 0.0f;

    for (uint32_t index = 0; index < self.numOsc; index++) {
        float re = modeRealData[index];
        float im = modeImagData[index];
        coherenceMag2 += re * re + im * im;
        guidanceSpeed += fabsf(re * im);
    }

    if (self.numOsc > 0) {
        coherenceMag2 /= (float)self.numOsc;
        guidanceSpeed /= (float)self.numOsc;
    }

    reading->pressure_grad_x = dpx;
    reading->pressure_grad_y = dpy;
    reading->pressure_grad_z = dpz;
    reading->pressure_grad_norm = sqrtf(dpx * dpx + dpy * dpy + dpz * dpz);
    reading->divergence = divergence;
    reading->coherence_mag2 = coherenceMag2;
    reading->guidance_speed = guidanceSpeed * self.config.hbar_eff;
    reading->viscosity_proxy = (fabsf(divergence) > 1e-8f) ? (1.0f / fabsf(divergence)) : 0.0f;

    return YES;
}

- (BOOL)computeProjectionReading:(ManifoldReading *)reading error:(NSString **)error {
    if (reading == NULL) {
        if (error != NULL) {
            *error = @"projection reading is required";
        }

        return NO;
    }

    uint32_t gx = self.config.grid_x;
    uint32_t gz = self.config.grid_z;

    if (gx < 3 || gz < 3) {
        if (error != NULL) {
            *error = @"projection reading requires at least a 3x3 rho lattice";
        }

        return NO;
    }

    float spacing = self.config.domain_x / (float)gx;

    if (spacing <= 0.0f) {
        if (error != NULL) {
            *error = @"projection reading requires positive grid spacing";
        }

        return NO;
    }

    uint32_t expected = gx * gz;
    float *projection = (float *)calloc(expected, sizeof(float));

    if (projection == NULL) {
        if (error != NULL) {
            *error = @"projection reading buffer allocation failed";
        }

        return NO;
    }

    if (![self readRhoMaxProjection:projection length:expected error:error]) {
        free(projection);

        return NO;
    }

    float gradSumSq = 0.0f;
    float curvatureSum = 0.0f;
    uint32_t sampleCount = 0;

    for (uint32_t zIndex = 1; zIndex + 1 < gz; zIndex++) {
        for (uint32_t xIndex = 1; xIndex + 1 < gx; xIndex++) {
            uint32_t centerIndex = xIndex + zIndex * gx;
            float center = projection[centerIndex];
            float dRhoDx = (projection[(xIndex + 1) + zIndex * gx] -
                            projection[(xIndex - 1) + zIndex * gx]) /
                           (2.0f * spacing);
            float dRhoDz = (projection[xIndex + (zIndex + 1) * gx] -
                            projection[xIndex + (zIndex - 1) * gx]) /
                           (2.0f * spacing);

            gradSumSq += dRhoDx * dRhoDx + dRhoDz * dRhoDz;

            float laplacian = (projection[(xIndex + 1) + zIndex * gx] +
                                 projection[(xIndex - 1) + zIndex * gx] +
                                 projection[xIndex + (zIndex + 1) * gx] +
                                 projection[xIndex + (zIndex - 1) * gx] -
                                 4.0f * center) /
                                (spacing * spacing);

            curvatureSum += fabsf(laplacian);
            sampleCount++;
        }
    }

    free(projection);

    if (sampleCount == 0) {
        if (error != NULL) {
            *error = @"projection reading has no interior samples";
        }

        return NO;
    }

    float gradNorm = sqrtf(gradSumSq / (float)sampleCount);
    float curvature = curvatureSum / (float)sampleCount;

    reading->pressure_grad_x = 0.0f;
    reading->pressure_grad_y = 0.0f;
    reading->pressure_grad_z = 0.0f;
    reading->pressure_grad_norm = gradNorm;
    reading->divergence = curvature;
    reading->coherence_mag2 = 0.0f;
    reading->guidance_speed = 0.0f;
    reading->viscosity_proxy = (curvature > 0.0f) ? (1.0f / curvature) : 0.0f;

    return YES;
}

- (BOOL)readOscillators:(ManifoldOscillator *)out count:(uint32_t)count error:(NSString **)error {
    if (out == NULL) {
        if (error != NULL) {
            *error = @"oscillator read buffer is required";
        }

        return NO;
    }

    if (count < self.numOsc) {
        if (error != NULL) {
            *error = @"oscillator read buffer is too small";
        }

        return NO;
    }

    float *phaseData = (float *)self.oscPhase.contents;
    float *omegaData = (float *)self.oscOmega.contents;
    float *ampData = (float *)self.oscAmp.contents;
    float *heatData = (float *)self.oscHeat.contents;
    float *posData = (float *)self.particlePos.contents;
    float *velData = (float *)self.particleVel.contents;

    for (uint32_t index = 0; index < self.numOsc; index++) {
        out[index].phase = phaseData[index];
        out[index].omega = omegaData[index];
        out[index].amplitude = ampData[index];
        out[index].heat = heatData[index];
        out[index].pos_x = posData[index * 3 + 0];
        out[index].pos_y = posData[index * 3 + 1];
        out[index].pos_z = posData[index * 3 + 2];
        out[index].vel_x = velData[index * 3 + 0];
        out[index].vel_y = velData[index * 3 + 1];
        out[index].vel_z = velData[index * 3 + 2];
    }

    return YES;
}

- (BOOL)readRhoMaxProjection:(float *)out length:(uint32_t)length error:(NSString **)error {
    if (out == NULL) {
        if (error != NULL) {
            *error = @"rho projection buffer is required";
        }

        return NO;
    }

    float *momRhoData = (float *)self.momRho.contents;
    uint32_t gx = self.config.grid_x;
    uint32_t gy = self.config.grid_y;
    uint32_t gz = self.config.grid_z;
    uint32_t expected = gx * gz;

    if (length < expected) {
        if (error != NULL) {
            *error = @"rho projection buffer is too small";
        }

        return NO;
    }

    for (uint32_t zIndex = 0; zIndex < gz; zIndex++) {
        for (uint32_t xIndex = 0; xIndex < gx; xIndex++) {
            float peak = 0.0f;

            for (uint32_t yIndex = 0; yIndex < gy; yIndex++) {
                uint32_t index = manifold_cell_index(xIndex, yIndex, zIndex, gx, gy, gz);
                float value = momRhoData[index * 4 + 3];

                if (value > peak) {
                    peak = value;
                }
            }

            out[xIndex + zIndex * gx] = peak;
        }
    }

    return YES;
}

- (BOOL)step:(ManifoldReading *)reading error:(NSString **)error {
    [self beginStepDispatches];
    if (![self runPicScatter:error]) {
        [self endStepDispatches];
        return NO;
    }
    [self endStepDispatches];

    [self beginStepDispatches];
    if (![self runGravityPoisson:error]) {
        [self endStepDispatches];
        return NO;
    }
    [self endStepDispatches];

    [self beginStepDispatches];
    if (![self runGasStep:error]) {
        [self endStepDispatches];
        return NO;
    }
    [self endStepDispatches];

    [self beginStepDispatches];
    if (![self runParticleCollisions:error]) {
        [self endStepDispatches];
        return NO;
    }
    [self endStepDispatches];

    [self beginStepDispatches];
    if (![self runPicGather:error]) {
        [self endStepDispatches];
        return NO;
    }
    [self endStepDispatches];

    [self beginStepDispatches];
    if (![self runCoherenceStep:error]) {
        [self endStepDispatches];
        return NO;
    }
    [self endStepDispatches];

    [self beginStepDispatches];
    if (![self runProjectModesToSpatialPsi:error]) {
        [self endStepDispatches];
        return NO;
    }
    [self endStepDispatches];

    [self beginStepDispatches];
    if (![self runPilotWaveGather:error]) {
        [self endStepDispatches];
        return NO;
    }
    [self endStepDispatches];

    ManifoldReading modeReading;

    if (![self computeReading:&modeReading error:error]) {
        return NO;
    }

    ManifoldReading bulkReading;

    if (![self computeProjectionReading:&bulkReading error:error]) {
        return NO;
    }

    reading->pressure_grad_x = bulkReading.pressure_grad_x;
    reading->pressure_grad_y = bulkReading.pressure_grad_y;
    reading->pressure_grad_z = bulkReading.pressure_grad_z;
    reading->pressure_grad_norm = bulkReading.pressure_grad_norm;
    reading->divergence = bulkReading.divergence;
    reading->viscosity_proxy = bulkReading.viscosity_proxy;
    reading->coherence_mag2 = modeReading.coherence_mag2;
    reading->guidance_speed = modeReading.guidance_speed;

    return YES;
}

@end

void *manifold_solver_create(
    const ManifoldConfig *config,
    const void *metallib_bytes,
    size_t metallib_length,
    char *err_out,
    int err_cap
) {
    if (config == NULL || metallib_bytes == NULL || metallib_length == 0) {
        manifold_write_error(err_out, err_cap, @"config and metallib payload are required");
        return NULL;
    }

    NSString *error = nil;
    ManifoldSolver *solver = [[ManifoldSolver alloc] initWithConfig:config
                                                      metallibBytes:metallib_bytes
                                                     metallibLength:metallib_length
                                                              error:&error];

    if (solver == nil) {
        manifold_write_error(err_out, err_cap, error ?: @"failed to create manifold solver");
        return NULL;
    }

    return (__bridge_retained void *)solver;
}

void manifold_solver_destroy(void *handle) {
    if (handle == NULL) {
        return;
    }

    ManifoldSolver *solver = (__bridge_transfer ManifoldSolver *)handle;
    (void)solver;
}

int manifold_solver_reset_deposits(void *handle, char *err_out, int err_cap) {
    if (handle == NULL) {
        manifold_write_error(err_out, err_cap, @"solver handle is nil");
        return 1;
    }

    ManifoldSolver *solver = (__bridge ManifoldSolver *)handle;
    [solver resetDepositsInternal];
    return 0;
}

int manifold_solver_deposit_cell(
    void *handle,
    uint32_t cell_x,
    uint32_t cell_y,
    uint32_t cell_z,
    float rho,
    float mom_x,
    float mom_y,
    float mom_z,
    float e_int,
    char *err_out,
    int err_cap
) {
    if (handle == NULL) {
        manifold_write_error(err_out, err_cap, @"solver handle is nil");
        return 1;
    }

    ManifoldSolver *solver = (__bridge ManifoldSolver *)handle;
    NSString *error = nil;

    if (![solver depositCell:cell_x cellY:cell_y cellZ:cell_z rho:rho momX:mom_x momY:mom_y momZ:mom_z eInt:e_int error:&error]) {
        manifold_write_error(err_out, err_cap, error ?: @"deposit failed");
        return 1;
    }

    return 0;
}

int manifold_solver_set_oscillators(
    void *handle,
    const ManifoldOscillator *oscillators,
    uint32_t count,
    char *err_out,
    int err_cap
) {
    if (handle == NULL) {
        manifold_write_error(err_out, err_cap, @"solver handle is nil");
        return 1;
    }

    ManifoldSolver *solver = (__bridge ManifoldSolver *)handle;
    NSString *error = nil;

    if (![solver setOscillators:oscillators count:count error:&error]) {
        manifold_write_error(err_out, err_cap, error ?: @"set oscillators failed");
        return 1;
    }

    return 0;
}

int manifold_solver_step(void *handle, ManifoldReading *reading, char *err_out, int err_cap) {
    if (handle == NULL || reading == NULL) {
        manifold_write_error(err_out, err_cap, @"solver handle and reading are required");
        return 1;
    }

    ManifoldSolver *solver = (__bridge ManifoldSolver *)handle;
    NSString *error = nil;

    if (![solver step:reading error:&error]) {
        manifold_write_error(err_out, err_cap, error ?: @"step failed");
        return 1;
    }

    return 0;
}

int manifold_solver_read_rho_projection(
    void *handle,
    float *out,
    uint32_t out_length,
    uint32_t *grid_x,
    uint32_t *grid_z,
    char *err_out,
    int err_cap
) {
    if (handle == NULL || out == NULL || grid_x == NULL || grid_z == NULL) {
        manifold_write_error(err_out, err_cap, @"solver handle and rho projection buffers are required");
        return 1;
    }

    ManifoldSolver *solver = (__bridge ManifoldSolver *)handle;
    NSString *error = nil;

    if (![solver readRhoMaxProjection:out length:out_length error:&error]) {
        manifold_write_error(err_out, err_cap, error ?: @"rho projection read failed");
        return 1;
    }

    *grid_x = solver.config.grid_x;
    *grid_z = solver.config.grid_z;

    return 0;
}

int manifold_solver_read_projection_reading(
    void *handle,
    ManifoldReading *reading,
    char *err_out,
    int err_cap
) {
    if (handle == NULL || reading == NULL) {
        manifold_write_error(err_out, err_cap, @"solver handle and projection reading are required");
        return 1;
    }

    ManifoldSolver *solver = (__bridge ManifoldSolver *)handle;
    NSString *error = nil;

    if (![solver computeProjectionReading:reading error:&error]) {
        manifold_write_error(err_out, err_cap, error ?: @"projection reading failed");
        return 1;
    }

    return 0;
}

int manifold_solver_read_oscillators(
    void *handle,
    ManifoldOscillator *out,
    uint32_t count,
    char *err_out,
    int err_cap
) {
    if (handle == NULL || out == NULL) {
        manifold_write_error(err_out, err_cap, @"solver handle and oscillator buffer are required");
        return 1;
    }

    ManifoldSolver *solver = (__bridge ManifoldSolver *)handle;
    NSString *error = nil;

    if (![solver readOscillators:out count:count error:&error]) {
        manifold_write_error(err_out, err_cap, error ?: @"oscillator read failed");
        return 1;
    }

    return 0;
}
