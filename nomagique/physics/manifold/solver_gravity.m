#import "solver_private.h"
#import <Accelerate/Accelerate.h>

static float manifold_wavenumber(int index, int count, float length) {
    int wrapped = (index <= count / 2) ? index : (index - count);

    return 2.0f * (float)M_PI * (float)wrapped / length;
}

static void manifold_naive_dft_line(
    size_t length,
    vDSP_DFT_Direction direction,
    float *realIn,
    float *imagIn,
    float *realOut,
    float *imagOut
) {
    float sign = (direction == vDSP_DFT_FORWARD) ? -1.0f : 1.0f;
    float invLength = 1.0f / (float)length;

    for (size_t kIndex = 0; kIndex < length; kIndex++) {
        float sumRe = 0.0f;
        float sumIm = 0.0f;

        for (size_t nIndex = 0; nIndex < length; nIndex++) {
            float angle = sign * 2.0f * (float)M_PI * (float)(kIndex * nIndex) * invLength;
            float cosine = cosf(angle);
            float sine = sinf(angle);

            sumRe += realIn[nIndex] * cosine - imagIn[nIndex] * sine;
            sumIm += realIn[nIndex] * sine + imagIn[nIndex] * cosine;
        }

        realOut[kIndex] = sumRe;
        imagOut[kIndex] = sumIm;
    }
}

static void manifold_fft_line(
    vDSP_DFT_Setup setup,
    float *realIn,
    float *imagIn,
    float *realOut,
    float *imagOut,
    size_t length,
    vDSP_DFT_Direction direction
) {
    if (setup != NULL) {
        vDSP_DFT_Execute(setup, realIn, imagIn, realOut, imagOut);
    }

    if (setup == NULL) {
        manifold_naive_dft_line(length, direction, realIn, imagIn, realOut, imagOut);
    }

    memcpy(realIn, realOut, length * sizeof(float));
    memcpy(imagIn, imagOut, length * sizeof(float));
}

@implementation ManifoldSolver (GravityPrivate)

