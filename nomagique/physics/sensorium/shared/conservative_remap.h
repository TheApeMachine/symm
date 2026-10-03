#ifndef MANIFOLD_CONSERVATIVE_REMAP_H
#define MANIFOLD_CONSERVATIVE_REMAP_H
#include "coupled_core.h"
#if defined(__METAL_VERSION__)
#define MC_LOG metal::log
#define MC_LOCAL thread
#elif defined(__CUDACC__)
#define MC_LOG logf
#define MC_LOCAL
#else
#define MC_LOG std::log
#define MC_LOCAL
#endif
// A transport matrix T_cp partitions cell mass into fixed-mass parcels.
// Rows sum to rho_c*dV (up to the separately recorded mass-roundoff scale),
// columns sum to m_p.  Kernel exp(-periodic_distance^2/(2*sigma^2)) is a
// NUMERICAL locality prior; sigma=width_cells*dx, not a physical force.
// Log-domain Sinkhorn scaling never truncates support or floors density.
MF_FN float mc_transport_cost(unsigned c,MF_PTR const float* x,unsigned i,
                              MFHydroParamsV2 p,float width_cells) {
    unsigned coord[3]={c/(p.ny*p.nz),(c/p.nz)%p.ny,c%p.nz};
    unsigned dim[3]={p.nx,p.ny,p.nz};float s=0;
    for(unsigned a=0;a<3;++a){
        float extent=float(dim[a])*p.dx;
        float d=mc_wrap(x[3*i+a]-float(coord[a])*p.dx+.5f*extent,extent)-.5f*extent;
        float q=d/(p.dx*width_cells);s+=.5f*q*q;
    }
    return s;
}
// Unequal marginals produce large dual potentials even when their sum is
// small. Store a two-float expansion so cancellation does not set the solver's
// convergence floor. Requires strict IEEE addition (no fast-math reassociation).
struct MCReal {float hi,lo;};
MF_FN MCReal mc_real_add(MCReal a,MCReal b){
    float sum=a.hi+b.hi,virtual_b=sum-a.hi;
    float error=(a.hi-(sum-virtual_b))+(b.hi-virtual_b)+a.lo+b.lo;
    float hi=sum+error;return {hi,error-(hi-sum)};
}
MF_FN MCReal mc_real_sub(MCReal a,MCReal b){return mc_real_add(a,{-b.hi,-b.lo});}
MF_FN float mc_real_value(MCReal a){return a.hi+a.lo;}
MF_FN MCReal mc_real_load(MF_PTR const float* values,unsigned i){return {values[2*i],values[2*i+1]};}
MF_FN bool mc_real_greater(MCReal a,MCReal b){return a.hi>b.hi||(a.hi==b.hi&&a.lo>b.lo);}
struct MCLogSum {MCReal maximum;float sum,correction;unsigned count;};
MF_FN MCReal mc_remap_logweight(unsigned cell,MF_PTR const float* pos,unsigned particle,
    MFHydroParamsV2 p,float width,MF_PTR const float* rows){
    return mc_real_add(mc_real_load(rows,cell),{-mc_transport_cost(cell,pos,particle,p,width),0});
}
MF_FN void mc_log_add(MC_LOCAL MCLogSum& s,MCReal x){
    if(!s.count){s.maximum=x;s.sum=1;s.correction=0;s.count=1;return;}
    if(mc_real_greater(x,s.maximum)){float factor=MR_EXP(mc_real_value(mc_real_sub(s.maximum,x)));s.sum*=factor;s.correction*=factor;s.maximum=x;}
    float y=MR_EXP(mc_real_value(mc_real_sub(x,s.maximum)))-s.correction,t=s.sum+y;
    s.correction=(t-s.sum)-y;s.sum=t;++s.count;
}
MF_FN MCReal mc_log_total(MCLogSum s){return mc_real_add(s.maximum,{MC_LOG(s.sum),0});}
MF_FN float mc_row_mass(MFHydroStateV2 s,MFHydroParamsV2 p,float mass_scale){return s.q[0]*(p.dx*p.dx*p.dx)*mass_scale;}
// Weighted mean/variance is evaluated about a reference velocity. This avoids
// E-K cancellation and prevents high-Mach bulk speed contaminating mixing heat.
struct MCTransportMoments {float weight,delta[3],thermal,total,variance,correction[6];};
MF_FN void mc_compensated_add(MC_LOCAL float& sum,MC_LOCAL float& correction,float value){float y=value-correction,t=sum+y;correction=(t-sum)-y;sum=t;}
#endif
