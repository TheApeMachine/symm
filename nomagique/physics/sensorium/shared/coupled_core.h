#ifndef MANIFOLD_COUPLED_CORE_H
#define MANIFOLD_COUPLED_CORE_H
#include "physics_v2_core.h"
#include "radix2_math.h"
#if defined(__METAL_VERSION__)
#define MC_FMOD metal::fmod
#elif defined(__CUDACC__)
#define MC_FMOD fmodf
#else
#define MC_FMOD std::fmod
#endif
struct MFCIC { unsigned index[8];float weight[8];unsigned status; };
MF_FN float mc_wrap(float x,float extent) {
    float r=MC_FMOD(x,extent);return r<0 ? r+extent : r;
}
// Hydro AoS uses z-fast layout; the legacy display grid uses x-fast layout.
// Keep conversion explicit: cubic-only tests would miss this permutation.
MF_FN unsigned mc_hindex(unsigned x,unsigned y,unsigned z,MFHydroParamsV2 p) {return z+p.nz*(y+p.ny*x);}
MF_FN unsigned mc_display_index(unsigned i,MFHydroParamsV2 p) {
    unsigned z=i%p.nz,y=(i/p.nz)%p.ny,x=i/(p.nz*p.ny);
    return x+p.nx*(y+p.ny*z);
}
MF_FN MFCIC mc_cic(float x,float y,float z,MFHydroParamsV2 p) {
    MFCIC out={};float xyz[3]={x,y,z};unsigned dims[3]={p.nx,p.ny,p.nz},base[3];float f[3];
    for(unsigned a=0;a<3;++a) {
        if(!MF_FINITE(xyz[a]) || !(p.dx>0) || !dims[a]) {out.status=MF_PHYSICS_BAD_STATE;return out;}
        float q=mc_wrap(xyz[a],float(dims[a])*p.dx)/p.dx;
        if(!MF_FINITE(q)||q<0||q>float(dims[a])){out.status=MF_PHYSICS_BAD_STATE;return out;}
        // q==dim is a representational periodic boundary, not a state floor.
        if(q==float(dims[a]))q=0;
        base[a]=unsigned(MF_FLOOR(q));f[a]=q-float(base[a]);
    }
    unsigned k=0;
    for(unsigned a=0;a<2;++a)for(unsigned b=0;b<2;++b)for(unsigned c=0;c<2;++c) {
        out.index[k]=mc_hindex((base[0]+a)%p.nx,(base[1]+b)%p.ny,(base[2]+c)%p.nz,p);
        out.weight[k]=(a?f[0]:1-f[0])*(b?f[1]:1-f[1])*(c?f[2]:1-f[2]);++k;
    }
    return out;
}
MF_FN MFHydroStateV2 mc_sample(MF_PTR const MFHydroStateV2* state,MFCIC shape) {
    MFHydroStateV2 s=mf_zero_state();
    for(unsigned k=0;k<8;++k)for(unsigned j=0;j<6;++j)s.q[j]+=shape.weight[k]*state[shape.index[k]].q[j];
    return s;
}
MF_FN unsigned mc_particle_valid(MF_PTR const float* x,MF_PTR const float* v,float m,float q,unsigned i) {
    if(!MF_FINITE(m)||m<=0||!MF_FINITE(q)||q<0)return MF_PHYSICS_BAD_STATE;
    for(unsigned a=0;a<3;++a)if(!MF_FINITE(x[3*i+a])||!MF_FINITE(v[3*i+a]))return MF_PHYSICS_BAD_STATE;
    return 0;
}
MF_FN MFStepResult mc_gravity_increment(MFHydroStateV2 s,float ax,float ay,float az,float dt,MFHydroParamsV2 p) {
    MFStepResult r={};r.state=s;
    if(mf_primitive(s,p).status){r.status=MF_PHYSICS_BAD_STATE;return r;}
    float a[3]={ax,ay,az};
    for(unsigned d=0;d<3;++d) {
        if(!MF_FINITE(a[d])){r.status=MF_PHYSICS_BAD_STATE;return r;}
        float dp=s.q[0]*a[d]*dt;
        r.state.q[4]+=(s.q[1+d]+.5f*dp)*a[d]*dt;
        r.state.q[1+d]+=dp;
    }
    r.status=mf_primitive(r.state,p).status;
    return r;
}
MF_FN unsigned mc_line_length(unsigned axis,MFHydroParamsV2 p){return axis==0?p.nx:axis==1?p.ny:p.nz;}
MF_FN unsigned mc_line_index(unsigned line,unsigned k,unsigned axis,MFHydroParamsV2 p) {
    if(axis==0)return mc_hindex(k,line/p.nz,line%p.nz,p);
    if(axis==1)return mc_hindex(line/p.nz,k,line%p.nz,p);
    return mc_hindex(line/p.ny,line%p.ny,k,p);
}
MF_FN float mc_poisson_eigenvalue(unsigned i,MFHydroParamsV2 p) {
    unsigned k[3]={i/(p.ny*p.nz),(i/p.nz)%p.ny,i%p.nz};unsigned n[3]={p.nx,p.ny,p.nz};float sum=0;
    for(unsigned a=0;a<3;++a){float s=MF_SIN(3.14159265358979323846f*float(k[a])/float(n[a]));sum+=s*s;}
    return -4*sum/(p.dx*p.dx);
}

