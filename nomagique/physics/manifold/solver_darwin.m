#import "solver_private.h"

static float manifold_debug_float(uint32_t bits) {
    float value;
    memcpy(&value, &bits, sizeof(value));

    return value;
}

static float manifold_wrap_coordinate(float value, float extent) {
    return value - floorf(value / extent) * extent;
}

/*
Process-lifetime Metal host. Never a field solver: field Close must not tear
down device/library/pipelines, or the next Field create walks freed objects.
*/
static void *gManifoldMetalHostRaw = NULL;
static NSData *gManifoldMetallib = nil;
static NSLock *gManifoldMetalLock = nil;

static ManifoldSolver *manifold_metal_host_load(void) {
    if (gManifoldMetalHostRaw == NULL) {
        return nil;
    }

    return (__bridge ManifoldSolver *)gManifoldMetalHostRaw;
}

@interface ManifoldSolver (MetalHostPrivate)
- (BOOL)installSharedMetalWithBytes:(const void *)metallibBytes
                             length:(size_t)metallibLength
                              config:(const ManifoldConfig *)config
                              error:(NSString **)error;
- (void)adoptMetalFrom:(ManifoldSolver *)host;
- (id<MTLBuffer>)trackResident:(id<MTLBuffer>)buffer;
@end

static void manifold_metal_lock_init(void) {
    static dispatch_once_t onceToken;
    dispatch_once(&onceToken, ^{
        gManifoldMetalLock = [[NSLock alloc] init];
    });
}

static ManifoldSolver *manifold_shared_metal_host(
    const void *metallibBytes,
    size_t metallibLength,
    const ManifoldConfig *config,
    NSString **error
) {
    manifold_metal_lock_init();
    [gManifoldMetalLock lock];

    ManifoldSolver *existing = manifold_metal_host_load();

    if (existing != nil) {
        [gManifoldMetalLock unlock];
        return existing;
    }

    if (metallibBytes == NULL || metallibLength == 0) {
        if (error != nil) {
            *error = @"metallib payload is empty";
        }

        [gManifoldMetalLock unlock];
        return nil;
    }

    ManifoldSolver *host = [[ManifoldSolver alloc] init];

    if (![host installSharedMetalWithBytes:metallibBytes
                                   length:metallibLength
                                   config:config
                                    error:error]) {
        [gManifoldMetalLock unlock];
        return nil;
    }

    // Process-lifetime ownership via CF only — never an ARC static.
    gManifoldMetalHostRaw = (void *)CFBridgingRetain(host);
    [gManifoldMetalLock unlock];
    return host;
}

@implementation ManifoldSolver

- (BOOL)installSharedMetalWithBytes:(const void *)metallibBytes
                             length:(size_t)metallibLength
                              config:(const ManifoldConfig *)config
                              error:(NSString **)error {
    if (config != NULL) {
        self.config = *config;
    }

    self.device = MTLCreateSystemDefaultDevice();

    if (self.device == nil) {
        if (error != nil) {
            *error = @"Metal device unavailable";
        }

        return NO;
    }

    self.queue = [self.device newCommandQueue];

    if (self.queue == nil) {
        if (error != nil) {
            *error = @"Metal command queue unavailable";
        }

        return NO;
    }

    gManifoldMetallib = [NSData dataWithBytes:metallibBytes length:metallibLength];
    CFBridgingRetain(gManifoldMetallib);
    NSError *metalError = nil;
    dispatch_data_t metallibData = dispatch_data_create(
        gManifoldMetallib.bytes,
        gManifoldMetallib.length,
        dispatch_get_global_queue(QOS_CLASS_DEFAULT, 0),
        DISPATCH_DATA_DESTRUCTOR_DEFAULT
    );
    self.library = [self.device newLibraryWithData:metallibData error:&metalError];

    if (self.library == nil) {
        if (error != nil) {
            *error = metalError.localizedDescription ?: @"failed to load kernels.metallib";
        }

        return NO;
    }

    if (![self buildPipelines:error]) {
        return NO;
    }

    self.simdWidth = (uint32_t)self.accumulateForces.threadExecutionWidth;

    if (self.simdWidth == 0) {
        self.simdWidth = 32;
    }

    self.maxThreadsPerThreadgroup = manifold_pipeline_max_threads(self.accumulateForces);
    self.maxCarriersForTG = manifold_max_carriers_for_pipeline(self.device, self.accumulateForces);
    uint32_t gpeThreadLimit = (uint32_t)manifold_pipeline_max_threads(self.gpeStep);

    if (self.maxCarriersForTG > gpeThreadLimit) {
        self.maxCarriersForTG = gpeThreadLimit;
    }

    return YES;
}

- (void)adoptMetalFrom:(ManifoldSolver *)host {
    self.metalOwner = host;
    self.device = host.device;
    self.library = host.library;
}

- (id<MTLBuffer>)trackResident:(id<MTLBuffer>)buffer {
    if (buffer != nil && buffer.heap == nil) {
        self.residentBytes += (uint64_t)buffer.length;
    }

    return buffer;
}

