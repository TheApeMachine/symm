#pragma once

#import <Foundation/Foundation.h>
#import <Metal/Metal.h>
#include "bridge.h"
#include <math.h>
#include <stddef.h>
#include <string.h>

static const uint32_t kMaxCarriersForTG = 256u;
static const uint32_t kCarrierTileSize = 256u;
static const uint32_t kModeAnchors = 8u;
static const uint32_t kCarrierAccumThreadgroupBytes = 8u * (uint32_t)sizeof(uint32_t);
static const uint32_t kScanThreads = 256u;
static const uint32_t kGasBrickX = 2u;
static const uint32_t kGasBrickY = 4u;
static const uint32_t kGasBrickZ = 32u;
static const uint32_t kHeavyKernelThreads = 256u;

static inline NSUInteger manifold_pipeline_max_threads(id<MTLComputePipelineState> pipeline) {
    if (pipeline == nil) {
        return 1u;
    }

    NSUInteger hwMax = pipeline.maxTotalThreadsPerThreadgroup;

    if (hwMax == 0u) {
        hwMax = 1u;
    }

    return hwMax;
}

static inline NSUInteger manifold_clamp_threadgroup_width(
    NSUInteger requested,
    id<MTLComputePipelineState> pipeline
) {
    if (requested == 0u) {
        requested = 1u;
    }

    NSUInteger hwMax = manifold_pipeline_max_threads(pipeline);

    if (requested > hwMax) {
        return hwMax;
    }

    return requested;
}

static inline uint32_t manifold_max_carriers_for_threadgroup(id<MTLDevice> device) {
    uint32_t memoryLimit = (uint32_t)(device.maxThreadgroupMemoryLength / kCarrierAccumThreadgroupBytes);
    uint32_t threadLimit = (uint32_t)device.maxThreadsPerThreadgroup.width;

    if (memoryLimit > kMaxCarriersForTG) {
        memoryLimit = kMaxCarriersForTG;
    }

    if (threadLimit > kMaxCarriersForTG) {
        threadLimit = kMaxCarriersForTG;
    }

    return memoryLimit < threadLimit ? memoryLimit : threadLimit;
}

static inline uint32_t manifold_max_carriers_for_pipeline(
    id<MTLDevice> device,
    id<MTLComputePipelineState> pipeline
) {
    uint32_t memoryLimit = (uint32_t)(device.maxThreadgroupMemoryLength / kCarrierAccumThreadgroupBytes);
    uint32_t threadLimit = (uint32_t)manifold_pipeline_max_threads(pipeline);

    if (memoryLimit > kMaxCarriersForTG) {
        memoryLimit = kMaxCarriersForTG;
    }

    if (threadLimit > kMaxCarriersForTG) {
        threadLimit = kMaxCarriersForTG;
    }

    return memoryLimit < threadLimit ? memoryLimit : threadLimit;
}

static inline NSUInteger manifold_simd_threadgroup_width(
    NSUInteger count,
    NSUInteger simdWidth,
    NSUInteger maxThreadsPerThreadgroup
) {
    if (simdWidth == 0) {
        simdWidth = 32;
    }

    if (count == 0) {
        return simdWidth;
    }

    NSUInteger aligned = ((count + simdWidth - 1) / simdWidth) * simdWidth;

    if (aligned > maxThreadsPerThreadgroup) {
        return maxThreadsPerThreadgroup;
    }

    return aligned;
}

static inline NSUInteger manifold_simd_threadgroup_width_for_pipeline(
    NSUInteger count,
    NSUInteger simdWidth,
    id<MTLComputePipelineState> pipeline
) {
    return manifold_simd_threadgroup_width(
        count,
        simdWidth,
        manifold_pipeline_max_threads(pipeline)
    );
}

enum {
    GasBoundaryPeriodic = 0u,
    GasBoundaryOutflow = 1u,
    GasBoundaryReflecting = 2u,
};