- (BOOL)runGravityPoisson:(NSString **)error {
    (void)error;
    uint32_t gx = self.config.grid_x;
    uint32_t gy = self.config.grid_y;
    uint32_t gz = self.config.grid_z;
    size_t numCells = (size_t)self.numCells;
    float *momRhoData = (float *)self.momRho.contents;
    float *phiData = (float *)self.gravityPotential.contents;
    float fourPiG = 4.0f * (float)M_PI * self.config.g_interaction;

    float *real = (float *)malloc(numCells * sizeof(float));
    float *imag = (float *)calloc(numCells, sizeof(float));
    float *realScratch = (float *)malloc(numCells * sizeof(float));
    float *imagScratch = (float *)malloc(numCells * sizeof(float));
    size_t lineCapacity = (size_t)gy > (size_t)gx ? (size_t)gy : (size_t)gx;
    float *lineRe = (float *)malloc(lineCapacity * sizeof(float));
    float *lineIm = (float *)calloc(lineCapacity, sizeof(float));
    float *outRe = (float *)malloc(lineCapacity * sizeof(float));
    float *outIm = (float *)calloc(lineCapacity, sizeof(float));

    if (real == NULL || imag == NULL || realScratch == NULL || imagScratch == NULL ||
        lineRe == NULL || lineIm == NULL || outRe == NULL || outIm == NULL) {
        free(real);
        free(imag);
        free(realScratch);
        free(imagScratch);
        free(lineRe);
        free(lineIm);
        free(outRe);
        free(outIm);

        if (error != nil) {
            *error = @"gravity FFT allocation failed";
        }

        return NO;
    }

    for (size_t i = 0; i < numCells; i++) {
        real[i] = momRhoData[i * 4 + 3];
    }

    vDSP_DFT_Setup setupZ = vDSP_DFT_zop_CreateSetup(NULL, gz, vDSP_DFT_FORWARD);
    vDSP_DFT_Setup setupY = vDSP_DFT_zop_CreateSetup(NULL, gy, vDSP_DFT_FORWARD);
    vDSP_DFT_Setup setupX = vDSP_DFT_zop_CreateSetup(NULL, gx, vDSP_DFT_FORWARD);

    for (uint32_t ix = 0; ix < gx; ix++) {
        for (uint32_t iy = 0; iy < gy; iy++) {
            size_t base = (size_t)ix * (size_t)gy * (size_t)gz + (size_t)iy * (size_t)gz;
            manifold_fft_line(setupZ, real + base, imag + base, realScratch + base, imagScratch + base, gz, vDSP_DFT_FORWARD);
        }
    }

    for (uint32_t ix = 0; ix < gx; ix++) {
        for (uint32_t iz = 0; iz < gz; iz++) {
            for (uint32_t iy = 0; iy < gy; iy++) {
                size_t index = (size_t)ix * (size_t)gy * (size_t)gz + (size_t)iy * (size_t)gz + (size_t)iz;
                lineRe[iy] = real[index];
                lineIm[iy] = imag[index];
            }

            manifold_fft_line(setupY, lineRe, lineIm, outRe, outIm, gy, vDSP_DFT_FORWARD);

            for (uint32_t iy = 0; iy < gy; iy++) {
                size_t index = (size_t)ix * (size_t)gy * (size_t)gz + (size_t)iy * (size_t)gz + (size_t)iz;
                real[index] = outRe[iy];
                imag[index] = outIm[iy];
            }
        }
    }

    for (uint32_t iy = 0; iy < gy; iy++) {
        for (uint32_t iz = 0; iz < gz; iz++) {
            for (uint32_t ix = 0; ix < gx; ix++) {
                size_t index = (size_t)ix * (size_t)gy * (size_t)gz + (size_t)iy * (size_t)gz + (size_t)iz;
                lineRe[ix] = real[index];
                lineIm[ix] = imag[index];
            }

            manifold_fft_line(setupX, lineRe, lineIm, outRe, outIm, gx, vDSP_DFT_FORWARD);

            for (uint32_t ix = 0; ix < gx; ix++) {
                size_t index = (size_t)ix * (size_t)gy * (size_t)gz + (size_t)iy * (size_t)gz + (size_t)iz;
                real[index] = outRe[ix];
                imag[index] = outIm[ix];
            }
        }
    }

    for (uint32_t ix = 0; ix < gx; ix++) {
        float kx = manifold_wavenumber((int)ix, (int)gx, self.config.domain_x);

        for (uint32_t iy = 0; iy < gy; iy++) {
            float ky = manifold_wavenumber((int)iy, (int)gy, self.config.domain_y);

            for (uint32_t iz = 0; iz < gz; iz++) {
                float kz = manifold_wavenumber((int)iz, (int)gz, self.config.domain_z);
                float k2 = kx * kx + ky * ky + kz * kz;
                size_t index = (size_t)ix * (size_t)gy * (size_t)gz + (size_t)iy * (size_t)gz + (size_t)iz;

                if (k2 > 0.0f) {
                    float scale = -fourPiG / k2;
                    real[index] *= scale;
                    imag[index] *= scale;
                }

                if (k2 == 0.0f) {
                    real[index] = 0.0f;
                    imag[index] = 0.0f;
                }
            }
        }
    }

    vDSP_DFT_Setup invSetupX = vDSP_DFT_zop_CreateSetup(NULL, gx, vDSP_DFT_INVERSE);
    vDSP_DFT_Setup invSetupY = vDSP_DFT_zop_CreateSetup(NULL, gy, vDSP_DFT_INVERSE);
    vDSP_DFT_Setup invSetupZ = vDSP_DFT_zop_CreateSetup(NULL, gz, vDSP_DFT_INVERSE);

    for (uint32_t ix = 0; ix < gx; ix++) {
        for (uint32_t iy = 0; iy < gy; iy++) {
            size_t base = (size_t)ix * (size_t)gy * (size_t)gz + (size_t)iy * (size_t)gz;
            manifold_fft_line(invSetupZ, real + base, imag + base, realScratch + base, imagScratch + base, gz, vDSP_DFT_INVERSE);
        }
    }

    for (uint32_t ix = 0; ix < gx; ix++) {
        for (uint32_t iz = 0; iz < gz; iz++) {
            for (uint32_t iy = 0; iy < gy; iy++) {
                size_t index = (size_t)ix * (size_t)gy * (size_t)gz + (size_t)iy * (size_t)gz + (size_t)iz;
                lineRe[iy] = real[index];
                lineIm[iy] = imag[index];
            }

            manifold_fft_line(invSetupY, lineRe, lineIm, outRe, outIm, gy, vDSP_DFT_INVERSE);

            for (uint32_t iy = 0; iy < gy; iy++) {
                size_t index = (size_t)ix * (size_t)gy * (size_t)gz + (size_t)iy * (size_t)gz + (size_t)iz;
                real[index] = outRe[iy];
                imag[index] = outIm[iy];
            }
        }
    }

    for (uint32_t iy = 0; iy < gy; iy++) {
        for (uint32_t iz = 0; iz < gz; iz++) {
            for (uint32_t ix = 0; ix < gx; ix++) {
                size_t index = (size_t)ix * (size_t)gy * (size_t)gz + (size_t)iy * (size_t)gz + (size_t)iz;
                lineRe[ix] = real[index];
                lineIm[ix] = imag[index];
            }

            manifold_fft_line(invSetupX, lineRe, lineIm, outRe, outIm, gx, vDSP_DFT_INVERSE);

            for (uint32_t ix = 0; ix < gx; ix++) {
                size_t index = (size_t)ix * (size_t)gy * (size_t)gz + (size_t)iy * (size_t)gz + (size_t)iz;
                real[index] = outRe[ix];
                imag[index] = outIm[ix];
            }
        }
    }

    float invNorm = 1.0f / (float)numCells;

    for (size_t index = 0; index < numCells; index++) {
        phiData[index] = real[index] * invNorm;
    }

    self.gravityReady = YES;

    if (setupZ != NULL) {
        vDSP_DFT_DestroySetup(setupZ);
    }

    if (setupY != NULL) {
        vDSP_DFT_DestroySetup(setupY);
    }

    if (setupX != NULL) {
        vDSP_DFT_DestroySetup(setupX);
    }

    if (invSetupX != NULL) {
        vDSP_DFT_DestroySetup(invSetupX);
    }

    if (invSetupY != NULL) {
        vDSP_DFT_DestroySetup(invSetupY);
    }

    if (invSetupZ != NULL) {
        vDSP_DFT_DestroySetup(invSetupZ);
    }
    free(real);
    free(imag);
    free(realScratch);
    free(imagScratch);
    free(lineRe);
    free(lineIm);
    free(outRe);
    free(outIm);

    return YES;
}

@end