// Normalized heat kernel/autocorrelation on a periodic circle. The image and
// Fourier forms are Poisson-summation equivalents. For sigma/L<.2 the omitted
// image tail is below 1e-30; otherwise the four-mode Fourier tail is <1e-16.
// sigma==0 is the DECLARED zero-temperature/uniform-coherence limit, not a floor.
MF_FN float mc_periodic_gaussian(float d,float length,float sigma) {
    if(!MF_FINITE(d)||!MF_FINITE(length)||length<=0||!MF_FINITE(sigma)||sigma<0)return mr_nan();
    if(sigma==0 || sigma>=length)return 1; // Fourier correction < 4e-17 at sigma/L>=1
    float a=sigma/length,x=mc_wrap(d,length)/length; if(x>.5f)x-=1;
    if(a==0)return x==0?1:0; // representational zero-width limit (sigma was positive)
    float numerator=0,denominator=1;
    if(a<.2f) {
        float t=x/(2*a);numerator=MR_EXP(-t*t);
        for(unsigned j=1;j<=3;++j){
            float q=float(j)/(2*a),plus=(x+float(j))/(2*a),minus=(x-float(j))/(2*a);
            numerator+=MR_EXP(-plus*plus)+MR_EXP(-minus*minus);denominator+=2*MR_EXP(-q*q);
        }
    }else{
        numerator=1;
        for(unsigned j=1;j<=4;++j){float q=6.2831853071795864769f*a*float(j),c=2*MR_EXP(-q*q);
            numerator+=c*MR_COS(6.2831853071795864769f*float(j)*x);denominator+=c;}
    }
    return numerator/denominator;
}
// Lorentzian response; scale first so finite large frequencies/linewidths do
// not produce an inf/inf NaN. This is exactly gamma^2/(delta^2+gamma^2).
MF_FN float mc_resonance(float a,float b,float gamma){
    if(!MF_FINITE(a)||!MF_FINITE(b)||!MF_FINITE(gamma)||gamma<=0)return mr_nan();
    float scale=mf_max(gamma,mf_max(mf_abs(a),mf_abs(b)));
    float g=gamma/scale,d=a/scale-b/scale;
    if(d==0)return 1; // includes identical frequencies with subnormal linewidth
    float t=g/mf_abs(d);
    if(t>=1){float inv=1/t;return 1/(1+inv*inv);}
    return t*t/(1+t*t);
}
struct MFCouplingBudget {float heat,effective_amp,spent;unsigned status;};
MF_FN MFCouplingBudget mc_coupling_budget(float heat,float amp,float rate,float dt){
    MFCouplingBudget out={};
    if(!mf_finite_nonnegative(heat)||!mf_finite_nonnegative(amp)||!mf_finite_nonnegative(rate)||!mf_finite_nonnegative(dt)) {out.status=MF_PHYSICS_BAD_STATE;return out;}
    // Reordered to avoid overflowing amp*amp before multiplying by a small dt.
    float required=((rate*dt)*amp)*amp;
    if(!MF_FINITE(required)){out.status=MF_PHYSICS_BAD_STATE;return out;}
    out.spent=mf_min(heat,required);out.heat=heat-out.spent;
    float fraction=required>0?out.spent/required:1;
    out.effective_amp=amp*MF_SQRT(fraction);return out;
}
#if defined(__METAL_VERSION__)
#define MC_ATAN2 metal::atan2
#elif defined(__CUDACC__)
#define MC_ATAN2 atan2f
#else
#define MC_ATAN2 std::atan2
#endif
struct MFPhaseFlow {float phase,u0,u1,u2,u3,rate;unsigned status;};
MF_FN float mc_phase_potential(float theta,float real,float imag){return -real*MF_COS(theta)-imag*MF_SIN(theta);}
// theta_dot=omega-dU/dtheta with unit mobility in declared model units,
// U=-Re(B exp(-i theta)). Natural rotation is an external drive. The gradient
// subflow is solved analytically, not by explicit Euler or a stabilizing clamp.
MF_FN MFPhaseFlow mc_phase_flow(float theta,float omega,float real,float imag,float dt){
    MFPhaseFlow out={};
    if(!MF_FINITE(theta)||!MF_FINITE(omega)||!MF_FINITE(real)||!MF_FINITE(imag)||!mf_finite_nonnegative(dt)){out.status=MF_PHYSICS_BAD_STATE;return out;}
    float scale=mf_max(mf_abs(real),mf_abs(imag));
    float amplitude=scale>0 ? scale*MF_SQRT((real/scale)*(real/scale)+(imag/scale)*(imag/scale)):0;out.rate=mf_abs(omega)+amplitude;
    if(!MF_FINITE(out.rate)){out.status=MF_PHYSICS_BAD_STATE;return out;}
    out.u0=mc_phase_potential(theta,real,imag);
    float half=mc_wrap(theta+.5f*dt*omega,6.2831853071795864769f);
    out.u1=mc_phase_potential(half,real,imag);
    float relaxed=half;
    if(amplitude>0){
        float target=MC_ATAN2(imag,real),delta=MC_ATAN2(MF_SIN(half-target),MF_COS(half-target));
        relaxed=target+2*MC_ATAN2(MR_EXP(-amplitude*dt)*MF_SIN(.5f*delta),MF_COS(.5f*delta));
    }
    out.u2=mc_phase_potential(relaxed,real,imag);
    out.phase=mc_wrap(relaxed+.5f*dt*omega,6.2831853071795864769f);
    out.u3=mc_phase_potential(out.phase,real,imag);
    if(!MF_FINITE(out.phase)||!MF_FINITE(out.u0)||!MF_FINITE(out.u1)||!MF_FINITE(out.u2)||!MF_FINITE(out.u3))out.status=MF_PHYSICS_BAD_STATE;
    return out;
}
#endif