typedef struct GasGridParamsHost {
    uint32_t num_cells;
    uint32_t grid_x;
    uint32_t grid_y;
    uint32_t grid_z;
    float dx;
    float dy;
    float dz;
    float inv_dx;
    float inv_dy;
    float inv_dz;
    float inv_dx2;
    float inv_dy2;
    float inv_dz2;
    float dt;
    float gamma;
    float c_v;
    float rho_min;
    float p_min;
    float mu;
    float k_thermal;
    uint32_t boundary_x_low;
    uint32_t boundary_x_high;
    uint32_t boundary_y_low;
    uint32_t boundary_y_high;
    uint32_t boundary_z_low;
    uint32_t boundary_z_high;
} GasGridParamsHost;

typedef struct BinParamsHost {
    float omega_min;
    float inv_bin_width;
} BinParamsHost;

typedef struct CoherenceParamsHost {
    uint32_t num_osc;
    uint32_t max_carriers;
    uint32_t num_carriers;
    float dt;
    float coupling_scale;
    float carrier_reg;
    uint32_t rng_seed;
    float conflict_threshold;
    float offender_weight_floor;
    float gate_width_min;
    float gate_width_max;
    float ema_alpha;
    float recenter_alpha;
    uint32_t mode;
    float anchor_random_eps;
    float stable_amp_threshold;
    float crystallize_amp_threshold;
    float crystallize_conflict_threshold;
    uint32_t crystallize_age;
    float crystallized_coupling_boost;
    float volatile_decay_mul;
    float stable_decay_mul;
    float crystallized_decay_mul;
    float topdown_phase_scale;
    float topdown_energy_scale;
    float topdown_random_energy_eps;
    float repulsion_scale;
    float domain_x;
    float domain_y;
    float domain_z;
    float spatial_sigma;
    float metabolic_rate;
    uint32_t carrier_tile_base;
    uint32_t carrier_tile_count;
} CoherenceParamsHost;

typedef struct GPEParamsHost {
    float dt;
    float hbar_eff;
    float mass_eff;
    float g_interaction;
    float energy_decay;
    float chemical_potential;
    float inv_domega2;
    uint32_t anchors;
    uint32_t rng_seed;
    float anchor_eps;
} GPEParamsHost;

typedef struct CarrierAccumHost {
    float force_r;
    float force_i;
    float w_sum;
    float w_omega_sum;
    float w_omega2_sum;
    float w_amp_sum;
    uint32_t offender_score;
    uint32_t offender_idx;
} CarrierAccumHost;

typedef struct SortScatterParamsHost {
    uint32_t num_particles;
    uint32_t num_cells;
    uint32_t grid_x;
    uint32_t grid_y;
    uint32_t grid_z;
    float cell_x;
    float cell_y;
    float cell_z;
    float inv_cell_x;
    float inv_cell_y;
    float inv_cell_z;
    uint32_t boundary_x_low;
    uint32_t boundary_x_high;
    uint32_t boundary_y_low;
    uint32_t boundary_y_high;
    uint32_t boundary_z_low;
    uint32_t boundary_z_high;
} SortScatterParamsHost;

typedef struct PicGatherParamsHost {
    uint32_t num_particles;
    uint32_t grid_x;
    uint32_t grid_y;
    uint32_t grid_z;
    float cell_x;
    float cell_y;
    float cell_z;
    float inv_cell_x;
    float inv_cell_y;
    float inv_cell_z;
    float dt;
    float domain_x;
    float domain_y;
    float domain_z;
    float gamma;
    float R_specific;
    float c_v;
    float rho_min;
    float p_min;
    float gravity_enabled;
    uint32_t boundary_x_low;
    uint32_t boundary_x_high;
    uint32_t boundary_y_low;
    uint32_t boundary_y_high;
    uint32_t boundary_z_low;
    uint32_t boundary_z_high;
} PicGatherParamsHost;

typedef struct SpatialHashParamsHost {
    uint32_t num_particles;
    uint32_t grid_x;
    uint32_t grid_y;
    uint32_t grid_z;
    float cell_size;
    float inv_cell_size;
    float domain_min_x;
    float domain_min_y;
    float domain_min_z;
} SpatialHashParamsHost;

