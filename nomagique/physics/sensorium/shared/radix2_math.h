#ifndef MANIFOLD_RADIX2_MATH_H
#define MANIFOLD_RADIX2_MATH_H
/* Scalar FFT mathematics shared by CUDA, Metal, and the executable CPU tests.
   No DFT fallback and no padding of the physical lattice. */
#if defined(__METAL_VERSION__)
#define MR_FN inline
#define MR_SIN metal::sin
#define MR_COS metal::cos
#define MR_EXP metal::exp
#define MR_FINITE metal::isfinite
#elif defined(__CUDACC__)
#include <limits>
#define MR_FN __host__ __device__ inline
#define MR_SIN sinf
#define MR_COS cosf
#define MR_EXP expf
#define MR_FINITE isfinite
#else
#include <cmath>
#include <limits>
#define MR_FN inline
#define MR_SIN std::sin
#define MR_COS std::cos
#define MR_EXP std::exp
#define MR_FINITE std::isfinite
#endif
struct MRComplex { float r, i; };
MR_FN MRComplex mr_add(MRComplex a, MRComplex b) { return {a.r+b.r,a.i+b.i}; }
MR_FN MRComplex mr_sub(MRComplex a, MRComplex b) { return {a.r-b.r,a.i-b.i}; }
MR_FN MRComplex mr_mul(MRComplex a, MRComplex b) { return {a.r*b.r-a.i*b.i,a.r*b.i+a.i*b.r}; }
MR_FN MRComplex mr_scale(MRComplex a, float s) { return {a.r*s,a.i*s}; }
MR_FN float mr_norm2(MRComplex a) { return a.r*a.r+a.i*a.i; }
MR_FN MRComplex mr_cis(float x) { return {MR_COS(x),MR_SIN(x)}; }
MR_FN bool mr_power2(unsigned n) { return n && !(n & (n-1u)); }
MR_FN unsigned mr_next_power2(unsigned n) {
    if (!n || n>0x40000000u) return 0u;
    --n; n|=n>>1; n|=n>>2; n|=n>>4; n|=n>>8; n|=n>>16;
    return n+1u;
}
MR_FN unsigned mr_workspace(unsigned n) {
    if (!n) return 0;
    if (mr_power2(n)) return n;
    // Bound all address arithmetic, including n + 2*L, without overflow.
    if (n>0x10000000u) return 0;
    unsigned l=mr_next_power2(2*n-1);
    return n+2*l;
}
MR_FN unsigned mr_workspace_bound(unsigned capacity) {
    // Worst case across every active count <= capacity, not just capacity itself.
    if (!capacity || capacity>0x10000000u) return 0;
    return capacity+2*mr_next_power2(2*capacity-1);
}
MR_FN unsigned mr_bit_reverse(unsigned x, unsigned n) {
    unsigned r=0;
    for (unsigned mask=n>>1;mask;mask>>=1) { r=(r<<1)|(x&1u);x>>=1; }
    return r;
}
MR_FN MRComplex mr_chirp(unsigned i,unsigned n,int sign) {
    // n is resource-bounded by the host. 64-bit integer reduction avoids loss
    // of all useful phase bits from first converting i*i to float.
#if defined(__METAL_VERSION__)
    unsigned residue=unsigned((ulong(i)*ulong(i)) % (2ul*ulong(n)));
#else
    unsigned residue=unsigned((static_cast<unsigned long long>(i)*i) % (2ull*n));
#endif
    return mr_cis(float(sign)*3.14159265358979323846f*(float(residue)/float(n)));
}
MR_FN MRComplex mr_local_half(MRComplex z,float ws,float g,float mu,float dt,float hbar) {
    float h=-ws+g*mr_norm2(z)-mu;
    return mr_mul(z,mr_cis(-h*(0.5f*dt)/hbar));
}
MR_FN float mr_kinetic_phase(unsigned k,unsigned n,float inv_domega2,float hbar,float mass,float dt) {
    float v=MR_SIN(3.14159265358979323846f*(float(k)/float(n)));
    // Exact eigenvalue of the original periodic SECOND-DIFFERENCE Laplacian.
    return (hbar/(2.0f*mass))*(-4.0f*v*v*inv_domega2)*dt;
}
MR_FN float mr_nan() {
#if defined(__METAL_VERSION__)
    return as_type<float>(0x7fc00000u);
#elif defined(__CUDA_ARCH__)
    return __uint_as_float(0x7fc00000u);
#else
    return std::numeric_limits<float>::quiet_NaN();
#endif
}
MR_FN MRComplex mr_open_step(MRComplex z,float fr,float fi,float was,float floor,float decay,float dt) {
    if (!MR_FINITE(fr) || !MR_FINITE(fi) || !MR_FINITE(was) || was<0 ||
        !MR_FINITE(floor) || floor<0 || !MR_FINITE(dt) ||
        (dt<0 && (decay>0 || fr!=0 || fi!=0)) ||
        !MR_FINITE(decay) || decay<0) {
        // Invalid input remains invalid, including when the drive gate is closed.
        float invalid=mr_nan();
        return {invalid,invalid};
    }
    MRComplex drive={0,0};
    if (was>floor) drive={fr/was,fi/was};
    if (decay>0) z=mr_scale(z,MR_EXP(-decay*dt));
    return mr_add(z,mr_scale(drive,dt));
}
#endif
