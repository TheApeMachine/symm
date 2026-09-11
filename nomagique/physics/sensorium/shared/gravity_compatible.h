#ifndef MANIFOLD_GRAVITY_COMPATIBLE_H
#define MANIFOLD_GRAVITY_COMPATIBLE_H
#include "conservative_remap.h"
// Periodic Poisson is self-adjoint. Thus Delta U_g = sum Delta rho *
// (phi_old+phi_new)/2 * volume. Reuse the EXACT RK2 mass fluxes to distribute
// -Delta U_g locally, not a global scalar energy repair.
struct MCGravityWork {float density;unsigned status;};
MF_FN float mc_phi_average(MF_PTR const float* a,MF_PTR const float* b,unsigned i,MFHydroParamsV2 p){
    unsigned j=mc_display_index(i,p);return .5f*(a[j]+b[j]);
}
MF_FN MCGravityWork mc_gravity_flux_work(MF_PTR const MFHydroStateV2* initial,MF_PTR const MFHydroStateV2* stage,
    MF_PTR const float* phi0,MF_PTR const float* phi1,unsigned i,MFHydroParamsV2 p){
    MCGravityWork out={};float center=mc_phi_average(phi0,phi1,i,p);
    for(unsigned a=0;a<3;++a){unsigned lo=mf_hneighbor(i,a,-1,p),hi=mf_hneighbor(i,a,+1,p);
        auto f0=mf_face(initial,i,a,p),f1=mf_face(stage,i,a,p),g0=mf_face(initial,lo,a,p),g1=mf_face(stage,lo,a,p);
        if(f0.status||f1.status||g0.status||g1.status){out.status=MF_PHYSICS_BAD_STATE;return out;}
        float fp=.5f*(f0.flux.q[0]+f1.flux.q[0]),fm=.5f*(g0.flux.q[0]+g1.flux.q[0]);
        float left=mc_phi_average(phi0,phi1,lo,p),right=mc_phi_average(phi0,phi1,hi,p);
        out.density-=.5f*(p.dt/p.dx)*(fp*(right-center)+fm*(center-left));
    }
    if(!MF_FINITE(out.density))out.status=MF_PHYSICS_BAD_STATE;return out;
}
// A conservative remap B_cp moves mass from grid cells to particle shapes.
// Work for that numerical transport is -sum B_cp (W_p phi_bar - phi_bar_c).
// This is the same discrete potential-energy identity, with the remap flux
// replacing the hydro flux. It cannot be replaced by arbitrary energy scaling.
MF_FN MCGravityWork mc_gravity_remap_work(MF_PTR const MFHydroStateV2* state,MF_PTR const float* pos,
    MF_PTR const float* mass,MF_PTR const float* rows,MF_PTR const float* phi0,MF_PTR const float* phi1,
    unsigned i,float width,MFHydroParamsV2 p){
    MCGravityWork out={};auto shape=mc_cic(pos[3*i],pos[3*i+1],pos[3*i+2],p);if(shape.status){out.status=shape.status;return out;}
    float at_particle=0;for(unsigned k=0;k<8;++k)at_particle+=shape.weight[k]*mc_phi_average(phi0,phi1,shape.index[k],p);
    MCLogSum ls={};for(unsigned c=0;c<p.n;++c)if(state[c].q[0]>0)mc_log_add(ls,rows[c]-mc_transport_cost(c,pos,i,p,width));
    float work=0,correction=0;for(unsigned c=0;c<p.n;++c)if(state[c].q[0]>0){float f=MR_EXP(rows[c]-mc_transport_cost(c,pos,i,p,width)-ls.maximum)/ls.sum;
        mc_compensated_add(work,correction,f*(mc_phi_average(phi0,phi1,c,p)-at_particle));}
    out.density=mass[i]*work;if(!MF_FINITE(out.density)||!ls.count)out.status=MF_PHYSICS_BAD_STATE;return out;
}
#endif