typedef struct SpatialCollisionParamsHost {
    uint32_t num_particles;
    uint32_t grid_x;
    uint32_t grid_y;
    uint32_t grid_z;
    float cell_size;
    float inv_cell_size;
    float domain_min_x;
    float domain_min_y;
    float domain_min_z;
    float dt;
    float particle_radius;
    float young_modulus;
    float thermal_conductivity;
    float specific_heat;
    float restitution;
} SpatialCollisionParamsHost;

typedef struct ParticleInteractionParamsHost {
    uint32_t num_particles;
    float dt;
    float particle_radius;
    float young_modulus;
    float thermal_conductivity;
    float specific_heat;
    float restitution;
} ParticleInteractionParamsHost;

typedef struct ModeProjectParamsHost {
    uint32_t num_modes;
    uint32_t num_particles;
    uint32_t anchors_per_mode;
    uint32_t grid_x;
    uint32_t grid_y;
    uint32_t grid_z;
    float cell_x;
    float cell_y;
    float cell_z;
    float inv_cell_x;
    float inv_cell_y;
    float inv_cell_z;
    float domain_x;
    float dt;
} ModeProjectParamsHost;

typedef struct PilotWaveParamsHost {
    uint32_t num_particles;
    uint32_t grid_x;
    uint32_t grid_y;
    uint32_t grid_z;
    float cell_x;
    float cell_y;
    float cell_z;
    float inv_cell_x;
    float inv_cell_y;
    float inv_cell_z;
    float dt;
    float domain_x;
    float domain_y;
    float domain_z;
    float hbar_eff;
    float eps_denom;
    float mass_min;
    uint32_t boundary_x_low;
    uint32_t boundary_x_high;
    uint32_t boundary_y_low;
    uint32_t boundary_y_high;
    uint32_t boundary_z_low;
    uint32_t boundary_z_high;
} PilotWaveParamsHost;

_Static_assert(sizeof(SortScatterParamsHost) == 68, "SortScatterParamsHost ABI mismatch");
_Static_assert(sizeof(PicGatherParamsHost) == 104, "PicGatherParamsHost ABI mismatch");
_Static_assert(sizeof(ModeProjectParamsHost) == 56, "ModeProjectParamsHost ABI mismatch");
_Static_assert(sizeof(PilotWaveParamsHost) == 92, "PilotWaveParamsHost ABI mismatch");
_Static_assert(offsetof(PilotWaveParamsHost, mass_min) == 64, "PilotWaveParamsHost field mismatch");

typedef struct ParticleGenParamsHost {
    uint32_t num_particles;
    float grid_x;
    float grid_y;
    float grid_z;
    float energy_scale;
    uint32_t pattern;
    float center_x;
    float center_y;
    float center_z;
    float spread;
    float dir_x;
    float dir_y;
    float dir_z;
} ParticleGenParamsHost;

typedef struct {
    uint32_t x;
    uint32_t y;
    uint32_t z;
    bool is_ghost;
} ManifoldGasNeighborCoord;

void manifold_write_error(char *err_out, int err_cap, NSString *message);
uint32_t manifold_cell_index(uint32_t x, uint32_t y, uint32_t z, uint32_t gx, uint32_t gy, uint32_t gz);
uint32_t manifold_gas_boundary_mode(
    const ManifoldConfig *config,
    uint32_t axis,
    bool high_face
);
ManifoldGasNeighborCoord manifold_gas_neighbor_coord(
    uint32_t x,
    uint32_t y,
    uint32_t z,
    uint32_t axis,
    bool high_face,
    uint32_t gx,
    uint32_t gy,
    uint32_t gz,
    uint32_t boundary
);
float manifold_gas_pressure_at(
    float *eData,
    float gamma,
    uint32_t x,
    uint32_t y,
    uint32_t z,
    uint32_t gx,
    uint32_t gy,
    uint32_t gz,
    bool is_ghost,
    uint32_t ghost_axis,
    uint32_t ghost_boundary,
    uint32_t cx,
    uint32_t cy,
    uint32_t cz
);
void manifold_gas_velocity_at(
    float *momRhoData,
    uint32_t x,
    uint32_t y,
    uint32_t z,
    uint32_t gx,
    uint32_t gy,
    uint32_t gz,
    bool is_ghost,
    uint32_t ghost_axis,
    uint32_t ghost_boundary,
    uint32_t cx,
    uint32_t cy,
    uint32_t cz,
    float *ux,
    float *uy,
    float *uz
);
float manifold_pressure_at(
    float *eData,
    float gamma,
    uint32_t x,
    uint32_t y,
    uint32_t z,
    uint32_t gx,
    uint32_t gy,
    uint32_t gz
);
void manifold_velocity_at(
    float *momRhoData,
    uint32_t x,
    uint32_t y,
    uint32_t z,
    uint32_t gx,
    uint32_t gy,
    uint32_t gz,
    float *ux,
    float *uy,
    float *uz
);

