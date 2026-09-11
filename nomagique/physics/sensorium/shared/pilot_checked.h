#ifndef MANIFOLD_PILOT_CHECKED_H
#define MANIFOLD_PILOT_CHECKED_H
#include "conservative_remap.h"
// Interpolate the complex field, not its phase. Scaling BEFORE multiplication
// makes the current ratio amplitude-invariant without a density/mass floor.
struct MCPilotSample { float r,i,dr[3],di[3],scale,density; unsigned status; };
MF_FN MCPilotSample mc_pilot_sample(MF_PTR const float* re,MF_PTR const float* im,
    float x,float y,float z,MFHydroParamsV2 p) {
    MCPilotSample o={};float xyz[3]={x,y,z},f[3];unsigned n[3]={p.nx,p.ny,p.nz},base[3];
    for(unsigned a=0;a<3;++a){if(!MF_FINITE(xyz[a])){o.status=MF_PHYSICS_BAD_STATE;return o;}
        float q=mc_wrap(xyz[a],n[a]*p.dx)/p.dx;if(q==float(n[a]))q=0;
        base[a]=unsigned(MF_FLOOR(q));f[a]=q-base[a];}
    unsigned ids[8],k=0;
    for(unsigned a=0;a<2;++a)for(unsigned b=0;b<2;++b)for(unsigned c=0;c<2;++c){
        if((n[0]==1&&a)||(n[1]==1&&b)||(n[2]==1&&c))continue;
        unsigned j=mc_hindex((base[0]+a)%n[0],(base[1]+b)%n[1],(base[2]+c)%n[2],p);ids[k++]=j;
        if(!MF_FINITE(re[j])||!MF_FINITE(im[j])){o.status=MF_PHYSICS_BAD_STATE;return o;}
        o.scale=mf_max(o.scale,mf_max(mf_abs(re[j]),mf_abs(im[j])));}
    if(o.scale==0){o.status=MF_PHYSICS_UNRESOLVED_NODE;return o;}
    k=0;for(unsigned a=0;a<2;++a)for(unsigned b=0;b<2;++b)for(unsigned c=0;c<2;++c){
        if((n[0]==1&&a)||(n[1]==1&&b)||(n[2]==1&&c))continue;
        float w[3]={n[0]==1?1.f:(a?f[0]:1-f[0]),n[1]==1?1.f:(b?f[1]:1-f[1]),n[2]==1?1.f:(c?f[2]:1-f[2])};
        float sgn[3]={n[0]==1?0.f:(a?1.f:-1.f),n[1]==1?0.f:(b?1.f:-1.f),n[2]==1?0.f:(c?1.f:-1.f)};
        float r=re[ids[k]]/o.scale,i=im[ids[k]]/o.scale;++k;
        o.r+=w[0]*w[1]*w[2]*r;o.i+=w[0]*w[1]*w[2]*i;
        for(unsigned d=0;d<3;++d){float dw=sgn[d]*w[(d+1)%3]*w[(d+2)%3]/p.dx;o.dr[d]+=dw*r;o.di[d]+=dw*i;}}
    float rho=o.r*o.r+o.i*o.i;
    // Numerical interpolation resolution, relative to the local coefficients.
    // There is NO altered denominator: either resolve the ratio, or reject.
    constexpr float eta=32*1.1920928955078125e-7f;
    if(!(rho>eta*eta)){o.status=MF_PHYSICS_UNRESOLVED_NODE;return o;}
    o.density=(o.scale*o.scale)*rho;
    if(!MF_FINITE(o.density))o.status=MF_PHYSICS_BAD_STATE;
    return o;
}
struct MCPilotVelocity {float v[3],density;unsigned status;};
MF_FN MCPilotVelocity mc_pilot_velocity(MF_PTR const float* re,MF_PTR const float* im,
    float x,float y,float z,float mass,float hbar,MFHydroParamsV2 p){
    MCPilotVelocity o={};if(!MF_FINITE(mass)||mass<=0||!MF_FINITE(hbar)||hbar<=0){o.status=MF_PHYSICS_BAD_PARAMETERS;return o;}
    MCPilotSample s=mc_pilot_sample(re,im,x,y,z,p);o.status=s.status;if(o.status)return o;
    o.density=s.density;float rho=s.r*s.r+s.i*s.i;
    for(unsigned a=0;a<3;++a){o.v[a]=(hbar/mass)*(s.r*s.di[a]-s.i*s.dr[a])/rho;if(!MF_FINITE(o.v[a]))o.status=MF_PHYSICS_BAD_STATE;}
    return o;
}
// A segment is split at every crossed grid face. On each resulting cell box,
// a multi-affine interpolant is a convex combination of its eight corner
// values. A strictly positive projection onto the start field certifies that
// the entire box (hence the segment) excludes zero, with a rounding margin.
// Failure is conservative: a valid path may request a smaller step.
MF_FN bool mc_pilot_segment_clear(MF_PTR const float* re,MF_PTR const float* im,
    MC_LOCAL const float* x,MC_LOCAL const float* y,MFHydroParamsV2 p){
    float cuts[8]={0,1};unsigned count=2;
    for(unsigned a=0;a<3;++a){float d=y[a]-x[a];if(mf_abs(d)>.5f*p.dx)return false;
        if(d==0)continue;float low=mf_min(x[a],y[a]),high=mf_max(x[a],y[a]);
        float face=(MF_FLOOR(low/p.dx)+1)*p.dx;
        if(face>low&&face<high)cuts[count++]=(face-x[a])/d;}
    for(unsigned i=1;i<count;++i){float v=cuts[i];unsigned j=i;while(j&&cuts[j-1]>v){cuts[j]=cuts[j-1];--j;}cuts[j]=v;}
    for(unsigned c=0;c+1<count;++c){if(cuts[c+1]==cuts[c])continue;
        float a[3],b[3];for(unsigned d=0;d<3;++d){a[d]=x[d]+cuts[c]*(y[d]-x[d]);b[d]=x[d]+cuts[c+1]*(y[d]-x[d]);}
        auto s=mc_pilot_sample(re,im,.5f*(a[0]+b[0]),.5f*(a[1]+b[1]),.5f*(a[2]+b[2]),p);if(s.status)return false;
        for(unsigned k=0;k<8;++k){
            if(((k&1)&&a[0]==b[0])||((k&2)&&a[1]==b[1])||((k&4)&&a[2]==b[2]))continue;
            auto corner=mc_pilot_sample(re,im,(k&1)?b[0]:a[0],(k&2)?b[1]:a[1],(k&4)?b[2]:a[2],p);
            if(corner.status)return false;
            float dot=s.r*corner.r+s.i*corner.i;
            if(!(dot>64*1.1920928955078125e-7f))return false;}
    }return true;
}
struct MCPilotAdvance {float x[3],guide[3],density,error,pathcells,speed;unsigned status;};
MF_FN MCPilotAdvance mc_pilot_midpoint(MF_PTR const float* re,MF_PTR const float* im,
    MC_LOCAL const float* x,MC_LOCAL const float* prior,float mass,float hbar,float dt,MFHydroParamsV2 p){
    MCPilotAdvance o={};auto v0=mc_pilot_velocity(re,im,x[0],x[1],x[2],mass,hbar,p);o.status=v0.status;if(o.status)return o;
    float mid[3];for(unsigned a=0;a<3;++a)mid[a]=x[a]+.5f*dt*(v0.v[a]-prior[a]);
    if(!mc_pilot_segment_clear(re,im,x,mid,p)){o.status=MF_PHYSICS_CFL;return o;}
    auto vm=mc_pilot_velocity(re,im,mid[0],mid[1],mid[2],mass,hbar,p);o.status=vm.status;if(o.status)return o;
    float length2=0;for(unsigned a=0;a<3;++a){float d=dt*(vm.v[a]-prior[a]);o.x[a]=x[a]+d;length2+=d*d;}
    if(!mc_pilot_segment_clear(re,im,x,o.x,p)){o.status=MF_PHYSICS_CFL;return o;}
    auto end=mc_pilot_velocity(re,im,o.x[0],o.x[1],o.x[2],mass,hbar,p);o.status=end.status;if(o.status)return o;
    o.pathcells=MF_SQRT(length2)/p.dx;o.density=end.density;
    float speed2=0;for(unsigned a=0;a<3;++a){o.guide[a]=end.v[a];speed2+=end.v[a]*end.v[a];}o.speed=MF_SQRT(speed2);return o;
}
MF_FN MCPilotAdvance mc_pilot_checked(MF_PTR const float* re,MF_PTR const float* im,
    MC_LOCAL const float* x,MC_LOCAL const float* prior,float mass,float hbar,float dt,float tolerance,float maxcells,MFHydroParamsV2 p){
    auto full=mc_pilot_midpoint(re,im,x,prior,mass,hbar,dt,p);if(full.status)return full;
    auto half=mc_pilot_midpoint(re,im,x,prior,mass,hbar,.5f*dt,p);if(half.status)return half;
    auto out=mc_pilot_midpoint(re,im,half.x,prior,mass,hbar,.5f*dt,p);if(out.status)return out;
    float e2=0;for(unsigned a=0;a<3;++a){float d=(out.x[a]-full.x[a])/p.dx;e2+=d*d;}
    out.error=MF_SQRT(e2)/3;out.pathcells+=half.pathcells;
    if(out.error>tolerance||out.pathcells>maxcells){out.status=MF_PHYSICS_CFL;return out;}
    float dims[3]={float(p.nx),float(p.ny),float(p.nz)};for(unsigned a=0;a<3;++a)out.x[a]=mc_wrap(out.x[a],dims[a]*p.dx);
    return out;
}
#endif
