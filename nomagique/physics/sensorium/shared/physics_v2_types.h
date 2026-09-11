#ifndef MANIFOLD_PHYSICS_V2_TYPES_H
#define MANIFOLD_PHYSICS_V2_TYPES_H

/* Deliberately scalar PODs: no host/device float3 alignment dependency.
   All energies in MFHydroStateV2 are densities, not specific energies. */
#define MF_PHYSICS_ABI_VERSION 2u
#define MF_RHO 0
#define MF_MX 1
#define MF_MY 2
#define MF_MZ 3
#define MF_ETOT 4
#define MF_EAUX 5

typedef struct { float q[6]; } MFHydroStateV2;
typedef struct {
    unsigned n, nx, ny, nz;
    float dx, dt, gamma, cv;
    float mu, bulk_viscosity, k_thermal;
    float eta_pressure, eta_sync, cfl;
    unsigned reconstruction; /* 0=piecewise constant; 1=MC MUSCL */
    unsigned gravity;        /* supplied acceleration, not a Poisson solver */
} MFHydroParamsV2;

typedef struct {
    float rate;             /* combined advection/diffusion rate [1/time] */
    float kinetic_density;
    float thermal_disagreement; /* signed (E-K)-e_aux */
    float speed;
    float vorticity;
    float gradient_frobenius;
    float thermal_fraction;
    float used_auxiliary;   /* 0 or 1 */
} MFHydroDiagnosticV2;

typedef struct { float re, im; } MFWaveValueV2;
typedef struct {
    unsigned n, nx, ny, nz;
    float dx, dt, hbar, mass, g;
} MFWaveParamsV2;
typedef struct {
    float number_density, energy_density;
    float jx, jy, jz; /* oriented positive-face probability/number current */
} MFWaveDiagnosticV2;

typedef struct { float x[3], v[3], mass, heat; } MFParticleStateV2;
typedef struct {
    unsigned n;
    float dt, radius, young_modulus, poisson_ratio;
    float normal_relaxation_time; /* tau>=0 in F_d=-tau*(dF_H/delta)*v_n */
    float conductivity, cv;
    float domain_x, domain_y, domain_z; /* all >0 or all zero */
} MFContactParamsV2;
typedef struct {
    float dvx, dvy, dvz;
    float heat_increment;
    float elastic_energy; /* half of sum of incident pair energies */
    float thermal_rate;
    float contact_rate;
} MFContactResultV2;

enum MFPhysicsStatusV2 {
    MF_PHYSICS_OK = 0,
    MF_PHYSICS_BAD_PARAMETERS = 1,
    MF_PHYSICS_BAD_STATE = 2,
    MF_PHYSICS_CFL = 3,
    MF_PHYSICS_NEGATIVE_UPDATE = 4,
    MF_PHYSICS_VACUUM_TRANSPORT = 5,
    MF_PHYSICS_UNRESOLVED_NODE = 6,
    MF_PHYSICS_COINCIDENT_CENTERS = 7
};

#if defined(__cplusplus) && !defined(__METAL_VERSION__)
static_assert(sizeof(MFHydroStateV2) == 24, "hydro state ABI");
static_assert(sizeof(MFHydroParamsV2) == 64, "hydro parameter ABI");
static_assert(sizeof(MFHydroDiagnosticV2) == 32, "hydro diagnostic ABI");
static_assert(sizeof(MFWaveValueV2) == 8, "wave ABI");
static_assert(sizeof(MFWaveParamsV2) == 36, "wave parameter ABI");
static_assert(sizeof(MFParticleStateV2) == 32, "particle ABI");
static_assert(sizeof(MFContactParamsV2) == 44, "contact parameter ABI");
#endif
#endif