@interface ManifoldSolver : NSObject
@property(nonatomic, strong) id<MTLDevice> device;
@property(nonatomic, strong) id<MTLCommandQueue> queue;
@property(nonatomic, strong) id<MTLLibrary> library;
@property(nonatomic, strong) id<MTLComputePipelineState> clearField;
@property(nonatomic, strong) id<MTLComputePipelineState> clearBufferU32;
@property(nonatomic, strong) id<MTLComputePipelineState> pipelineCopyBufferU32;
@property(nonatomic, strong) id<MTLComputePipelineState> pipelineCopyBufferFloat;
@property(nonatomic, strong) id<MTLComputePipelineState> pipelineCopyBitsToFloat;
@property(nonatomic, strong) id<MTLComputePipelineState> scatterPrefixSeedLast;
@property(nonatomic, strong) id<MTLComputePipelineState> clearCarrierAccums;
@property(nonatomic, strong) id<MTLComputePipelineState> deriveMaxCarrierBin;
@property(nonatomic, strong) id<MTLComputePipelineState> initOmegaScanKeys;
@property(nonatomic, strong) id<MTLComputePipelineState> gasApplySources;
@property(nonatomic, strong) id<MTLComputePipelineState> gasComputePrimitives;
@property(nonatomic, strong) id<MTLComputePipelineState> gasStage1;
@property(nonatomic, strong) id<MTLComputePipelineState> gasStage2;
@property(nonatomic, strong) id<MTLComputePipelineState> reduceOmegaMinMax;
@property(nonatomic, strong) id<MTLComputePipelineState> computeBinParams;
@property(nonatomic, strong) id<MTLComputePipelineState> binCountCarriers;
@property(nonatomic, strong) id<MTLComputePipelineState> binScatterCarriers;
@property(nonatomic, strong) id<MTLComputePipelineState> scanPass1;
@property(nonatomic, strong) id<MTLComputePipelineState> scanAddBlockOffsets;
@property(nonatomic, strong) id<MTLComputePipelineState> scanFinalizeTotal;
@property(nonatomic, strong) id<MTLComputePipelineState> precomputeCarrierAnchorPositions;
@property(nonatomic, strong) id<MTLComputePipelineState> prepOscillatorCoupling;
@property(nonatomic, strong) id<MTLComputePipelineState> accumulateForces;
@property(nonatomic, strong) id<MTLComputePipelineState> gpeStep;
@property(nonatomic, strong) id<MTLComputePipelineState> updatePhases;
@property(nonatomic, strong) id<MTLComputePipelineState> scatterComputeCellIdx;
@property(nonatomic, strong) id<MTLComputePipelineState> scatterCountCells;
@property(nonatomic, strong) id<MTLComputePipelineState> scatterPrefixUpsweep;
@property(nonatomic, strong) id<MTLComputePipelineState> scatterPrefixDownsweep;
@property(nonatomic, strong) id<MTLComputePipelineState> scatterReorderParticles;
@property(nonatomic, strong) id<MTLComputePipelineState> scatterGatherCells;
@property(nonatomic, strong) id<MTLComputePipelineState> picGatherUpdate;
@property(nonatomic, strong) id<MTLComputePipelineState> picGatherPilotWave;
@property(nonatomic, strong) id<MTLComputePipelineState> projectModesToSpatialPsi;
@property(nonatomic, strong) id<MTLComputePipelineState> particleInteractions;
@property(nonatomic, strong) id<MTLComputePipelineState> spatialHashAssign;
@property(nonatomic, strong) id<MTLComputePipelineState> spatialHashScatter;
@property(nonatomic, strong) id<MTLComputePipelineState> spatialHashCollisions;
@property(nonatomic, strong) id<MTLComputePipelineState> reduceFloatStatsPass1;
@property(nonatomic, strong) id<MTLComputePipelineState> reduceFloatStatsFinalize;
@property(nonatomic, strong) id<MTLComputePipelineState> generateParticlePositions;
@property(nonatomic, strong) id<MTLComputePipelineState> initializeParticleProperties;
@property(nonatomic, strong) id<MTLComputePipelineState> coherenceFuseBinning;
@property(nonatomic, assign) BOOL gravityReady;
@property(nonatomic, assign) ManifoldConfig config;
@property(nonatomic, assign) ManifoldControls controls;
@property(nonatomic, assign) uint32_t numCells;
@property(nonatomic, assign) uint32_t numOsc;
@property(nonatomic, assign) uint32_t numBins;
@property(nonatomic, assign) uint32_t maxCarriersForTG;
@property(nonatomic, assign) uint32_t simdWidth;
@property(nonatomic, assign) NSUInteger maxThreadsPerThreadgroup;
@property(nonatomic, assign) uint64_t residentBytes;
@property(nonatomic, strong) ManifoldSolver *metalOwner;
@property(nonatomic, strong) id<MTLBuffer> momRho;
@property(nonatomic, strong) id<MTLBuffer> eInt;
@property(nonatomic, strong) id<MTLBuffer> momRhoStage;
@property(nonatomic, strong) id<MTLBuffer> eStage;
@property(nonatomic, strong) id<MTLBuffer> entropyStage;
@property(nonatomic, strong) id<MTLBuffer> particleCicA;
@property(nonatomic, strong) id<MTLBuffer> particleCicB;
@property(nonatomic, strong) id<MTLHeap> gpuHeap;
@property(nonatomic, strong) id<MTLCommandBuffer> stepCommandBuffer;
@property(nonatomic, strong) id<MTLComputeCommandEncoder> stepEncoder;
@property(nonatomic, assign) BOOL stepDispatchActive;
@property(nonatomic, strong) id<MTLBuffer> gasPrim;
@property(nonatomic, strong) id<MTLBuffer> gasParams;
@property(nonatomic, strong) id<MTLBuffer> sourceMomRho;
@property(nonatomic, strong) id<MTLBuffer> sourceE;
@property(nonatomic, strong) id<MTLBuffer> maxCarrierBinKey;
@property(nonatomic, strong) id<MTLBuffer> dbgCap;
@property(nonatomic, strong) id<MTLBuffer> dbgHead;
@property(nonatomic, strong) id<MTLBuffer> dbgWords;
@property(nonatomic, strong) id<MTLBuffer> oscPhase;
@property(nonatomic, strong) id<MTLBuffer> oscOmega;
@property(nonatomic, strong) id<MTLBuffer> oscAmp;
@property(nonatomic, strong) id<MTLBuffer> oscHeat;
@property(nonatomic, strong) id<MTLBuffer> particlePos;
@property(nonatomic, strong) id<MTLBuffer> particleVel;
@property(nonatomic, strong) id<MTLBuffer> particleMass;
@property(nonatomic, strong) id<MTLBuffer> particleEnergy;
@property(nonatomic, strong) id<MTLBuffer> particlePosSorted;
@property(nonatomic, strong) id<MTLBuffer> particleVelSorted;
@property(nonatomic, strong) id<MTLBuffer> particleMassSorted;
@property(nonatomic, strong) id<MTLBuffer> particleHeatSorted;
@property(nonatomic, strong) id<MTLBuffer> particleEnergySorted;
@property(nonatomic, strong) id<MTLBuffer> particleCellIdx;
@property(nonatomic, strong) id<MTLBuffer> scatterCellCounts;
@property(nonatomic, strong) id<MTLBuffer> scatterCellStarts;
@property(nonatomic, strong) id<MTLBuffer> scatterCellOffsets;
@property(nonatomic, strong) id<MTLBuffer> sortedOriginalIdx;
@property(nonatomic, strong) id<MTLBuffer> rhoAtomic;
@property(nonatomic, strong) id<MTLBuffer> momAtomic;
@property(nonatomic, strong) id<MTLBuffer> eAtomic;
@property(nonatomic, strong) id<MTLBuffer> gravityPotential;
@property(nonatomic, strong) id<MTLBuffer> sortScatterParams;
@property(nonatomic, strong) id<MTLBuffer> picGatherParams;
@property(nonatomic, strong) id<MTLBuffer> particleExcitation;
@property(nonatomic, strong) id<MTLBuffer> particleVelIn;
@property(nonatomic, strong) id<MTLBuffer> particleHeatIn;
@property(nonatomic, strong) id<MTLBuffer> hashCellCounts;
@property(nonatomic, strong) id<MTLBuffer> hashCellStarts;
@property(nonatomic, strong) id<MTLBuffer> hashCellOffsets;
@property(nonatomic, strong) id<MTLBuffer> hashSortedIdx;
@property(nonatomic, strong) id<MTLBuffer> hashParticleCellIdx;
@property(nonatomic, strong) id<MTLBuffer> hashNumCellsBuf;
@property(nonatomic, strong) id<MTLBuffer> hashNumParticlesBuf;
@property(nonatomic, strong) id<MTLBuffer> spatialHashParams;
@property(nonatomic, strong) id<MTLBuffer> spatialCollisionParams;
@property(nonatomic, strong) id<MTLBuffer> particleInteractionParams;
@property(nonatomic, strong) id<MTLBuffer> psiReField;
@property(nonatomic, strong) id<MTLBuffer> psiImField;
@property(nonatomic, strong) id<MTLBuffer> psiReAtomic;
@property(nonatomic, strong) id<MTLBuffer> psiImAtomic;
@property(nonatomic, strong) id<MTLBuffer> modeProjectParams;
@property(nonatomic, strong) id<MTLBuffer> pilotWaveParams;
@property(nonatomic, strong) id<MTLBuffer> particleGenParams;
@property(nonatomic, strong) id<MTLBuffer> particleRandomVals;
@property(nonatomic, strong) id<MTLBuffer> reduceGroupStats;
@property(nonatomic, strong) id<MTLBuffer> reduceStatsOut;
@property(nonatomic, strong) id<MTLBuffer> hashBlockSums;
@property(nonatomic, strong) id<MTLBuffer> modeReal;
@property(nonatomic, strong) id<MTLBuffer> modeImag;
@property(nonatomic, strong) id<MTLBuffer> modeOmega;
@property(nonatomic, strong) id<MTLBuffer> modeGate;
@property(nonatomic, strong) id<MTLBuffer> modeAnchorIdx;
@property(nonatomic, strong) id<MTLBuffer> modeAnchorWeight;
@property(nonatomic, strong) id<MTLBuffer> modeAnchorPos;
@property(nonatomic, strong) id<MTLBuffer> oscCouplingPrep;
@property(nonatomic, strong) id<MTLBuffer> accums;
@property(nonatomic, strong) id<MTLBuffer> numCarriers;
@property(nonatomic, strong) id<MTLBuffer> omegaMinKey;
@property(nonatomic, strong) id<MTLBuffer> omegaMaxKey;
@property(nonatomic, strong) id<MTLBuffer> binCounts;
@property(nonatomic, strong) id<MTLBuffer> binStarts;
@property(nonatomic, strong) id<MTLBuffer> binOffsets;
@property(nonatomic, strong) id<MTLBuffer> carrierBinnedIdx;
@property(nonatomic, strong) id<MTLBuffer> binParams;
@property(nonatomic, strong) id<MTLBuffer> numBinsBuf;
@property(nonatomic, strong) id<MTLBuffer> gateWidthMaxBuf;
@property(nonatomic, strong) id<MTLBuffer> scanBlockSums;
@property(nonatomic, strong) id<MTLBuffer> scanBlockPrefix;
@property(nonatomic, strong) id<MTLBuffer> scanBlockScratch;
@property(nonatomic, strong) id<MTLBuffer> coherenceParams;
@property(nonatomic, strong) id<MTLBuffer> gpeParams;
- (instancetype)initWithConfig:(const ManifoldConfig *)config
                 metallibBytes:(const void *)metallibBytes
                metallibLength:(size_t)metallibLength
                         error:(NSString **)error;