- (instancetype)initWithConfig:(const ManifoldConfig *)config
                 metallibBytes:(const void *)metallibBytes
                metallibLength:(size_t)metallibLength
                         error:(NSString **)error {
    self = [super init];

    if (self == nil) {
        return nil;
    }

    ManifoldSolver *host = manifold_shared_metal_host(metallibBytes, metallibLength, config, error);

    if (host == nil) {
        return nil;
    }

    [self adoptMetalFrom:host];
    self.queue = [self.device newCommandQueue];

    if (self.queue == nil) {
        if (error != nil) {
            *error = @"Metal command queue unavailable";
        }

        return nil;
    }

    self.config = *config;
    self.controls = (ManifoldControls){
        .dt = config->dt,
        .metabolic_rate = config->metabolic_rate,
        .topdown_phase_scale = 0.0f,
        .topdown_energy_scale = 0.0f,
    };
    self.numCells = config->grid_x * config->grid_y * config->grid_z;
    self.residentBytes = 0;

    // Pipelines are per-field: function constants bind grid size, and AGX does
    // not keep shared pipeline states alive across Field teardown.
    if (![self buildPipelines:error]) {
        return nil;
    }

    self.simdWidth = (uint32_t)self.accumulateForces.threadExecutionWidth;

    if (self.simdWidth == 0) {
        self.simdWidth = 32;
    }

    self.maxThreadsPerThreadgroup = manifold_pipeline_max_threads(self.accumulateForces);
    self.maxCarriersForTG = manifold_max_carriers_for_pipeline(self.device, self.accumulateForces);
    uint32_t gpeThreadLimit = (uint32_t)manifold_pipeline_max_threads(self.gpeStep);

    if (self.maxCarriersForTG > gpeThreadLimit) {
        self.maxCarriersForTG = gpeThreadLimit;
    }

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

- (BOOL)setControls:(const ManifoldControls *)controls error:(NSString **)error {
    if (controls == NULL) {
        if (error != nil) {
            *error = @"runtime controls are required";
        }

        return NO;
    }

    if (!isfinite(controls->dt) || controls->dt <= 0.0f) {
        if (error != nil) {
            *error = @"runtime control dt must be positive and finite";
        }

        return NO;
    }

    if (!isfinite(controls->metabolic_rate) || controls->metabolic_rate < 0.0f) {
        if (error != nil) {
            *error = @"runtime control metabolic_rate must be non-negative and finite";
        }

        return NO;
    }

    if (!isfinite(controls->topdown_phase_scale) || controls->topdown_phase_scale < 0.0f) {
        if (error != nil) {
            *error = @"runtime control topdown_phase_scale must be non-negative and finite";
        }

        return NO;
    }

    if (!isfinite(controls->topdown_energy_scale) || controls->topdown_energy_scale < 0.0f) {
        if (error != nil) {
            *error = @"runtime control topdown_energy_scale must be non-negative and finite";
        }

        return NO;
    }

    if (!isfinite(controls->g_interaction) || controls->g_interaction <= 0.0f) {
        if (error != nil) {
            *error = @"runtime control g_interaction must be positive and finite";
        }

        return NO;
    }

    if (!isfinite(controls->energy_decay) || controls->energy_decay < 0.0f) {
        if (error != nil) {
            *error = @"runtime control energy_decay must be non-negative and finite";
        }

        return NO;
    }

    self.controls = *controls;

    ManifoldConfig config = self.config;
    config.g_interaction = controls->g_interaction;
    config.energy_decay = controls->energy_decay;
    self.config = config;

    return YES;
}

- (void)drainGPUQueue {
    if (self.queue == nil) {
        return;
    }

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
    self.gasApplySources = [self pipelineNamed:@"gas_apply_sources" error:&pipelineError];
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
    size_t gpuOnlyBytes = cellBytes * 4 + cellBytes * 2 + gasPrimBytes +
        (size_t)maxModes * kModeAnchors * 3 * sizeof(float) +
        (size_t)maxModes * 4 * sizeof(float) +
        particleCicBytes * 2;
    MTLHeapDescriptor *heapDescriptor = [[MTLHeapDescriptor alloc] init];
    heapDescriptor.size = gpuOnlyBytes + (4u << 20);
    heapDescriptor.storageMode = MTLStorageModePrivate;
    self.gpuHeap = [self.device newHeapWithDescriptor:heapDescriptor];
    self.residentBytes = 0;

    if (self.gpuHeap != nil) {
        self.residentBytes += (uint64_t)self.gpuHeap.size;
    }

    self.momRho = [self trackResident:[self newSharedBufferWithLength:cellBytes * 4]];
    self.eInt = [self trackResident:[self newSharedBufferWithLength:cellBytes]];
    self.momRhoStage = [self trackResident:[self newGPUBufferWithLength:cellBytes * 4]];
    self.eStage = [self trackResident:[self newGPUBufferWithLength:cellBytes]];
    self.entropyStage = [self trackResident:[self newGPUBufferWithLength:cellBytes]];
    self.gasPrim = [self trackResident:[self newGPUBufferWithLength:gasPrimBytes]];
    self.particleCicA = [self trackResident:[self newGPUBufferWithLength:particleCicBytes]];
    self.particleCicB = [self trackResident:[self newGPUBufferWithLength:particleCicBytes]];
    self.gasParams = [self trackResident:[self.device newBufferWithLength:sizeof(GasGridParamsHost) options:MTLResourceStorageModeShared]];
    self.sourceMomRho = [self trackResident:[self newSharedBufferWithLength:cellBytes * 4]];
    self.sourceE = [self trackResident:[self newSharedBufferWithLength:cellBytes]];
    self.dbgCap = [self trackResident:[self.device newBufferWithLength:sizeof(uint32_t) options:MTLResourceStorageModeShared]];
    self.dbgHead = [self trackResident:[self.device newBufferWithLength:sizeof(uint32_t) options:MTLResourceStorageModeShared]];
    self.dbgWords = [self trackResident:[self.device
        newBufferWithLength:(size_t)self.numCells * 6u * sizeof(uint32_t)
        options:MTLResourceStorageModeShared
    ]];

    self.oscPhase = [self trackResident:[self.device newBufferWithLength:(size_t)maxModes * sizeof(float) options:MTLResourceStorageModeShared]];
    self.oscOmega = [self trackResident:[self.device newBufferWithLength:(size_t)maxModes * sizeof(float) options:MTLResourceStorageModeShared]];
    self.oscAmp = [self trackResident:[self.device newBufferWithLength:(size_t)maxModes * sizeof(float) options:MTLResourceStorageModeShared]];
    self.oscHeat = [self trackResident:[self.device newBufferWithLength:(size_t)maxModes * sizeof(float) options:MTLResourceStorageModeShared]];
    self.particlePos = [self trackResident:[self.device newBufferWithLength:(size_t)maxModes * 3 * sizeof(float) options:MTLResourceStorageModeShared]];
    self.particleVel = [self trackResident:[self.device newBufferWithLength:(size_t)maxModes * 3 * sizeof(float) options:MTLResourceStorageModeShared]];
    self.particleMass = [self trackResident:[self.device newBufferWithLength:(size_t)maxModes * sizeof(float) options:MTLResourceStorageModeShared]];
    self.particleEnergy = [self trackResident:[self.device newBufferWithLength:(size_t)maxModes * sizeof(float) options:MTLResourceStorageModeShared]];
    self.particlePosSorted = [self trackResident:[self.device newBufferWithLength:(size_t)maxModes * 3 * sizeof(float) options:MTLResourceStorageModeShared]];
    self.particleVelSorted = [self trackResident:[self.device newBufferWithLength:(size_t)maxModes * 3 * sizeof(float) options:MTLResourceStorageModeShared]];
    self.particleMassSorted = [self trackResident:[self.device newBufferWithLength:(size_t)maxModes * sizeof(float) options:MTLResourceStorageModeShared]];
    self.particleHeatSorted = [self trackResident:[self.device newBufferWithLength:(size_t)maxModes * sizeof(float) options:MTLResourceStorageModeShared]];
    self.particleEnergySorted = [self trackResident:[self.device newBufferWithLength:(size_t)maxModes * sizeof(float) options:MTLResourceStorageModeShared]];
    self.particleCellIdx = [self trackResident:[self.device newBufferWithLength:(size_t)maxModes * sizeof(uint32_t) options:MTLResourceStorageModeShared]];
    self.scatterCellCounts = [self trackResident:[self.device newBufferWithLength:cellBytes options:MTLResourceStorageModeShared]];
    self.scatterCellStarts = [self trackResident:[self.device newBufferWithLength:cellBytes options:MTLResourceStorageModeShared]];
    self.scatterCellOffsets = [self trackResident:[self.device newBufferWithLength:cellBytes options:MTLResourceStorageModeShared]];
    self.sortedOriginalIdx = [self trackResident:[self.device newBufferWithLength:(size_t)maxModes * sizeof(uint32_t) options:MTLResourceStorageModeShared]];
    self.rhoAtomic = [self trackResident:[self.device newBufferWithLength:cellBytes options:MTLResourceStorageModeShared]];
    self.momAtomic = [self trackResident:[self.device newBufferWithLength:cellBytes * 3 options:MTLResourceStorageModeShared]];
    self.eAtomic = [self trackResident:[self.device newBufferWithLength:cellBytes options:MTLResourceStorageModeShared]];
    self.gravityPotential = [self trackResident:[self.device newBufferWithLength:cellBytes options:MTLResourceStorageModeShared]];
    self.sortScatterParams = [self trackResident:[self.device newBufferWithLength:sizeof(SortScatterParamsHost) options:MTLResourceStorageModeShared]];
    self.picGatherParams = [self trackResident:[self.device newBufferWithLength:sizeof(PicGatherParamsHost) options:MTLResourceStorageModeShared]];
    self.particleExcitation = [self trackResident:[self.device newBufferWithLength:(size_t)maxModes * sizeof(float) options:MTLResourceStorageModeShared]];
    self.particleVelIn = [self trackResident:[self.device newBufferWithLength:(size_t)maxModes * 3 * sizeof(float) options:MTLResourceStorageModeShared]];
    self.particleHeatIn = [self trackResident:[self.device newBufferWithLength:(size_t)maxModes * sizeof(float) options:MTLResourceStorageModeShared]];
    self.hashCellCounts = [self trackResident:[self.device newBufferWithLength:cellBytes options:MTLResourceStorageModeShared]];
    self.hashCellStarts = [self trackResident:[self.device newBufferWithLength:((size_t)self.numCells + 1u) * sizeof(uint32_t) options:MTLResourceStorageModeShared]];
    self.hashCellOffsets = [self trackResident:[self.device newBufferWithLength:cellBytes options:MTLResourceStorageModeShared]];
    self.hashSortedIdx = [self trackResident:[self.device newBufferWithLength:(size_t)maxModes * sizeof(uint32_t) options:MTLResourceStorageModeShared]];
    self.hashParticleCellIdx = [self trackResident:[self.device newBufferWithLength:(size_t)maxModes * sizeof(uint32_t) options:MTLResourceStorageModeShared]];
    self.hashNumCellsBuf = [self trackResident:[self.device newBufferWithLength:sizeof(uint32_t) options:MTLResourceStorageModeShared]];
    self.hashNumParticlesBuf = [self trackResident:[self.device newBufferWithLength:sizeof(uint32_t) options:MTLResourceStorageModeShared]];
    *(uint32_t *)self.hashNumCellsBuf.contents = self.numCells;
    self.spatialHashParams = [self trackResident:[self.device newBufferWithLength:sizeof(SpatialHashParamsHost) options:MTLResourceStorageModeShared]];
    self.spatialCollisionParams = [self trackResident:[self.device newBufferWithLength:sizeof(SpatialCollisionParamsHost) options:MTLResourceStorageModeShared]];
    self.particleInteractionParams = [self trackResident:[self.device newBufferWithLength:sizeof(ParticleInteractionParamsHost) options:MTLResourceStorageModeShared]];
    self.psiReField = [self trackResident:[self.device newBufferWithLength:cellBytes options:MTLResourceStorageModeShared]];
    self.psiImField = [self trackResident:[self.device newBufferWithLength:cellBytes options:MTLResourceStorageModeShared]];
    self.psiReAtomic = [self trackResident:[self.device newBufferWithLength:cellBytes options:MTLResourceStorageModeShared]];
    self.psiImAtomic = [self trackResident:[self.device newBufferWithLength:cellBytes options:MTLResourceStorageModeShared]];
    self.modeProjectParams = [self trackResident:[self.device newBufferWithLength:sizeof(ModeProjectParamsHost) options:MTLResourceStorageModeShared]];
    self.pilotWaveParams = [self trackResident:[self.device newBufferWithLength:sizeof(PilotWaveParamsHost) options:MTLResourceStorageModeShared]];
    self.particleGenParams = [self trackResident:[self.device newBufferWithLength:sizeof(ParticleGenParamsHost) options:MTLResourceStorageModeShared]];
    self.particleRandomVals = [self trackResident:[self.device newBufferWithLength:(size_t)maxModes * 4 * sizeof(float) options:MTLResourceStorageModeShared]];
    self.reduceGroupStats = [self trackResident:[self.device newBufferWithLength:(size_t)maxModes * 4 * sizeof(float) options:MTLResourceStorageModeShared]];
    self.reduceStatsOut = [self trackResident:[self.device newBufferWithLength:4 * sizeof(float) options:MTLResourceStorageModeShared]];
    self.hashBlockSums = [self trackResident:[self.device newBufferWithLength:(size_t)maxModes * sizeof(uint32_t) options:MTLResourceStorageModeShared]];
    self.gravityReady = NO;
    self.modeReal = [self trackResident:[self.device newBufferWithLength:(size_t)maxModes * sizeof(float) options:MTLResourceStorageModeShared]];
    self.modeImag = [self trackResident:[self.device newBufferWithLength:(size_t)maxModes * sizeof(float) options:MTLResourceStorageModeShared]];
    self.modeOmega = [self trackResident:[self.device newBufferWithLength:(size_t)maxModes * sizeof(float) options:MTLResourceStorageModeShared]];
    self.modeGate = [self trackResident:[self.device newBufferWithLength:(size_t)maxModes * sizeof(float) options:MTLResourceStorageModeShared]];
    self.modeAnchorIdx = [self trackResident:[self.device newBufferWithLength:(size_t)maxModes * 8 * sizeof(uint32_t) options:MTLResourceStorageModeShared]];
    self.modeAnchorWeight = [self trackResident:[self.device newBufferWithLength:(size_t)maxModes * 8 * sizeof(float) options:MTLResourceStorageModeShared]];
    self.modeAnchorPos = [self trackResident:[self newGPUBufferWithLength:(size_t)maxModes * kModeAnchors * 3 * sizeof(float)]];
    self.oscCouplingPrep = [self trackResident:[self newGPUBufferWithLength:(size_t)maxModes * 4 * sizeof(float)]];
    self.accums = [self trackResident:[self.device newBufferWithLength:(size_t)maxModes * sizeof(CarrierAccumHost) options:MTLResourceStorageModeShared]];
    self.numCarriers = [self trackResident:[self.device newBufferWithLength:sizeof(uint32_t) options:MTLResourceStorageModeShared]];
    self.omegaMinKey = [self trackResident:[self.device newBufferWithLength:sizeof(uint32_t) options:MTLResourceStorageModeShared]];
    self.omegaMaxKey = [self trackResident:[self.device newBufferWithLength:sizeof(uint32_t) options:MTLResourceStorageModeShared]];
    self.binCounts = [self trackResident:[self.device newBufferWithLength:(size_t)maxModes * sizeof(uint32_t) options:MTLResourceStorageModeShared]];
    self.binStarts = [self trackResident:[self.device newBufferWithLength:((size_t)maxModes + 1) * sizeof(uint32_t) options:MTLResourceStorageModeShared]];
    self.binOffsets = [self trackResident:[self.device newBufferWithLength:(size_t)maxModes * sizeof(uint32_t) options:MTLResourceStorageModeShared]];
    self.carrierBinnedIdx = [self trackResident:[self.device newBufferWithLength:(size_t)maxModes * sizeof(uint32_t) options:MTLResourceStorageModeShared]];
    self.binParams = [self trackResident:[self.device newBufferWithLength:sizeof(BinParamsHost) options:MTLResourceStorageModeShared]];
    self.numBinsBuf = [self trackResident:[self.device newBufferWithLength:sizeof(uint32_t) options:MTLResourceStorageModeShared]];
    self.gateWidthMaxBuf = [self trackResident:[self.device newBufferWithLength:sizeof(float) options:MTLResourceStorageModeShared]];
    *(float *)self.gateWidthMaxBuf.contents = self.config.gate_width_max;
    self.scanBlockSums = [self trackResident:[self.device newBufferWithLength:(size_t)maxModes * sizeof(uint32_t) options:MTLResourceStorageModeShared]];
    self.scanBlockPrefix = [self trackResident:[self.device newBufferWithLength:(size_t)maxModes * sizeof(uint32_t) options:MTLResourceStorageModeShared]];
    self.scanBlockScratch = [self trackResident:[self.device newBufferWithLength:(size_t)maxModes * sizeof(uint32_t) options:MTLResourceStorageModeShared]];
    self.coherenceParams = [self trackResident:[self.device newBufferWithLength:sizeof(CoherenceParamsHost) options:MTLResourceStorageModeShared]];
    self.gpeParams = [self trackResident:[self.device newBufferWithLength:sizeof(GPEParamsHost) options:MTLResourceStorageModeShared]];

    [self resetDepositsInternal];
    [self resetSourcesInternal];
    *(uint32_t *)self.dbgCap.contents = self.numCells;
    *(uint32_t *)self.dbgHead.contents = 0;

    return YES;
}

- (void)resetDepositsInternal {
    [self runClearField:self.momRho count:(self.numCells * 4)];
    [self runClearField:self.eInt count:self.numCells];
    [self resetSourcesInternal];
}

- (void)resetSourcesInternal {
    [self runClearField:self.sourceMomRho count:(self.numCells * 4)];
    [self runClearField:self.sourceE count:self.numCells];
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
    float *sourceMomRhoData = (float *)self.sourceMomRho.contents;
    float *sourceEData = (float *)self.sourceE.contents;

    uint32_t base = index * 4;
    sourceMomRhoData[base + 0] += momX;
    sourceMomRhoData[base + 1] += momY;
    sourceMomRhoData[base + 2] += momZ;
    sourceMomRhoData[base + 3] += rho;
    sourceEData[index] += eInt;

    return YES;
}

- (BOOL)sourceCell:(uint32_t)cellX cellY:(uint32_t)cellY cellZ:(uint32_t)cellZ
          deltaMomX:(float)deltaMomX deltaMomY:(float)deltaMomY deltaMomZ:(float)deltaMomZ
           deltaRho:(float)deltaRho deltaE:(float)deltaE error:(NSString **)error {
    if (cellX >= self.config.grid_x || cellY >= self.config.grid_y || cellZ >= self.config.grid_z) {
        if (error != nil) {
            *error = @"source cell out of bounds";
        }

        return NO;
    }

    if (!isfinite(deltaMomX) || !isfinite(deltaMomY) || !isfinite(deltaMomZ) ||
        !isfinite(deltaRho) || !isfinite(deltaE)) {
        if (error != nil) {
            *error = @"source deltas must be finite";
        }

        return NO;
    }

    uint32_t index = manifold_cell_index(cellX, cellY, cellZ, self.config.grid_x, self.config.grid_y, self.config.grid_z);
    float *sourceMomRhoData = (float *)self.sourceMomRho.contents;
    float *sourceEData = (float *)self.sourceE.contents;
    uint32_t base = index * 4;

    sourceMomRhoData[base + 0] += deltaMomX;
    sourceMomRhoData[base + 1] += deltaMomY;
    sourceMomRhoData[base + 2] += deltaMomZ;
    sourceMomRhoData[base + 3] += deltaRho;
    sourceEData[index] += deltaE;

    return YES;
}

- (BOOL)readCell:(uint32_t)cellX cellY:(uint32_t)cellY cellZ:(uint32_t)cellZ
             rho:(float *)rho momX:(float *)momX momY:(float *)momY momZ:(float *)momZ
            eInt:(float *)eInt error:(NSString **)error {
    if (rho == NULL || momX == NULL || momY == NULL || momZ == NULL || eInt == NULL) {
        if (error != nil) {
            *error = @"cell read buffers are required";
        }

        return NO;
    }

    if (cellX >= self.config.grid_x || cellY >= self.config.grid_y || cellZ >= self.config.grid_z) {
        if (error != nil) {
            *error = @"read cell out of bounds";
        }

        return NO;
    }

    uint32_t index = manifold_cell_index(cellX, cellY, cellZ, self.config.grid_x, self.config.grid_y, self.config.grid_z);
    float *momRhoData = (float *)self.momRho.contents;
    float *eData = (float *)self.eInt.contents;
    uint32_t base = index * 4;

    *momX = momRhoData[base + 0];
    *momY = momRhoData[base + 1];
    *momZ = momRhoData[base + 2];
    *rho = momRhoData[base + 3];
    *eInt = eData[index];

    return YES;
}

- (BOOL)conservedStateIsFinite:(NSString **)error {
    float *momRhoData = (float *)self.momRho.contents;
    float *eData = (float *)self.eInt.contents;

    for (uint32_t index = 0; index < self.numCells; index++) {
        uint32_t base = index * 4;

        if (!isfinite(momRhoData[base + 0]) || !isfinite(momRhoData[base + 1]) ||
            !isfinite(momRhoData[base + 2]) || !isfinite(momRhoData[base + 3]) ||
            !isfinite(eData[index])) {
            if (error != nil) {
                uint32_t debugCount = *(uint32_t *)self.dbgHead.contents;
                uint32_t *debugWords = (uint32_t *)self.dbgWords.contents;

                if (debugCount == 0) {
                    *error = [NSString stringWithFormat:
                        @"conserved gas state is not finite at cell %u: rho=%g mom=(%g,%g,%g) e_int=%g",
                        index,
                        momRhoData[base + 3],
                        momRhoData[base + 0],
                        momRhoData[base + 1],
                        momRhoData[base + 2],
                        eData[index]
                    ];

                    return NO;
                }

                *error = [NSString stringWithFormat:
                    @"conserved gas state is not finite at cell %u: rho=%g mom=(%g,%g,%g) e_int=%g; "
                    @"gpu tag=0x%x gid=%u values=(%g,%g,%g,%g)",
                    index,
                    momRhoData[base + 3],
                    momRhoData[base + 0],
                    momRhoData[base + 1],
                    momRhoData[base + 2],
                    eData[index],
                    debugWords[0],
                    debugWords[1],
                    manifold_debug_float(debugWords[2]),
                    manifold_debug_float(debugWords[3]),
                    manifold_debug_float(debugWords[4]),
                    manifold_debug_float(debugWords[5])
                ];
            }

            return NO;
        }
    }

    return YES;
}

- (BOOL)applySources:(NSString **)error {
    [self configureGasParams];
    *(uint32_t *)self.dbgHead.contents = 0;

    [self dispatchGasBrickSynchronized:self.gasApplySources
                               buffers:@[
                                   self.momRho,
                                   self.eInt,
                                   self.sourceMomRho,
                                   self.sourceE,
                                   self.momRhoStage,
                                   self.eStage,
                                   self.gasParams,
                                   self.dbgHead,
                                   self.dbgWords,
                                   self.dbgCap
                               ]];

    [self runCopyFloat:self.momRhoStage dst:self.momRho count:(self.numCells * 4)];
    [self runCopyFloat:self.eStage dst:self.eInt count:self.numCells];

    if (self.stepDispatchActive) {
        [self flushStepDispatches];
    }

    [self resetSourcesInternal];

    return [self conservedStateIsFinite:error];
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

    for (uint32_t index = 0; index < count; index++) {
        const ManifoldOscillator *oscillator = &oscillators[index];

        if (!isfinite(oscillator->phase) || !isfinite(oscillator->omega) ||
            !isfinite(oscillator->amplitude) || !(oscillator->amplitude > 0.0f) ||
            !isfinite(oscillator->pos_x) ||
            !isfinite(oscillator->pos_y) || !isfinite(oscillator->pos_z) ||
            !isfinite(oscillator->heat) || oscillator->heat < 0.0f ||
            !isfinite(oscillator->vel_x) ||
            !isfinite(oscillator->vel_y) || !isfinite(oscillator->vel_z)) {
            if (error != nil) {
                *error = @"oscillator state must be finite";
            }

            return NO;
        }
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
            posData[index * 3 + 0] = manifold_wrap_coordinate(oscillator->pos_x, self.config.domain_x);
            posData[index * 3 + 1] = manifold_wrap_coordinate(oscillator->pos_y, self.config.domain_y);
            posData[index * 3 + 2] = manifold_wrap_coordinate(oscillator->pos_z, self.config.domain_z);
        }
    }

    if (needsGeneratedPositions) {
        [self configureParticleGenParams];
        [self seedRandomValuesFromOscillators:oscillators count:count];
        [self dispatchGridKernel:self.generateParticlePositions
                         buffers:@[self.particlePos, self.particleRandomVals, self.particleGenParams]
                     threadCount:count];
    }

    [self initializeParticleStateFromOscillators:oscillators count:count];

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
    float dy = self.config.domain_y / (float)self.config.grid_y;
    float dz = self.config.domain_z / (float)self.config.grid_z;

    params->num_cells = self.numCells;
    params->grid_x = self.config.grid_x;
    params->grid_y = self.config.grid_y;
    params->grid_z = self.config.grid_z;
    params->dx = dx;
    params->dy = dy;
    params->dz = dz;
    params->inv_dx = 1.0f / dx;
    params->inv_dy = 1.0f / dy;
    params->inv_dz = 1.0f / dz;
    params->inv_dx2 = params->inv_dx * params->inv_dx;
    params->inv_dy2 = params->inv_dy * params->inv_dy;
    params->inv_dz2 = params->inv_dz * params->inv_dz;
    params->dt = self.controls.dt;
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
    params->boundary_x_low = self.config.boundary_x_low;
    params->boundary_x_high = self.config.boundary_x_high;
    params->boundary_y_low = self.config.boundary_y_low;
    params->boundary_y_high = self.config.boundary_y_high;
    params->boundary_z_low = self.config.boundary_z_low;
    params->boundary_z_high = self.config.boundary_z_high;
}

