#ifndef SENSORIUM_RECIPROCAL_COHERENCE_H
#define SENSORIUM_RECIPROCAL_COHERENCE_H
#include "coupled_core.h"
#ifndef MC_LOCAL
#if defined(__METAL_VERSION__)
#define MC_LOCAL thread
#else
#define MC_LOCAL
#endif
#endif

// The actual spectral interaction Hamiltonian is
// H_int = -domega * sum_k |psi_k|^2 sum_i A_i R_ik
//             sum_a alpha_ka G(x_i-x_anchor(ka)) / sum_a alpha_ka.
// A, R, alpha and the bath width are frozen parameters of an interaction
// substep. Both the GPE potential and the material/anchor force are derivatives
// of this SAME term. Parameter changes remain separately ledgered work.
struct MCOverlapGradient { float value, derivative; unsigned status; };
MF_FN MCOverlapGradient mc_periodic_gaussian_gradient(float d,float length,float sigma) {
    MCOverlapGradient out={};
    out.value=mc_periodic_gaussian(d,length,sigma);
    if(!MF_FINITE(out.value)){out.status=MF_PHYSICS_BAD_PARAMETERS;return out;}
    if(sigma==0 || sigma>=length)return out;
    float a=sigma/length,x=mc_wrap(d,length)/length;if(x>.5f)x-=1;
    if(a==0){out.status=MF_PHYSICS_BAD_PARAMETERS;return out;}
    float derivative=0,denominator=1;
    if(a<.2f){
        // Form t*exp(-t^2)/sigma, not d/(sigma*sigma). The latter can
        // overflow before multiplying an exponentially tiny weight.
        for(int image=-3;image<=3;++image){
            float t=(x+float(image))/(2*a),v=MR_EXP(-t*t);
            if(v!=0)derivative-=t*v/sigma;
        }
        for(unsigned j=1;j<=3;++j){float t=float(j)/(2*a);denominator+=2*MR_EXP(-t*t);}
    }else{
        for(unsigned j=1;j<=4;++j){
            float q=6.2831853071795864769f*a*float(j),c=2*MR_EXP(-q*q);
            derivative-=c*MF_SIN(6.2831853071795864769f*float(j)*x)*(6.2831853071795864769f*float(j)/length);
            denominator+=c;
        }
    }
    out.derivative=derivative/denominator;
    if(!MF_FINITE(out.derivative))out.status=MF_PHYSICS_BAD_STATE;
    return out;
}
struct MCCoherencePair {float value, gradient[3];unsigned status;};
MF_FN MCCoherencePair mc_coherence_pair(MF_PTR const float* pos,unsigned i,unsigned j,
                                        float sigma,MFHydroParamsV2 p) {
    MCCoherencePair out={};float length[3]={p.nx*p.dx,p.ny*p.dx,p.nz*p.dx};
    MCOverlapGradient axis[3];out.value=1;
    for(unsigned a=0;a<3;++a){
        axis[a]=mc_periodic_gaussian_gradient(pos[3*i+a]-pos[3*j+a],length[a],sigma);
        if(axis[a].status){out.status=axis[a].status;return out;}
        out.value*=axis[a].value;
    }
    for(unsigned a=0;a<3;++a)
        out.gradient[a]=axis[a].derivative*axis[(a+1)%3].value*axis[(a+2)%3].value;
    return out;
}
struct MCAnchorNorm {float weight;unsigned status;};
MF_FN MCAnchorNorm mc_anchor_norm(MF_PTR const unsigned* indices,MF_PTR const float* weights,
                                   unsigned mode,unsigned particles,unsigned anchors) {
    MCAnchorNorm out={};
    for(unsigned a=0;a<anchors;++a){unsigned j=indices[mode*anchors+a];if(j==0xffffffffu)continue;
        float w=weights[mode*anchors+a];
        if(j>=particles||!mf_finite_nonnegative(w)){out.status=MF_PHYSICS_BAD_STATE;return out;}
        out.weight+=w;
    }
    if(!MF_FINITE(out.weight))out.status=MF_PHYSICS_BAD_STATE;
    return out;
}
MF_FN float mc_coherence_potential(MF_PTR const float* pos,MF_PTR const float* amplitude,
    MF_PTR const float* omega,MF_PTR const float* mode_omega,MF_PTR const float* linewidth,
    MF_PTR const unsigned* indices,MF_PTR const float* weights,unsigned mode,
    unsigned particles,unsigned anchors,float sigma,MFHydroParamsV2 p) {
    auto norm=mc_anchor_norm(indices,weights,mode,particles,anchors);
    if(norm.status)return mr_nan();if(norm.weight==0)return 0;
    float value=0;
    for(unsigned i=0;i<particles;++i){
        if(!mf_finite_nonnegative(amplitude[i]))return mr_nan();
        float r=mc_resonance(omega[i],mode_omega[mode],linewidth[mode]);
        if(!MF_FINITE(r))return mr_nan();
        float overlap=0;
        for(unsigned a=0;a<anchors;++a){unsigned j=indices[mode*anchors+a];if(j==0xffffffffu||weights[mode*anchors+a]==0)continue;
            auto term=mc_coherence_pair(pos,i,j,sigma,p);if(term.status)return mr_nan();
            overlap+=weights[mode*anchors+a]*term.value;
        }
        value-=amplitude[i]*r*overlap/norm.weight;
    }
    return value;
}
#endif