- (BOOL)setControls:(const ManifoldControls *)controls error:(NSString **)error;
- (void)resetDepositsInternal;
- (void)resetSourcesInternal;
- (BOOL)depositCell:(uint32_t)cellX cellY:(uint32_t)cellY cellZ:(uint32_t)cellZ
                rho:(float)rho momX:(float)momX momY:(float)momY momZ:(float)momZ eInt:(float)eInt error:(NSString **)error;
- (BOOL)sourceCell:(uint32_t)cellX cellY:(uint32_t)cellY cellZ:(uint32_t)cellZ
            deltaMomX:(float)deltaMomX deltaMomY:(float)deltaMomY deltaMomZ:(float)deltaMomZ
            deltaRho:(float)deltaRho deltaE:(float)deltaE error:(NSString **)error;
- (BOOL)applySources:(NSString **)error;
- (BOOL)readCell:(uint32_t)cellX cellY:(uint32_t)cellY cellZ:(uint32_t)cellZ
             rho:(float *)rho momX:(float *)momX momY:(float *)momY momZ:(float *)momZ
            eInt:(float *)eInt error:(NSString **)error;
- (BOOL)conservedStateIsFinite:(NSString **)error;
- (void)configureGasParams;
- (BOOL)setOscillators:(const ManifoldOscillator *)oscillators count:(uint32_t)count error:(NSString **)error;
- (BOOL)runGasStep:(NSString **)error;
- (BOOL)runGasTransport:(NSString **)error;
- (BOOL)computeReading:(ManifoldReading *)reading error:(NSString **)error;
- (BOOL)readRhoMaxProjection:(float *)out length:(uint32_t)length error:(NSString **)error;
- (BOOL)computeProjectionReading:(ManifoldReading *)reading error:(NSString **)error;
- (BOOL)readOscillators:(ManifoldOscillator *)out count:(uint32_t)count error:(NSString **)error;
- (BOOL)step:(ManifoldReading *)reading error:(NSString **)error;
@end