- (BOOL)runGasStep:(NSString **)error {
    if (![self applySources:error]) {
        return NO;
    }

    ManifoldControls intervalControls = self.controls;
    float remainingDelta = intervalControls.dt;

    while (remainingDelta > 0.0f) {
        ManifoldControls remainingControls = intervalControls;
        remainingControls.dt = remainingDelta;
        self.controls = remainingControls;

        uint32_t substeps = 0;

        if (![self gasSubsteps:&substeps error:error]) {
            self.controls = intervalControls;
            return NO;
        }

        ManifoldControls substepControls = intervalControls;
        substepControls.dt = remainingDelta / (float)substeps;
        self.controls = substepControls;
        *(uint32_t *)self.dbgHead.contents = 0;
        [self runGasSubstep];

        if (self.stepDispatchActive) {
            [self flushStepDispatches];
        }

        if (![self conservedStateIsFinite:error]) {
            self.controls = intervalControls;
            return NO;
        }

        remainingDelta = substeps == 1
            ? 0.0f
            : remainingDelta - substepControls.dt;
    }

    self.controls = intervalControls;
    return YES;
}

- (BOOL)runGasTransport:(NSString **)error {
    return [self runGasStep:error];
}

- (BOOL)computeReading:(ManifoldReading *)reading error:(NSString **)error {
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
    float dy = self.config.domain_y / (float)gy;
    float dz = self.config.domain_z / (float)gz;
    ManifoldConfig gasConfig = self.config;
    uint32_t bxm = manifold_gas_boundary_mode(&gasConfig, 0u, false);
    uint32_t bxp = manifold_gas_boundary_mode(&gasConfig, 0u, true);
    uint32_t bym = manifold_gas_boundary_mode(&gasConfig, 1u, false);
    uint32_t byp = manifold_gas_boundary_mode(&gasConfig, 1u, true);
    uint32_t bzm = manifold_gas_boundary_mode(&gasConfig, 2u, false);
    uint32_t bzp = manifold_gas_boundary_mode(&gasConfig, 2u, true);
    ManifoldGasNeighborCoord neighbor_xm = manifold_gas_neighbor_coord(cx, cy, cz, 0u, false, gx, gy, gz, bxm);
    ManifoldGasNeighborCoord neighbor_xp = manifold_gas_neighbor_coord(cx, cy, cz, 0u, true, gx, gy, gz, bxp);
    ManifoldGasNeighborCoord neighbor_ym = manifold_gas_neighbor_coord(cx, cy, cz, 1u, false, gx, gy, gz, bym);
    ManifoldGasNeighborCoord neighbor_yp = manifold_gas_neighbor_coord(cx, cy, cz, 1u, true, gx, gy, gz, byp);
    ManifoldGasNeighborCoord neighbor_zm = manifold_gas_neighbor_coord(cx, cy, cz, 2u, false, gx, gy, gz, bzm);
    ManifoldGasNeighborCoord neighbor_zp = manifold_gas_neighbor_coord(cx, cy, cz, 2u, true, gx, gy, gz, bzp);

    float pressure_xm = manifold_gas_pressure_at(
        eData, self.config.gamma, neighbor_xm.x, neighbor_xm.y, neighbor_xm.z, gx, gy, gz,
        neighbor_xm.is_ghost, 0u, bxm, cx, cy, cz);
    float pressure_xp = manifold_gas_pressure_at(
        eData, self.config.gamma, neighbor_xp.x, neighbor_xp.y, neighbor_xp.z, gx, gy, gz,
        neighbor_xp.is_ghost, 0u, bxp, cx, cy, cz);
    float pressure_ym = manifold_gas_pressure_at(
        eData, self.config.gamma, neighbor_ym.x, neighbor_ym.y, neighbor_ym.z, gx, gy, gz,
        neighbor_ym.is_ghost, 1u, bym, cx, cy, cz);
    float pressure_yp = manifold_gas_pressure_at(
        eData, self.config.gamma, neighbor_yp.x, neighbor_yp.y, neighbor_yp.z, gx, gy, gz,
        neighbor_yp.is_ghost, 1u, byp, cx, cy, cz);
    float pressure_zm = manifold_gas_pressure_at(
        eData, self.config.gamma, neighbor_zm.x, neighbor_zm.y, neighbor_zm.z, gx, gy, gz,
        neighbor_zm.is_ghost, 2u, bzm, cx, cy, cz);
    float pressure_zp = manifold_gas_pressure_at(
        eData, self.config.gamma, neighbor_zp.x, neighbor_zp.y, neighbor_zp.z, gx, gy, gz,
        neighbor_zp.is_ghost, 2u, bzp, cx, cy, cz);

    float dpx = (pressure_xp - pressure_xm) / (2.0f * dx);
    float dpy = (pressure_yp - pressure_ym) / (2.0f * dy);
    float dpz = (pressure_zp - pressure_zm) / (2.0f * dz);

    float ux_xm, uy_xm, uz_xm, ux_xp, uy_xp, uz_xp;
    float ux_ym, uy_ym, uz_ym, ux_yp, uy_yp, uz_yp;
    float ux_zm, uy_zm, uz_zm, ux_zp, uy_zp, uz_zp;
    manifold_gas_velocity_at(
        momRhoData, neighbor_xm.x, neighbor_xm.y, neighbor_xm.z, gx, gy, gz,
        neighbor_xm.is_ghost, 0u, bxm, cx, cy, cz, &ux_xm, &uy_xm, &uz_xm);
    manifold_gas_velocity_at(
        momRhoData, neighbor_xp.x, neighbor_xp.y, neighbor_xp.z, gx, gy, gz,
        neighbor_xp.is_ghost, 0u, bxp, cx, cy, cz, &ux_xp, &uy_xp, &uz_xp);
    manifold_gas_velocity_at(
        momRhoData, neighbor_ym.x, neighbor_ym.y, neighbor_ym.z, gx, gy, gz,
        neighbor_ym.is_ghost, 1u, bym, cx, cy, cz, &ux_ym, &uy_ym, &uz_ym);
    manifold_gas_velocity_at(
        momRhoData, neighbor_yp.x, neighbor_yp.y, neighbor_yp.z, gx, gy, gz,
        neighbor_yp.is_ghost, 1u, byp, cx, cy, cz, &ux_yp, &uy_yp, &uz_yp);
    manifold_gas_velocity_at(
        momRhoData, neighbor_zm.x, neighbor_zm.y, neighbor_zm.z, gx, gy, gz,
        neighbor_zm.is_ghost, 2u, bzm, cx, cy, cz, &ux_zm, &uy_zm, &uz_zm);
    manifold_gas_velocity_at(
        momRhoData, neighbor_zp.x, neighbor_zp.y, neighbor_zp.z, gx, gy, gz,
        neighbor_zp.is_ghost, 2u, bzp, cx, cy, cz, &ux_zp, &uy_zp, &uz_zp);

    float divergence = (ux_xp - ux_xm) / (2.0f * dx) +
        (uy_yp - uy_ym) / (2.0f * dy) +
        (uz_zp - uz_zm) / (2.0f * dz);
    float coherenceMag2 = 0.0f;

    for (uint32_t index = 0; index < self.numOsc; index++) {
        float re = modeRealData[index];
        float im = modeImagData[index];
        coherenceMag2 += re * re + im * im;
    }

    if (self.numOsc > 0) {
        coherenceMag2 /= (float)self.numOsc;
    }

    // GuidanceSpeed is the mean |v| after pilot-wave gather — the Bohm current
    // actually applied to carriers — not the |Re·Im|·ħ mode proxy.
    float guidanceSpeed = 0.0f;
    float *velData = (float *)self.particleVel.contents;

    for (uint32_t index = 0; index < self.numOsc; index++) {
        float vx = velData[index * 3 + 0];
        float vy = velData[index * 3 + 1];
        float vz = velData[index * 3 + 2];
        guidanceSpeed += sqrtf(vx * vx + vy * vy + vz * vz);
    }

    if (self.numOsc > 0) {
        guidanceSpeed /= (float)self.numOsc;
    }

    reading->pressure_grad_x = dpx;
    reading->pressure_grad_y = dpy;
    reading->pressure_grad_z = dpz;
    reading->pressure_grad_norm = sqrtf(dpx * dpx + dpy * dpy + dpz * dpz);
    reading->divergence = divergence;
    reading->coherence_mag2 = coherenceMag2;
    reading->guidance_speed = guidanceSpeed;
    reading->viscosity_proxy = (fabsf(divergence) > 1e-8f) ? (1.0f / fabsf(divergence)) : 0.0f;

    if (!isfinite(reading->pressure_grad_x) || !isfinite(reading->pressure_grad_y) ||
        !isfinite(reading->pressure_grad_z) || !isfinite(reading->pressure_grad_norm) ||
        !isfinite(reading->divergence) || !isfinite(reading->coherence_mag2) ||
        !isfinite(reading->guidance_speed) || !isfinite(reading->viscosity_proxy)) {
        if (error != nil) {
            *error = @"manifold reading is not finite";
        }

        return NO;
    }

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

- (BOOL)readPilotWaveProjection:(float *)mag2Out
                          velX:(float *)velXOut
                          velZ:(float *)velZOut
                         length:(uint32_t)length
                          error:(NSString **)error {
    if (mag2Out == NULL || velXOut == NULL || velZOut == NULL) {
        if (error != NULL) {
            *error = @"pilot-wave projection buffers are required";
        }

        return NO;
    }

    float *reData = (float *)self.psiReField.contents;
    float *imData = (float *)self.psiImField.contents;
    uint32_t gx = self.config.grid_x;
    uint32_t gy = self.config.grid_y;
    uint32_t gz = self.config.grid_z;
    uint32_t expected = gx * gz;

    if (length < expected) {
        if (error != NULL) {
            *error = @"pilot-wave projection buffer is too small";
        }

        return NO;
    }

    float cellX = self.config.domain_x / (float)gx;
    float cellZ = self.config.domain_z / (float)gz;
    float hbarEff = self.config.hbar_eff;
    float ampScale = 1.0f / fmaxf((float)self.numOsc, 1.0f);
    float massFloor = fmaxf(ampScale * 1e-3f, 1e-6f);
    float massScale = 0.0f;
    float *massData = (float *)self.particleMass.contents;

    for (uint32_t index = 0; index < self.numOsc; index++) {
        massScale += fmaxf(massData[index], massFloor);
    }

    if (self.numOsc > 0) {
        massScale /= (float)self.numOsc;
    }

    massScale = fmaxf(massScale, massFloor);

    if (!(cellX > 0.0f) || !(cellZ > 0.0f) || !(hbarEff > 0.0f) || !(massScale > 0.0f)) {
        if (error != NULL) {
            *error = @"pilot-wave projection requires positive cell scale and mass floor";
        }

        return NO;
    }

    // Mirror configurePilotWaveParams: wave ε << typical |Ψ|², not rho_min².
    float epsDenom = fmaxf(1e-12f, ampScale * ampScale * 1e-8f);
    uint32_t xLow = self.config.boundary_x_low;
    uint32_t xHigh = gx > 0u ? gx - 1u - self.config.boundary_x_high : 0u;
    uint32_t yLow = self.config.boundary_y_low;
    uint32_t yHigh = gy > 0u ? gy - 1u - self.config.boundary_y_high : 0u;
    uint32_t zLow = self.config.boundary_z_low;
    uint32_t zHigh = gz > 0u ? gz - 1u - self.config.boundary_z_high : 0u;

    if (xLow >= gx) {
        xLow = 0u;
    }

    if (xHigh >= gx) {
        xHigh = gx - 1u;
    }

    if (yLow >= gy) {
        yLow = 0u;
    }

    if (yHigh >= gy) {
        yHigh = gy - 1u;
    }

    if (zLow >= gz) {
        zLow = 0u;
    }

    if (zHigh >= gz) {
        zHigh = gz - 1u;
    }

    if (xHigh < xLow) {
        xLow = 0u;
        xHigh = gx - 1u;
    }

    if (yHigh < yLow) {
        yLow = 0u;
        yHigh = gy - 1u;
    }

    if (zHigh < zLow) {
        zLow = 0u;
        zHigh = gz - 1u;
    }

    for (uint32_t zIndex = 0; zIndex < gz; zIndex++) {
        for (uint32_t xIndex = 0; xIndex < gx; xIndex++) {
            float peakMag2 = 0.0f;
            uint32_t peakY = yLow;

            for (uint32_t yIndex = yLow; yIndex <= yHigh; yIndex++) {
                uint32_t index = manifold_cell_index(xIndex, yIndex, zIndex, gx, gy, gz);
                float re = reData[index];
                float im = imData[index];
                float mag2 = re * re + im * im;

                if (mag2 > peakMag2) {
                    peakMag2 = mag2;
                    peakY = yIndex;
                }
            }

            uint32_t xMinus = xIndex > xLow ? xIndex - 1u : xIndex;
            uint32_t xPlus = xIndex < xHigh ? xIndex + 1u : xIndex;
            uint32_t zMinus = zIndex > zLow ? zIndex - 1u : zIndex;
            uint32_t zPlus = zIndex < zHigh ? zIndex + 1u : zIndex;

            uint32_t centerIndex = manifold_cell_index(xIndex, peakY, zIndex, gx, gy, gz);
            uint32_t xPlusIndex = manifold_cell_index(xPlus, peakY, zIndex, gx, gy, gz);
            uint32_t xMinusIndex = manifold_cell_index(xMinus, peakY, zIndex, gx, gy, gz);
            uint32_t zPlusIndex = manifold_cell_index(xIndex, peakY, zPlus, gx, gy, gz);
            uint32_t zMinusIndex = manifold_cell_index(xIndex, peakY, zMinus, gx, gy, gz);

            float re = reData[centerIndex];
            float im = imData[centerIndex];
            float dReDx = (reData[xPlusIndex] - reData[xMinusIndex]) / (2.0f * cellX);
            float dReDz = (reData[zPlusIndex] - reData[zMinusIndex]) / (2.0f * cellZ);
            float dImDx = (imData[xPlusIndex] - imData[xMinusIndex]) / (2.0f * cellX);
            float dImDz = (imData[zPlusIndex] - imData[zMinusIndex]) / (2.0f * cellZ);
            // Match the pilot-wave advection kernel: Bohmian denominator is
            // |psi|^2 + eps_denom, and velocity scaling is hbar_eff / mass_min.
            float denom = re * re + im * im + epsDenom;
            float currentX = 0.0f;
            float currentZ = 0.0f;

            if (isfinite(re) && isfinite(im) && isfinite(denom) && denom > 0.0f &&
                isfinite(dReDx) && isfinite(dReDz) && isfinite(dImDx) && isfinite(dImDz)) {
                currentX = (re * dImDx - im * dReDx) / denom;
                currentZ = (re * dImDz - im * dReDz) / denom;
            }

            uint32_t outIndex = xIndex + zIndex * gx;

            mag2Out[outIndex] = peakMag2;
            // Match pic_gather_update_particles_pilot_wave: v = (ħ/m) · j
            // with the same mass floor the gather kernel uses.
            float invMass = 1.0f / massScale;
            velXOut[outIndex] = currentX * hbarEff * invMass;
            velZOut[outIndex] = currentZ * hbarEff * invMass;
        }
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
    [self runClearField:self.momRho count:(self.numCells * 4)];
    [self runClearField:self.eInt count:self.numCells];

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

    if (![self conservedStateIsFinite:error]) {
        return NO;
    }

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

    return [self computeReading:reading error:error];
}

@end


void *manifold_engine_create(
    const ManifoldConfig *config,
    const void *metallib_bytes,
    size_t metallib_length,
    char *err_out,
    int err_cap
) {
    @autoreleasepool {
        NSString *error = nil;
        ManifoldSolver *host = manifold_shared_metal_host(metallib_bytes, metallib_length, config, &error);

        if (host == nil) {
            manifold_write_error(err_out, err_cap, error ?: @"failed to create manifold engine");
            return NULL;
        }

        // Opaque sentinel — must not be the host, so destroy cannot free Metal.
        return (void *)0x1;
    }
}

void manifold_engine_destroy(void *handle) {
    (void)handle;
}

void *manifold_field_create(
    void *engine,
    const ManifoldConfig *config,
    char *err_out,
    int err_cap
) {
    @autoreleasepool {
        if (engine == NULL || config == NULL) {
            manifold_write_error(err_out, err_cap, @"engine and config are required");
            return NULL;
        }

        if (gManifoldMetallib == nil) {
            manifold_write_error(err_out, err_cap, @"shared metallib is not installed");
            return NULL;
        }

        NSString *error = nil;
        ManifoldSolver *field = [[ManifoldSolver alloc] initWithConfig:config
                                                         metallibBytes:gManifoldMetallib.bytes
                                                        metallibLength:gManifoldMetallib.length
                                                                 error:&error];

        if (field == nil) {
            manifold_write_error(err_out, err_cap, error ?: @"failed to create manifold field");
            return NULL;
        }

        return (__bridge_retained void *)field;
    }
}

uint64_t manifold_solver_resident_bytes(void *handle) {
    if (handle == NULL) {
        return 0;
    }

    ManifoldSolver *solver = (__bridge ManifoldSolver *)handle;
    return solver.residentBytes;
}

uint64_t manifold_device_working_set_budget(void) {
    ManifoldSolver *host = manifold_metal_host_load();

    if (host == nil || host.device == nil) {
        id<MTLDevice> device = MTLCreateSystemDefaultDevice();

        if (device == nil) {
            return 0;
        }

        return (uint64_t)device.recommendedMaxWorkingSetSize;
    }

    return (uint64_t)host.device.recommendedMaxWorkingSetSize;
}

uint64_t manifold_device_allocated_bytes(void) {
    ManifoldSolver *host = manifold_metal_host_load();

    if (host == nil || host.device == nil) {
        return 0;
    }

    return (uint64_t)host.device.currentAllocatedSize;
}

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

int manifold_solver_set_controls(
    void *handle,
    const ManifoldControls *controls,
    char *err_out,
    int err_cap
) {
    if (handle == NULL || controls == NULL) {
        manifold_write_error(err_out, err_cap, @"solver handle and runtime controls are required");
        return 1;
    }

    ManifoldSolver *solver = (__bridge ManifoldSolver *)handle;
    NSString *error = nil;

    if (![solver setControls:controls error:&error]) {
        manifold_write_error(err_out, err_cap, error ?: @"set controls failed");
        return 1;
    }

    return 0;
}

int manifold_solver_reset_deposits(void *handle, char *err_out, int err_cap) {
    if (handle == NULL) {
        manifold_write_error(err_out, err_cap, @"solver handle is nil");
        return 1;
    }

    ManifoldSolver *solver = (__bridge ManifoldSolver *)handle;
    [solver resetDepositsInternal];
    [solver resetSourcesInternal];
    return 0;
}

int manifold_solver_reset_sources(void *handle, char *err_out, int err_cap) {
    if (handle == NULL) {
        manifold_write_error(err_out, err_cap, @"solver handle is nil");
        return 1;
    }

    ManifoldSolver *solver = (__bridge ManifoldSolver *)handle;
    [solver resetSourcesInternal];
    return 0;
}

int manifold_solver_source_cell(
    void *handle,
    uint32_t cell_x,
    uint32_t cell_y,
    uint32_t cell_z,
    float delta_mom_x,
    float delta_mom_y,
    float delta_mom_z,
    float delta_rho,
    float delta_e,
    char *err_out,
    int err_cap
) {
    if (handle == NULL) {
        manifold_write_error(err_out, err_cap, @"solver handle is nil");
        return 1;
    }

    ManifoldSolver *solver = (__bridge ManifoldSolver *)handle;
    NSString *error = nil;

    if (![solver sourceCell:cell_x cellY:cell_y cellZ:cell_z
                  deltaMomX:delta_mom_x deltaMomY:delta_mom_y deltaMomZ:delta_mom_z
                   deltaRho:delta_rho deltaE:delta_e error:&error]) {
        manifold_write_error(err_out, err_cap, error ?: @"source cell failed");
        return 1;
    }

    return 0;
}

int manifold_solver_apply_sources(void *handle, char *err_out, int err_cap) {
    if (handle == NULL) {
        manifold_write_error(err_out, err_cap, @"solver handle is nil");
        return 1;
    }

    ManifoldSolver *solver = (__bridge ManifoldSolver *)handle;
    NSString *error = nil;

    if (![solver applySources:&error]) {
        manifold_write_error(err_out, err_cap, error ?: @"apply sources failed");
        return 1;
    }

    return 0;
}

int manifold_solver_read_cell(
    void *handle,
    uint32_t cell_x,
    uint32_t cell_y,
    uint32_t cell_z,
    float *rho,
    float *mom_x,
    float *mom_y,
    float *mom_z,
    float *e_int,
    char *err_out,
    int err_cap
) {
    if (handle == NULL) {
        manifold_write_error(err_out, err_cap, @"solver handle is nil");
        return 1;
    }

    ManifoldSolver *solver = (__bridge ManifoldSolver *)handle;
    NSString *error = nil;

    if (![solver readCell:cell_x cellY:cell_y cellZ:cell_z
                     rho:rho momX:mom_x momY:mom_y momZ:mom_z
                    eInt:e_int error:&error]) {
        manifold_write_error(err_out, err_cap, error ?: @"read cell failed");
        return 1;
    }

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
    @autoreleasepool {
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
}

int manifold_solver_step(void *handle, ManifoldReading *reading, char *err_out, int err_cap) {
    @autoreleasepool {
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
}

int manifold_solver_run_gas_transport(void *handle, char *err_out, int err_cap) {
    if (handle == NULL) {
        manifold_write_error(err_out, err_cap, @"solver handle is nil");
        return 1;
    }

    ManifoldSolver *solver = (__bridge ManifoldSolver *)handle;
    NSString *error = nil;

    if (![solver runGasTransport:&error]) {
        manifold_write_error(err_out, err_cap, error ?: @"gas transport failed");
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

int manifold_solver_read_pilot_wave_projection(
    void *handle,
    float *mag2_out,
    float *vel_x_out,
    float *vel_z_out,
    uint32_t out_length,
    uint32_t *grid_x,
    uint32_t *grid_z,
    char *err_out,
    int err_cap
) {
    if (handle == NULL || mag2_out == NULL || vel_x_out == NULL || vel_z_out == NULL ||
        grid_x == NULL || grid_z == NULL) {
        manifold_write_error(err_out, err_cap, @"solver handle and pilot-wave projection buffers are required");
        return 1;
    }

    ManifoldSolver *solver = (__bridge ManifoldSolver *)handle;
    NSString *error = nil;

    if (![solver readPilotWaveProjection:mag2_out
                                    velX:vel_x_out
                                    velZ:vel_z_out
                                  length:out_length
                                   error:&error]) {
        manifold_write_error(err_out, err_cap, error ?: @"pilot-wave projection read failed");
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
