#ifndef MANIFOLD_CONTACT_HASH_H
#define MANIFOLD_CONTACT_HASH_H
#include "conservative_remap.h"
// Generic periodic cell reach, with each cell enumerated ONCE even for one- or
// two-cell dimensions and cutoff larger than cell size. No 27-cell assumption.
struct MCNeighborAxis {unsigned first,count;};
MF_FN MCNeighborAxis mc_neighbor_axis(unsigned center,unsigned n,unsigned reach){
    MCNeighborAxis out={};if(reach>=n/2){out.first=0;out.count=n;}
    else{out.first=(center+n-reach)%n;out.count=2*reach+1;}return out;
}
MF_FN MFContactEvaluation mc_contact_hash_evaluate(MF_PTR const MFParticleStateV2* state,
    MF_PTR const unsigned* starts,MF_PTR const unsigned* sorted,unsigned i,MFContactParamsV2 c,MFHydroParamsV2 g,float max_speed){
    MFContactEvaluation out={};if(!mf_contact_params_valid(c)||!MF_FINITE(max_speed)||max_speed<0){out.status=MF_PHYSICS_BAD_PARAMETERS;return out;}
    unsigned dims[3]={g.nx,g.ny,g.nz},base[3],reach=unsigned(MF_FLOOR(2*c.radius/g.dx))+1;
    for(unsigned a=0;a<3;++a){float q=mc_wrap(state[i].x[a],dims[a]*g.dx)/g.dx;if(q==float(dims[a]))q=0;if(!MF_FINITE(q)||q<0){out.status=MF_PHYSICS_BAD_STATE;return out;}base[a]=unsigned(q);}
    MCNeighborAxis x=mc_neighbor_axis(base[0],g.nx,reach),y=mc_neighbor_axis(base[1],g.ny,reach),z=mc_neighbor_axis(base[2],g.nz,reach);
    for(unsigned ax=0;ax<x.count;++ax)for(unsigned ay=0;ay<y.count;++ay)for(unsigned az=0;az<z.count;++az){
        unsigned cell=mc_hindex((x.first+ax)%g.nx,(y.first+ay)%g.ny,(z.first+az)%g.nz,g);
        unsigned begin=starts[cell],end=starts[cell+1];if(begin>end||end>c.n){out.status=MF_PHYSICS_BAD_STATE;return out;}
        for(unsigned k=begin;k<end;++k){unsigned j=sorted[k];if(j>=c.n){out.status=MF_PHYSICS_BAD_STATE;return out;}if(j!=i){out=mf_contact_sum(out,mf_contact_pair(state[i],state[j],c));if(out.status)return out;}}
    }
    // max_speed is a global relative-speed bound from component ranges.
    // It is invariant under a common Galilean boost.
    return mf_contact_finish(out,state[i].heat,max_speed/c.radius,c);
}
// Frozen-position contact subflow. The material drift occurs ONCE in the gas/PIC
// transport operator. Two Heun evaluations integrate force/thermal transfer;
// an elastic force is constant during this kick, so its momentum is exact.
// The surrounding kick-drift-kick splits the Hertz potential from material
// transport without adding an extra position advance or restitution impulse.
MF_FN MFContactStepResult mc_contact_kick_state(MFParticleStateV2 initial,MFParticleStateV2 trial,MFContactEvaluation f,unsigned second){
    MFContactStepResult out={};out.state=initial;out.status=f.status;if(out.status)return out;
    float dv[3]={f.result.dvx,f.result.dvy,f.result.dvz};
    for(unsigned a=0;a<3;++a)out.state.v[a]=second?.5f*(initial.v[a]+trial.v[a]+dv[a]):initial.v[a]+dv[a];
    out.state.heat=second?.5f*(initial.heat+trial.heat+f.result.heat_increment):initial.heat+f.result.heat_increment;
    if(!mf_finite_nonnegative(out.state.heat))out.status=MF_PHYSICS_NEGATIVE_UPDATE;
    for(unsigned a=0;a<3;++a)if(!MF_FINITE(out.state.v[a]))out.status=MF_PHYSICS_BAD_STATE;
    return out;
}
#endif