@interface ManifoldSolver (DispatchPrivate)
- (void)beginStepDispatches;
- (void)flushStepDispatches;
- (void)endStepDispatches;
- (void)dispatchGasBrickSynchronized:(id<MTLComputePipelineState>)pipeline
                             buffers:(NSArray<id<MTLBuffer>> *)buffers;
- (id<MTLBuffer>)newSharedBufferWithLength:(size_t)length;
- (id<MTLBuffer>)newGPUBufferWithLength:(size_t)length;
- (void)dispatchGridKernel:(id<MTLComputePipelineState>)pipeline
                   buffers:(NSArray<id<MTLBuffer>> *)buffers
               threadCount:(NSUInteger)threadCount;
- (void)dispatchGridKernelSynchronized:(id<MTLComputePipelineState>)pipeline
                               buffers:(NSArray<id<MTLBuffer>> *)buffers
                           threadCount:(NSUInteger)threadCount;
- (void)dispatchThreadgroupKernelSynchronized:(id<MTLComputePipelineState>)pipeline
                                      buffers:(NSArray<id<MTLBuffer>> *)buffers
                                threadgroupSize:(NSUInteger)threadgroupSize
                               threadgroupCount:(NSUInteger)threadgroupCount
                       threadgroupMemoryLength:(NSUInteger)threadgroupMemoryLength;
