#ifndef SENSORIUM_PILOT_TEMPORAL_H
#define SENSORIUM_PILOT_TEMPORAL_H
#include "pilot_checked.h"

// Piecewise-linear time interpolation of the *complex field*, not of its
// phase or velocity. Space-time node certificates use all 16 box corners.
struct MCPilotField {
    MF_PTR const float* re0;
    MF_PTR const float* im0;
    MF_PTR const float* re1;
    MF_PTR const float* im1;
};
MF_FN MCPilotSample mc_pilot_sample_time(MCPilotField field,float t,
    float x,float y,float z,MFHydroParamsV2 p) {
    if(field.re0==field.re1&&field.im0==field.im1)
        return mc_pilot_sample(field.re0,field.im0,x,y,z,p);
    MCPilotSample a=mc_pilot_sample(field.re0,field.im0,x,y,z,p);
    MCPilotSample b=mc_pilot_sample(field.re1,field.im1,x,y,z,p),out={};
    if(a.status&&a.status!=MF_PHYSICS_UNRESOLVED_NODE)return a;
    if(b.status&&b.status!=MF_PHYSICS_UNRESOLVED_NODE)return b;
    // Endpoint nodes need not be nodes at an interior time. Combine the raw
    // scaled coefficients before deciding whether the requested ratio resolves.
    if(!MF_FINITE(t)||t<0||t>1){out.status=MF_PHYSICS_BAD_PARAMETERS;return out;}
    out.scale=mf_max((1-t)*a.scale,t*b.scale);
    if(out.scale==0){out.status=MF_PHYSICS_UNRESOLVED_NODE;return out;}
    float wa=(1-t)*(a.scale/out.scale),wb=t*(b.scale/out.scale);
    out.r=wa*a.r+wb*b.r;out.i=wa*a.i+wb*b.i;
    for(unsigned axis=0;axis<3;++axis){out.dr[axis]=wa*a.dr[axis]+wb*b.dr[axis];out.di[axis]=wa*a.di[axis]+wb*b.di[axis];}
    float rho=out.r*out.r+out.i*out.i;
    constexpr float eta=32*1.1920928955078125e-7f;
    if(!(rho>eta*eta)){out.status=MF_PHYSICS_UNRESOLVED_NODE;return out;}
    out.density=(out.scale*out.scale)*rho;
    if(!MF_FINITE(out.density))out.status=MF_PHYSICS_BAD_STATE;
    return out;
}
MF_FN MCPilotVelocity mc_pilot_velocity_time(MCPilotField field,float time,
    float x,float y,float z,float mass,float hbar,MFHydroParamsV2 p){
    MCPilotVelocity out={};
    if(!MF_FINITE(mass)||mass<=0||!MF_FINITE(hbar)||hbar<=0){out.status=MF_PHYSICS_BAD_PARAMETERS;return out;}
    auto s=mc_pilot_sample_time(field,time,x,y,z,p);out.status=s.status;if(out.status)return out;
    float rho=s.r*s.r+s.i*s.i;out.density=s.density;
    for(unsigned a=0;a<3;++a){out.v[a]=(hbar/mass)*(s.r*s.di[a]-s.i*s.dr[a])/rho;if(!MF_FINITE(out.v[a]))out.status=MF_PHYSICS_BAD_STATE;}
    return out;
}
MF_FN bool mc_pilot_segment_clear_time(MCPilotField field,MC_LOCAL const float* x,
    MC_LOCAL const float* y,float t0,float t1,MFHydroParamsV2 p){
    float cuts[8]={0,1};unsigned count=2;
    for(unsigned a=0;a<3;++a){float d=y[a]-x[a];if(mf_abs(d)>.5f*p.dx)return false;
        if(d==0)continue;float low=mf_min(x[a],y[a]),high=mf_max(x[a],y[a]);
        float face=(MF_FLOOR(low/p.dx)+1)*p.dx;
        if(face>low&&face<high)cuts[count++]=(face-x[a])/d;
    }
    for(unsigned i=1;i<count;++i){float v=cuts[i];unsigned j=i;while(j&&cuts[j-1]>v){cuts[j]=cuts[j-1];--j;}cuts[j]=v;}
    for(unsigned c=0;c+1<count;++c){if(cuts[c+1]==cuts[c])continue;
        float a[3],b[3];for(unsigned d=0;d<3;++d){a[d]=x[d]+cuts[c]*(y[d]-x[d]);b[d]=x[d]+cuts[c+1]*(y[d]-x[d]);}
        float ta=t0+cuts[c]*(t1-t0),tb=t0+cuts[c+1]*(t1-t0);
        auto s=mc_pilot_sample_time(field,.5f*(ta+tb),.5f*(a[0]+b[0]),.5f*(a[1]+b[1]),.5f*(a[2]+b[2]),p);if(s.status)return false;
        for(unsigned k=0;k<16;++k){
            if(((k&1)&&a[0]==b[0])||((k&2)&&a[1]==b[1])||((k&4)&&a[2]==b[2])||((k&8)&&ta==tb))continue;
            auto corner=mc_pilot_sample_time(field,(k&8)?tb:ta,(k&1)?b[0]:a[0],(k&2)?b[1]:a[1],(k&4)?b[2]:a[2],p);
            if(corner.status)return false;
            if(!(s.r*corner.r+s.i*corner.i>64*1.1920928955078125e-7f))return false;
        }
    }
    return true;
}
MF_FN MCPilotAdvance mc_pilot_midpoint_time(MCPilotField field,MC_LOCAL const float* x,
    MC_LOCAL const float* prior,float mass,float hbar,float dt,float t0,float t1,MFHydroParamsV2 p){
    MCPilotAdvance out={};auto first=mc_pilot_velocity_time(field,t0,x[0],x[1],x[2],mass,hbar,p);out.status=first.status;if(out.status)return out;
    float mid[3];for(unsigned a=0;a<3;++a)mid[a]=x[a]+.5f*dt*(first.v[a]-prior[a]);
    float tm=.5f*(t0+t1);
    if(!mc_pilot_segment_clear_time(field,x,mid,t0,tm,p)){out.status=MF_PHYSICS_CFL;return out;}
    auto middle=mc_pilot_velocity_time(field,tm,mid[0],mid[1],mid[2],mass,hbar,p);out.status=middle.status;if(out.status)return out;
    float d2=0;
    for(unsigned a=0;a<3;++a){float d=dt*(middle.v[a]-prior[a]);out.x[a]=x[a]+d;d2+=d*d;}
    if(!mc_pilot_segment_clear_time(field,x,out.x,t0,t1,p)){out.status=MF_PHYSICS_CFL;return out;}
    auto last=mc_pilot_velocity_time(field,t1,out.x[0],out.x[1],out.x[2],mass,hbar,p);out.status=last.status;if(out.status)return out;
    out.density=last.density;out.pathcells=MF_SQRT(d2)/p.dx;
    float speed2=0;for(unsigned a=0;a<3;++a){out.guide[a]=last.v[a];speed2+=last.v[a]*last.v[a];}out.speed=MF_SQRT(speed2);return out;
}
// A trial advances one subinterval of the same accepted physical interval.
// The positions stay unwrapped until the complete trajectory commits.
MF_FN MCPilotAdvance mc_pilot_trial_time(MCPilotField field,MC_LOCAL const float* x,
    MC_LOCAL const float* prior,float mass,float hbar,float dt,float t0,float t1,MFHydroParamsV2 p){
    auto full=mc_pilot_midpoint_time(field,x,prior,mass,hbar,dt,t0,t1,p);if(full.status)return full;
    float tm=.5f*(t0+t1);
    auto half=mc_pilot_midpoint_time(field,x,prior,mass,hbar,.5f*dt,t0,tm,p);if(half.status)return half;
    auto out=mc_pilot_midpoint_time(field,half.x,prior,mass,hbar,.5f*dt,tm,t1,p);if(out.status)return out;
    float e2=0;for(unsigned a=0;a<3;++a){float d=(out.x[a]-full.x[a])/p.dx;e2+=d*d;}
    out.error=MF_SQRT(e2)/3;out.pathcells+=half.pathcells;
    return out;
}
MF_FN MCPilotAdvance mc_pilot_checked_time(MCPilotField field,MC_LOCAL const float* x,
    MC_LOCAL const float* prior,float mass,float hbar,float dt,float tolerance,float maxcells,MFHydroParamsV2 p){
    MCPilotAdvance out={};float current[3]={x[0],x[1],x[2]};
    // Reject an undefined initial law immediately. Internal subdivisions can
    // resolve a moving field, not make an actual initial node well-defined.
    auto initial=mc_pilot_velocity_time(field,0,x[0],x[1],x[2],mass,hbar,p);
    if(initial.status){out.status=initial.status;return out;}
    float time=0,span=1,error=0,path=0;
    for(unsigned attempts=0;attempts<4096;++attempts){
        span=mf_min(span,1-time);
        if(!(time+span>time)){out.status=MF_PHYSICS_CFL;return out;}
        auto trial=mc_pilot_trial_time(field,current,prior,mass,hbar,dt*span,time,time+span,p);
        if(trial.status && trial.status!=MF_PHYSICS_CFL && trial.status!=MF_PHYSICS_UNRESOLVED_NODE)return trial;
        float remaining=tolerance-error;
        if(trial.status || trial.error>.25f*remaining){span*=.5f;continue;}
        path+=trial.pathcells;error+=trial.error;
        if(path>maxcells || error>tolerance){out.status=MF_PHYSICS_CFL;return out;}
        for(unsigned a=0;a<3;++a)current[a]=trial.x[a];
        time+=span;out=trial;
        if(time>=1){
            out.error=error;out.pathcells=path;
            float dims[3]={float(p.nx),float(p.ny),float(p.nz)};
            for(unsigned a=0;a<3;++a)out.x[a]=mc_wrap(current[a],dims[a]*p.dx);
            return out;
        }
        if(trial.error<.03125f*(tolerance-error))span*=2;
    }
    out.status=MF_PHYSICS_CFL;return out;
}
#endif
