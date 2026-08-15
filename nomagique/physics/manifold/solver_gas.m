#import "solver_private.h"

@implementation ManifoldSolver (GasPrivate)

/*
gasSubsteps derives one conservative timestep count from the materialized gas
state. It combines the rarefaction-head rate with the bounded variable-
conductivity rate, so the RK stages consume the complete requested interval.
*/
- (BOOL)gasSubsteps:(uint32_t *)substeps error:(NSString **)error {
    if (substeps == NULL) {
        if (error != nil) {
            *error = @"gas substep output is required";
        }

        return NO;
    }

    float waveRate = 0.0f;

    if (![self gasWaveRate:&waveRate error:error]) {
        return NO;
    }

    float diffusionRate = 0.0f;

    if (![self gasDiffusionRate:&diffusionRate error:error]) {
        return NO;
    }

    double combinedRate = (double)waveRate + (double)diffusionRate;

    if (!(combinedRate > 0.0)) {
        *substeps = 1;
        return YES;
    }

    // Size sub-steps against a target Courant number safely below 1 rather than
    // exactly 1. gasWaveRate samples the field BEFORE transport, but forcing and
    // advection push cells toward near-vacuum during the step, raising the true
    // per-substep wave speed above the sampled rate. Running the sub-step at
    // CFL=1 then overshoots (observed courant ~1.16) and the gas kernel NaNs the
    // cell (tag 0x21). The safety factor gives the sub-step count margin for that
    // intra-step speed growth. 0.8 covers the observed overshoot with headroom.
    const double kGasCFLSafety = 0.8;
    double stableDelta = nextafter(kGasCFLSafety / combinedRate, 0.0);
    double required = ceil((double)self.controls.dt / stableDelta);

    if (!isfinite(required) || required > (double)UINT32_MAX) {
        if (error != nil) {
            *error = @"gas stability subdivision exceeds uint32 capacity";
        }

        return NO;
    }

    *substeps = (uint32_t)fmax(required, 1.0);
    return YES;
}

- (BOOL)gasWaveRate:(float *)rate error:(NSString **)error {
    float *momRho = (float *)self.momRho.contents;
    float *internalEnergy = (float *)self.eInt.contents;
    float maxWaveX = 0.0f;
    float maxWaveY = 0.0f;
    float maxWaveZ = 0.0f;

    for (uint32_t index = 0; index < self.numCells; index++) {
        uint32_t base = index * 4;
        float rho = momRho[base + 3];
        float energy = internalEnergy[index];

        if (rho == 0.0f && energy == 0.0f &&
            momRho[base] == 0.0f && momRho[base + 1] == 0.0f && momRho[base + 2] == 0.0f) {
            continue;
        }

        if (!(rho > 0.0f) || !(energy >= 0.0f)) {
            if (error != nil) {
                *error = [NSString stringWithFormat:@"gas state %u is inadmissible before transport", index];
            }

            return NO;
        }

        float sound = sqrtf(self.config.gamma * (self.config.gamma - 1.0f) * energy / rho);
        float rarefaction = 2.0f * sound / (self.config.gamma - 1.0f);
        maxWaveX = fmaxf(maxWaveX, fabsf(momRho[base] / rho) + rarefaction);
        maxWaveY = fmaxf(maxWaveY, fabsf(momRho[base + 1] / rho) + rarefaction);
        maxWaveZ = fmaxf(maxWaveZ, fabsf(momRho[base + 2] / rho) + rarefaction);
    }

    *rate = maxWaveX * (float)self.config.grid_x / self.config.domain_x +
        maxWaveY * (float)self.config.grid_y / self.config.domain_y +
        maxWaveZ * (float)self.config.grid_z / self.config.domain_z;
    return isfinite(*rate);
}

- (BOOL)gasDiffusionRate:(float *)rate error:(NSString **)error {
    float rhoResolution = self.config.gas_envelope_rho_min;

    if (!(rhoResolution > 0.0f) || !(self.config.c_v > 0.0f)) {
        if (error != nil) {
            *error = @"gas diffusion requires positive density resolution and heat capacity";
        }

        return NO;
    }

    float inverseX = (float)self.config.grid_x / self.config.domain_x;
    float inverseY = (float)self.config.grid_y / self.config.domain_y;
    float inverseZ = (float)self.config.grid_z / self.config.domain_z;
    float inverseSquareSum = inverseX * inverseX + inverseY * inverseY + inverseZ * inverseZ;

    // Each cell has two faces per axis. Harmonic face conductivity is at most
    // twice the sub-resolution cell conductivity, giving this exact upper bound.
    *rate = 4.0f * self.config.k_thermal * inverseSquareSum /
        (rhoResolution * self.config.c_v);
    return isfinite(*rate) && *rate >= 0.0f;
}

/*
runGasSubstep advances one already-sized SSP-RK2 interval.
*/
- (void)runGasSubstep {
    [self configureGasParams];

    [self dispatchGasBrickKernel:self.gasComputePrimitives
                         buffers:@[
                             self.momRho, self.eInt,
                             self.gasPrim, self.gasParams
                         ]];

    [self dispatchGasBrickKernel:self.gasStage1
                         buffers:@[
                             self.momRho, self.eInt,
                             self.gasPrim,
                             self.momRhoStage, self.eStage, self.entropyStage,
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
                             self.gasPrim, self.entropyStage,
                             self.momRho, self.eInt,
                             self.gasParams, self.dbgHead, self.dbgWords, self.dbgCap
                         ]];
}

@end