- (void)dispatchGasBrickKernel:(id<MTLComputePipelineState>)pipeline
                       buffers:(NSArray<id<MTLBuffer>> *)buffers;
- (void)dispatchThreadgroupKernel:(id<MTLComputePipelineState>)pipeline
                          buffers:(NSArray<id<MTLBuffer>> *)buffers
                    threadgroupSize:(NSUInteger)threadgroupSize
                    threadgroupCount:(NSUInteger)threadgroupCount
            threadgroupMemoryLength:(NSUInteger)threadgroupMemoryLength;
- (void)dispatchThreadgroupKernel:(id<MTLComputePipelineState>)pipeline
                          buffers:(NSArray<id<MTLBuffer>> *)buffers
                    threadgroupSize:(NSUInteger)threadgroupSize
                    threadgroupCount:(NSUInteger)threadgroupCount
           threadgroupMemoryLengths:(NSArray<NSNumber *> *)threadgroupMemoryLengths;
@end

@interface ManifoldSolver (CoherencePrivate)
- (id<MTLComputePipelineState>)pipelineNamed:(NSString *)name error:(NSError **)error;
- (BOOL)runCoherenceStep:(NSString **)error;
@end

@interface ManifoldSolver (GravityPrivate)
- (BOOL)runGravityPoisson:(NSString **)error;
@end

