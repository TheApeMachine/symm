#ifndef MANIFOLD_SPECTRAL_GEOMETRY_H
#define MANIFOLD_SPECTRAL_GEOMETRY_H
#include "radix2_math.h"
// Canonical coefficients z_i=sqrt(w_i)*psi_i, w_i=sqrt(g_i)>0.
// H_edge/domega = hbar^2/(2*m*domega^2*w_face) *
//                 |z_j/sqrt(w_j)-z_i/sqrt(w_i)|^2.
// The two-site matrix is real Hermitian. Its exact exponential preserves
// canonical norm without a post-step normalization or a diagonal mass lump.
#if defined(__METAL_VERSION__)
#define MG_SQRT metal::sqrt
#elif defined(__CUDACC__)
#define MG_SQRT sqrtf
#else
#define MG_SQRT std::sqrt
#endif
struct MGPair { MRComplex left,right; };
MR_FN MGPair mg_bond(MRComplex x,MRComplex y,float wi,float wj,
                    float hbar,float mass,float inv_dx2,float dt) {
    if(!(wi>0&&wj>0&&hbar>0&&mass>0)||!MR_FINITE(wi)||!MR_FINITE(wj)) {
        MRComplex bad={mr_nan(),mr_nan()};return {bad,bad};
    }
    float face=0.5f*wi+0.5f*wj;
    float c=(hbar/(2*mass))*inv_dx2/face; // H/hbar, avoids hbar^2 overflow
    float a=c/wi,d=c/wj,b=-c/MG_SQRT(wi)/MG_SQRT(wj);
    float tr=.5f*a+.5f*d,delta=.5f*a-.5f*d;
    // For this rank-one edge q=trace >=0; no cancellation-prone eigen solve.
    float angle=tr*dt,cs=MR_COS(angle),sn=MR_SIN(angle);
    float sinc=tr>0?sn/tr:dt;
    MRComplex mx=mr_add(mr_scale(x,delta),mr_scale(y,b));
    MRComplex my=mr_sub(mr_scale(x,b),mr_scale(y,delta));
    MRComplex phase=mr_cis(-angle);
    MRComplex u={cs*x.r+sinc*mx.i,cs*x.i-sinc*mx.r};
    MRComplex v={cs*y.r+sinc*my.i,cs*y.i-sinc*my.r};
    return {mr_mul(phase,u),mr_mul(phase,v)};
}
MR_FN bool mg_edge_color(unsigned i,unsigned n,unsigned color) {
    if(n<2)return false;
    if((n&1u)&&i==n-1)return color==2;
    return (i&1u)==color;
}
#undef MG_SQRT
#endif