@interface ManifoldSolver (GasPrivate)
- (BOOL)gasSubsteps:(uint32_t *)substeps error:(NSString **)error;
- (BOOL)gasWaveRate:(float *)rate error:(NSString **)error;
- (BOOL)gasDiffusionRate:(float *)rate error:(NSString **)error;
- (void)runGasSubstep;
@end

@interface ManifoldSolver (ParticlesPrivate)
- (void)configureSpatialHashParams;
- (void)configureSpatialCollisionParams;
- (void)configureParticleInteractionParams;
- (BOOL)runDirectParticleInteractions:(NSString **)error;
- (BOOL)runSpatialHashCollisions:(NSString **)error;
- (BOOL)runParticleCollisions:(NSString **)error;
- (void)configureModeProjectParams;
- (void)configurePilotWaveParams;
- (void)clearPsiFields;
- (void)copyPsiAtomicsToFields;
- (BOOL)runProjectModesToSpatialPsi:(NSString **)error;
- (BOOL)runPilotWaveGather:(NSString **)error;
@end

@interface ManifoldSolver (UtilPrivate)
- (void)runClearField:(id<MTLBuffer>)field count:(uint32_t)count;
- (void)runClearU32:(id<MTLBuffer>)buffer count:(uint32_t)count;
- (void)runCopyU32:(id<MTLBuffer>)src dst:(id<MTLBuffer>)dst count:(uint32_t)count;
- (void)runCopyFloat:(id<MTLBuffer>)src dst:(id<MTLBuffer>)dst count:(uint32_t)count;
- (void)runCopyBitsToFloat:(id<MTLBuffer>)srcBits dst:(id<MTLBuffer>)dst count:(uint32_t)count;
- (void)runScatterPrefixSeedLast:(id<MTLBuffer>)data count:(uint32_t)count;
- (void)runClearCarrierAccums:(uint32_t)numCarriers;
- (uint32_t)runDeriveMaxCarrierBin;
- (void)runReduceFloatStats:(id<MTLBuffer>)values length:(uint32_t)length statsOut:(float *)statsOut;
- (void)configureParticleGenParams;
- (void)seedRandomValuesFromOscillators:(const ManifoldOscillator *)oscillators count:(uint32_t)count;
- (void)runInitializeParticleProperties:(const ManifoldOscillator *)oscillators count:(uint32_t)count;
- (BOOL)runExclusiveScanU32:(id<MTLBuffer>)input
                      output:(id<MTLBuffer>)output
                      length:(uint32_t)length
              writeTotalSlot:(BOOL)writeTotalSlot
                       error:(NSString **)error;
@end

@interface ManifoldSolver (PicPrivate)
- (float)gridSpacing;
- (void)initializeParticleStateFromOscillators:(const ManifoldOscillator *)oscillators count:(uint32_t)count;
- (void)configureSortScatterParams;
- (void)configurePicGatherParams;
- (BOOL)runScatterPrefixSum:(NSString **)error;
- (BOOL)runPicScatter:(NSString **)error;
- (BOOL)runPicGather:(NSString **)error;
@end
