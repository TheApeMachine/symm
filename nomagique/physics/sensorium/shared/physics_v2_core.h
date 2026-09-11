#ifndef MANIFOLD_PHYSICS_V2_CORE_H
#define MANIFOLD_PHYSICS_V2_CORE_H
#include "physics_v2_types.h"

/* One mathematical implementation, compiled by Metal, CUDA, and the CPU tests.
   No floors, velocity caps, density replacement, or energy renormalization. */
#if defined(__METAL_VERSION__)
#define MF_FN inline
#define MF_PTR device
#define MF_FINITE metal::isfinite
#define MF_SQRT metal::sqrt
#define MF_SIN metal::sin
#define MF_COS metal::cos
#define MF_FLOOR metal::floor
#elif defined(__CUDACC__)
#include <cmath>
#define MF_FN __host__ __device__ inline
#define MF_PTR
#define MF_FINITE isfinite
#define MF_SQRT sqrtf
#define MF_SIN sinf
#define MF_COS cosf
#define MF_FLOOR floorf
#else
#include <cmath>
#define MF_FN inline
#define MF_PTR
#define MF_FINITE std::isfinite
#define MF_SQRT std::sqrt
#define MF_SIN std::sin
#define MF_COS std::cos
#define MF_FLOOR std::floor
#endif

MF_FN float mf_abs(float a) { return a < 0 ? -a : a; }
MF_FN float mf_min(float a, float b) { return a < b ? a : b; }
MF_FN float mf_max(float a, float b) { return a > b ? a : b; }
MF_FN bool mf_finite_nonnegative(float a) { return MF_FINITE(a) && a >= 0; }
MF_FN MFHydroStateV2 mf_zero_state() {
    MFHydroStateV2 r;
    for (unsigned c=0;c<6;++c) r.q[c]=0;
    return r;
}
MF_FN bool mf_grid_valid(unsigned n, unsigned nx, unsigned ny, unsigned nz) {
    if (!n || !nx || !ny || !nz) return false;
    /* Division tests avoid overflowing even in a shader. */
    return n % nx == 0 && (n/nx) % ny == 0 && (n/nx)/ny == nz;
}
MF_FN bool mf_hydro_params_valid(MFHydroParamsV2 p) {
    return mf_grid_valid(p.n,p.nx,p.ny,p.nz) &&
        MF_FINITE(p.dx) && p.dx>0 && MF_FINITE(p.dt) && p.dt>0 &&
        MF_FINITE(p.gamma) && p.gamma>1 && MF_FINITE(p.cv) && p.cv>0 &&
        mf_finite_nonnegative(p.mu) && mf_finite_nonnegative(p.bulk_viscosity) &&
        mf_finite_nonnegative(p.k_thermal) &&
        MF_FINITE(p.eta_pressure) && p.eta_pressure>0 && p.eta_pressure<1 &&
        MF_FINITE(p.eta_sync) && p.eta_sync>=p.eta_pressure && p.eta_sync<1 &&
        MF_FINITE(p.cfl) && p.cfl>0 && p.cfl<=0.5f &&
        p.reconstruction<=1 && p.gravity<=1;
}
MF_FN unsigned mf_neighbor(unsigned i, unsigned a, int direction,
                           unsigned nx, unsigned ny, unsigned nz) {
    unsigned stride = a==0 ? ny*nz : (a==1 ? nz : 1u);
    unsigned dim = a==0 ? nx : (a==1 ? ny : nz);
    unsigned coordinate = (i/stride)%dim;
    if (direction>0) return coordinate+1==dim ? i-(dim-1)*stride : i+stride;
    return coordinate==0 ? i+(dim-1)*stride : i-stride;
}
MF_FN unsigned mf_hneighbor(unsigned i, unsigned a, int d, MFHydroParamsV2 p) {
    return mf_neighbor(i,a,d,p.nx,p.ny,p.nz);
}
MF_FN float mf_kinetic(MFHydroStateV2 s) {
    if (s.q[0]==0) return 0;
    float k=0;
    for (unsigned a=0;a<3;++a) k += 0.5f*s.q[1+a]*(s.q[1+a]/s.q[0]);
    return k;
}
struct MFPrimitive {
    float rho, u[3], pressure, temperature, sound, thermal, kinetic;
    unsigned status, auxiliary;
};
MF_FN MFPrimitive mf_primitive(MFHydroStateV2 s, MFHydroParamsV2 p) {
    MFPrimitive r={};
    r.status=MF_PHYSICS_BAD_STATE;
    for (unsigned c=0;c<6;++c) if (!MF_FINITE(s.q[c])) return r;
    if (s.q[0]<0 || s.q[4]<0) return r;
    if (s.q[0]==0) {
        /* True vacuum is exactly (0,0,0,0,0,0). A finite momentum in vacuum
           is invalid, not a reason to manufacture an effective density. */
        for (unsigned c=1;c<6;++c) if (s.q[c]!=0) return r;
        r.status=MF_PHYSICS_OK;
        return r;
    }
    r.rho=s.q[0];
    r.kinetic=mf_kinetic(s);
    if (!MF_FINITE(r.kinetic)) return r;
    float ec=s.q[4]-r.kinetic;
    /* E-K accumulates errors from separately advected rho, momentum, and E;
       its uncertainty is NOT limited to the rounding of this one subtraction.
       Apply the explicit dual-energy reliability band to both signs of E-K.
       A deficit outside that dimensionless band rejects. Inside it, e_aux may
       define thermodynamics, but E stays unchanged and the discrepancy is
       reported. This is a numerical acceptance criterion, not a proof that
       a negative conservative thermal residual is physically exact. */
    const float eps=1.1920928955078125e-7f;
    float reliability_band=mf_max(p.eta_pressure,8*eps)*mf_max(s.q[4],r.kinetic);
    if (ec < -reliability_band) return r;
    bool reliable=ec>p.eta_pressure*s.q[4];
    r.auxiliary=reliable ? 0u : 1u;
    r.thermal=reliable ? ec : s.q[5];
    if (!reliable && r.thermal>s.q[4]+8*eps*mf_max(s.q[4],r.kinetic)) return r;
    if (!(r.thermal>=0) || !MF_FINITE(r.thermal)) return r;
    for (unsigned a=0;a<3;++a) {
        r.u[a]=s.q[1+a]/r.rho;
        if (!MF_FINITE(r.u[a])) return r;
    }
    r.pressure=(p.gamma-1)*r.thermal;
    r.temperature=(r.thermal/r.rho)/p.cv;
    r.sound=MF_SQRT(p.gamma*(r.pressure/r.rho));
    if (!MF_FINITE(r.pressure) || !MF_FINITE(r.temperature) || !MF_FINITE(r.sound)) return r;
    r.status=MF_PHYSICS_OK;
    return r;
}
MF_FN MFHydroStateV2 mf_sync_aux(MFHydroStateV2 s, MFHydroParamsV2 p) {
    if (s.q[0]>0) {
        float ec=s.q[4]-mf_kinetic(s);
        if (ec>p.eta_sync*s.q[4] || (s.q[5]<0 && ec>p.eta_pressure*s.q[4])) s.q[5]=ec;
    }
    /* E_total is NEVER reset to K+e_aux. Its conservative budget survives. */
    return s;
}
MF_FN float mf_mc(float dl, float dr) {
    if (dl==0 || dr==0 || (dl<0)!=(dr<0)) return 0;
    float mag=mf_min(0.5f*mf_abs(dl+dr),mf_min(2*mf_abs(dl),2*mf_abs(dr)));
    return dl<0 ? -mag : mag;
}
MF_FN MFHydroStateV2 mf_edge(MF_PTR const MFHydroStateV2* u, unsigned i,
                            unsigned axis, int side, MFHydroParamsV2 p) {
    MFHydroStateV2 c=u[i];
    if (!p.reconstruction || c.q[0]==0) return c;
    MFHydroStateV2 l=u[mf_hneighbor(i,axis,-1,p)];
    MFHydroStateV2 r=u[mf_hneighbor(i,axis,+1,p)];
    MFHydroStateV2 slope=mf_zero_state();
    for (unsigned k=0;k<6;++k) slope.q[k]=mf_mc(c.q[k]-l.q[k],r.q[k]-c.q[k]);
    /* A single theta limits the whole reconstructed state, not the stored
       averages. Both faces use this same cell-local theta. */
    float theta=1;
    for (unsigned trial=0;trial<25;++trial) {
        MFHydroStateV2 lo=c,hi=c;
        for (unsigned k=0;k<6;++k) {
            lo.q[k]-=0.5f*theta*slope.q[k];
            hi.q[k]+=0.5f*theta*slope.q[k];
        }
        if (mf_primitive(lo,p).status==0 && mf_primitive(hi,p).status==0 && lo.q[5]>=0 && hi.q[5]>=0) {
            return side<0 ? lo : hi;
        }
        theta*=0.5f;
    }
    return c;
}
struct MFFace {
    MFHydroStateV2 flux;
    float normal_velocity, alpha, viscous_momentum[3], viscous_work, conductive_flux;
    unsigned status;
};
MF_FN MFHydroStateV2 mf_euler_flux(MFHydroStateV2 u, MFPrimitive v, unsigned axis) {
    MFHydroStateV2 f=mf_zero_state();
    f.q[0]=u.q[1+axis];
    for (unsigned a=0;a<3;++a) f.q[1+a]=u.q[1+a]*v.u[axis];
    f.q[1+axis]+=v.pressure;
    f.q[4]=(u.q[4]+v.pressure)*v.u[axis];
    f.q[5]=u.q[5]*v.u[axis];
    return f;
}
MF_FN MFFace mf_face(MF_PTR const MFHydroStateV2* u, unsigned il, unsigned axis, MFHydroParamsV2 p) {
    MFFace f={};
    unsigned ir=mf_hneighbor(il,axis,+1,p);
    MFPrimitive cl=mf_primitive(u[il],p), cr=mf_primitive(u[ir],p);
    if (cl.status || cr.status) { f.status=MF_PHYSICS_BAD_STATE; return f; }
    MFHydroStateV2 ul=mf_edge(u,il,axis,+1,p), ur=mf_edge(u,ir,axis,-1,p);
    MFPrimitive l=mf_primitive(ul,p),r=mf_primitive(ur,p);
    if (l.status || r.status) { f.status=MF_PHYSICS_BAD_STATE; return f; }
    MFHydroStateV2 fl=mf_euler_flux(ul,l,axis), fr=mf_euler_flux(ur,r,axis);
    f.alpha=mf_max(mf_abs(l.u[axis])+l.sound,mf_abs(r.u[axis])+r.sound);
    for (unsigned k=0;k<6;++k) f.flux.q[k]=0.5f*(fl.q[k]+fr.q[k])-0.5f*f.alpha*(ur.q[k]-ul.q[k]);
    /* Velocity of the symmetric HLL/LLF intermediate state for pressure work. */
    f.normal_velocity=0;
    if (f.alpha>0) {
        float rs=0.5f*(ul.q[0]+ur.q[0])-(0.5f/f.alpha)*(fr.q[0]-fl.q[0]);
        float ms=0.5f*(ul.q[axis+1]+ur.q[axis+1])-(0.5f/f.alpha)*(fr.q[axis+1]-fl.q[axis+1]);
        if (rs>0) f.normal_velocity=ms/rs;
    }
    bool transport=p.mu>0 || p.bulk_viscosity>0 || p.k_thermal>0;
    if (!transport) return f;
    // Declared free-vacuum boundary: zero traction and zero conductive flux.
    // No temperature or velocity is assigned to empty cells. The inviscid
    // Riemann flux still transports mass/energy across the interface.
    if (cl.rho==0 || cr.rho==0) return f;
    float grad[3][3];
    for (unsigned d=0;d<3;++d) {
        if (d==axis) {
            for (unsigned a=0;a<3;++a) grad[a][d]=(cr.u[a]-cl.u[a])/p.dx;
        } else {
            MFPrimitive lm=mf_primitive(u[mf_hneighbor(il,d,-1,p)],p);
            MFPrimitive lp=mf_primitive(u[mf_hneighbor(il,d,+1,p)],p);
            MFPrimitive rm=mf_primitive(u[mf_hneighbor(ir,d,-1,p)],p);
            MFPrimitive rp=mf_primitive(u[mf_hneighbor(ir,d,+1,p)],p);
            if (lm.status || lp.status || rm.status || rp.status) { f.status=MF_PHYSICS_BAD_STATE; return f; }
            for (unsigned a=0;a<3;++a) {
                // Fluid-side one-sided gradients where the central stencil
                // touches vacuum. Both sides absent gives no resolved slope.
                float dl=lm.rho>0 ? (cl.u[a]-lm.u[a])/p.dx : 0;
                float dr=lp.rho>0 ? (lp.u[a]-cl.u[a])/p.dx : 0;
                float left=(lm.rho>0 && lp.rho>0) ? .5f*(dl+dr) : dl+dr;
                dl=rm.rho>0 ? (cr.u[a]-rm.u[a])/p.dx : 0;
                dr=rp.rho>0 ? (rp.u[a]-cr.u[a])/p.dx : 0;
                float right=(rm.rho>0 && rp.rho>0) ? .5f*(dl+dr) : dl+dr;
                grad[a][d]=.5f*(left+right);
            }
        }
    }
    float div=grad[0][0]+grad[1][1]+grad[2][2];
    for (unsigned a=0;a<3;++a) {
        float tau=p.mu*(grad[a][axis]+grad[axis][a]);
        if (a==axis) tau+=(p.bulk_viscosity-(2.0f/3.0f)*p.mu)*div;
        f.viscous_momentum[a]=tau;
        f.flux.q[1+a]-=tau;
        f.viscous_work+=0.5f*(cl.u[a]+cr.u[a])*tau;
    }
    float heat=p.k_thermal*(cr.temperature-cl.temperature)/p.dx;
    f.conductive_flux=heat;
    f.flux.q[4]-=f.viscous_work+heat;
    f.flux.q[5]-=heat;
    return f;
}
struct MFRhs {
    MFHydroStateV2 derivative;
    float rate, viscous_heating, conductive_source;
    unsigned status;
};
MF_FN MFRhs mf_hydro_rhs(MF_PTR const MFHydroStateV2* u,
                        MF_PTR const float* acceleration, unsigned i, MFHydroParamsV2 p) {
    MFRhs r={};
    if (!mf_hydro_params_valid(p)) { r.status=MF_PHYSICS_BAD_PARAMETERS; return r; }
    MFPrimitive c=mf_primitive(u[i],p);
    if (c.status) { r.status=c.status; return r; }
    /* Validate the entire reconstruction/transport stencil before any limiters
       or max operations could mask a non-finite neighbor. */
    for (unsigned a=0;a<3;++a) for (int sign=-1;sign<=1;sign+=2) {
        unsigned j=mf_hneighbor(i,a,sign,p);
        if (mf_primitive(u[j],p).status) { r.status=MF_PHYSICS_BAD_STATE; return r; }
        if (p.reconstruction) {
            unsigned k=mf_hneighbor(j,a,sign,p);
            if (mf_primitive(u[k],p).status) { r.status=MF_PHYSICS_BAD_STATE; return r; }
        }
    }
    float divu=0,viscwork=0,viscmom[3]={0,0,0};
    float rhomin=c.rho;
    for (unsigned a=0;a<3;++a) {
        unsigned il=mf_hneighbor(i,a,-1,p);
        MFFace lo=mf_face(u,il,a,p),hi=mf_face(u,i,a,p);
        if (lo.status || hi.status) { r.status=lo.status ? lo.status : hi.status; return r; }
        for (unsigned k=0;k<6;++k) r.derivative.q[k]-=(hi.flux.q[k]-lo.flux.q[k])/p.dx;
        divu+=(hi.normal_velocity-lo.normal_velocity)/p.dx;
        for (unsigned b=0;b<3;++b) viscmom[b]+=(hi.viscous_momentum[b]-lo.viscous_momentum[b])/p.dx;
        viscwork+=(hi.viscous_work-lo.viscous_work)/p.dx;
        r.conductive_source+=(hi.conductive_flux-lo.conductive_flux)/p.dx;
        r.rate+=mf_max(lo.alpha,hi.alpha)/p.dx;
        float rl=u[il].q[0],rr=u[mf_hneighbor(i,a,+1,p)].q[0];
        if(rl>0 && rhomin>0)rhomin=mf_min(rhomin,rl);
        if(rr>0 && rhomin>0)rhomin=mf_min(rhomin,rr);
    }
    float heating=viscwork;
    for (unsigned a=0;a<3;++a) heating-=c.u[a]*viscmom[a];
    /* Compatible semidiscrete identity: viscous thermal production equals
       viscous total-energy work minus u dot viscous momentum source. */
    r.viscous_heating=heating;
    r.derivative.q[5]+=-c.pressure*divu+heating;
    if (c.rho>0 && (p.mu>0 || p.bulk_viscosity>0 || p.k_thermal>0)) {
        /* Deliberately conservative explicit stability bound, also accounting
           for cross derivatives. It is a guard, not a positivity theorem. */
        float diffusivity=((2*p.mu+p.bulk_viscosity)+p.k_thermal/p.cv)/rhomin;
        r.rate+=12*diffusivity/(p.dx*p.dx);
    }
    if (p.gravity) for (unsigned a=0;a<3;++a) {
        float g=acceleration[3*i+a];
        if (!MF_FINITE(g)) { r.status=MF_PHYSICS_BAD_STATE; return r; }
        r.derivative.q[1+a]+=u[i].q[0]*g;
        r.derivative.q[4]+=u[i].q[1+a]*g;
    }
    if (!MF_FINITE(r.rate)) { r.status=MF_PHYSICS_BAD_STATE; return r; }
    for (unsigned k=0;k<6;++k) if (!MF_FINITE(r.derivative.q[k])) { r.status=MF_PHYSICS_BAD_STATE; return r; }
    return r;
}
struct MFStepResult { MFHydroStateV2 state; unsigned status; };
MF_FN MFStepResult mf_hydro_stage(MF_PTR const MFHydroStateV2* initial,
                                 MF_PTR const MFHydroStateV2* stage,
                                 MF_PTR const float* acceleration, unsigned i,
                                 MFHydroParamsV2 p,unsigned second) {
    MFStepResult out={};
    MFRhs rhs=mf_hydro_rhs(stage,acceleration,i,p);
    out.status=rhs.status;
    if (out.status) return out;
    if (p.dt*rhs.rate>p.cfl) { out.status=MF_PHYSICS_CFL; return out; }
    for (unsigned k=0;k<6;++k) {
        float candidate=stage[i].q[k]+p.dt*rhs.derivative.q[k];
        out.state.q[k]=second ? 0.5f*initial[i].q[k]+0.5f*candidate : candidate;
    }
    out.state=mf_sync_aux(out.state,p);
    if (mf_primitive(out.state,p).status || out.state.q[5]<0) out.status=MF_PHYSICS_NEGATIVE_UPDATE;
    return out;
}
MF_FN MFHydroDiagnosticV2 mf_hydro_diagnostic(MF_PTR const MFHydroStateV2* u,
                      MF_PTR const float* acceleration,unsigned i, MFHydroParamsV2 p) {
    MFHydroDiagnosticV2 d={};
    MFPrimitive c=mf_primitive(u[i],p);
    d.kinetic_density=c.kinetic;
    d.thermal_disagreement=(u[i].q[4]-c.kinetic)-u[i].q[5];
    d.thermal_fraction=u[i].q[4]>0 ? c.thermal/u[i].q[4] : 0;
    d.used_auxiliary=float(c.auxiliary);
    d.rate=mf_hydro_rhs(u,acceleration,i,p).rate;
    float g[3][3],speed2=0,grad2=0;
    for (unsigned a=0;a<3;++a) {
        speed2+=c.u[a]*c.u[a];
        MFPrimitive l=mf_primitive(u[mf_hneighbor(i,a,-1,p)],p);
        MFPrimitive r=mf_primitive(u[mf_hneighbor(i,a,+1,p)],p);
        for (unsigned b=0;b<3;++b) {
            // No fictitious zero-velocity fluid beyond a traction-free surface.
            g[b][a]=c.rho==0 ? 0.0f : (l.rho>0 && r.rho>0 ? (r.u[b]-l.u[b])/(2*p.dx) :
                (r.rho>0 ? (r.u[b]-c.u[b])/p.dx : (l.rho>0 ? (c.u[b]-l.u[b])/p.dx : 0.0f)));
            grad2+=g[b][a]*g[b][a];
        }
    }
    float wx=g[2][1]-g[1][2],wy=g[0][2]-g[2][0],wz=g[1][0]-g[0][1];
    d.speed=MF_SQRT(speed2); d.gradient_frobenius=MF_SQRT(grad2);
    d.vorticity=MF_SQRT(wx*wx+wy*wy+wz*wz);
    return d;
}

/* Spatial Hamiltonian: H=sum_i dx^3 [hbar^2/(2m) sum_a
   |psi_(i+a)-psi_i|^2/dx^2 + V_i |psi_i|^2 + g/2 |psi_i|^4].
   Each colored bond is a disjoint exact two-site unitary evolution. */
MF_FN bool mf_wave_params_valid(MFWaveParamsV2 p) {
    return mf_grid_valid(p.n,p.nx,p.ny,p.nz) && MF_FINITE(p.dx) && p.dx>0 &&
        MF_FINITE(p.dt) && MF_FINITE(p.hbar) && p.hbar>0 &&
        MF_FINITE(p.mass) && p.mass>0 && MF_FINITE(p.g);
}
MF_FN MFWaveValueV2 mf_wave_phase(MFWaveValueV2 z,float angle) {
    float c=MF_COS(angle),s=MF_SIN(angle);
    MFWaveValueV2 r={z.re*c-z.im*s,z.re*s+z.im*c}; return r;
}
MF_FN MFWaveValueV2 mf_wave_local(MFWaveValueV2 z,float potential,MFWaveParamsV2 p,float dt) {
    if (dt==0) return z;
    float density=z.re*z.re+z.im*z.im;
    return mf_wave_phase(z,-dt*(potential+p.g*density)/p.hbar);
}
MF_FN unsigned mf_bond_color(unsigned edge,unsigned dim) {
    return (dim%2 && edge+1==dim) ? 2u : edge%2;
}
MF_FN MFWaveValueV2 mf_wave_bond(MF_PTR const MFWaveValueV2* psi,unsigned i,
                                 MFWaveParamsV2 p,unsigned axis,unsigned color,float dt) {
    if (dt==0) return psi[i];
    unsigned dims[3]={p.nx,p.ny,p.nz}; unsigned dim=dims[axis];
    if (dim==1) return psi[i];
    unsigned stride=axis==0 ? p.ny*p.nz : (axis==1 ? p.nz : 1u);
    unsigned coordinate=(i/stride)%dim;
    unsigned left_edge=coordinate==0 ? dim-1 : coordinate-1;
    unsigned other=i;
    if (mf_bond_color(coordinate,dim)==color) other=mf_neighbor(i,axis,+1,p.nx,p.ny,p.nz);
    else if (mf_bond_color(left_edge,dim)==color) other=mf_neighbor(i,axis,-1,p.nx,p.ny,p.nz);
    else return psi[i];
    MFWaveValueV2 a=psi[i],b=psi[other];
    MFWaveValueV2 mean={0.5f*a.re+0.5f*b.re,0.5f*a.im+0.5f*b.im};
    MFWaveValueV2 diff={0.5f*a.re-0.5f*b.re,0.5f*a.im-0.5f*b.im};
    diff=mf_wave_phase(diff,-p.hbar*dt/(p.mass*p.dx*p.dx));
    MFWaveValueV2 out={mean.re+diff.re,mean.im+diff.im}; return out;
}
MF_FN MFWaveDiagnosticV2 mf_wave_diagnostic(MF_PTR const MFWaveValueV2* psi,
                     MF_PTR const float* potential,unsigned i,MFWaveParamsV2 p) {
    MFWaveDiagnosticV2 d={}; MFWaveValueV2 a=psi[i];
    d.number_density=a.re*a.re+a.im*a.im;
    d.energy_density=potential[i]*d.number_density+0.5f*p.g*d.number_density*d.number_density;
    float current[3];
    for (unsigned axis=0;axis<3;++axis) {
        MFWaveValueV2 b=psi[mf_neighbor(i,axis,+1,p.nx,p.ny,p.nz)];
        float re=b.re-a.re,im=b.im-a.im;
        d.energy_density+=(p.hbar*p.hbar/(2*p.mass*p.dx*p.dx))*(re*re+im*im);
        current[axis]=(p.hbar/(p.mass*p.dx))*(a.re*b.im-a.im*b.re);
    }
    d.jx=current[0]; d.jy=current[1]; d.jz=current[2]; return d;
}

/* Hertz elastic potential and a specified Rayleigh dissipation law.
   F_H=(4/3) E* sqrt(R*) delta^(3/2); U_H=(2/5)F_H delta.
   F_n=max(0,F_H-tau*(dF_H/ddelta)*v_n); heat=(F_H-F_n)*v_n>=0.
   This is a compliant material law, NOT an impulse/restitution hybrid.
   tau is measured/calibrated; restitution is an outcome, not mis-promised. */
MF_FN bool mf_contact_params_valid(MFContactParamsV2 p) {
    bool domain=(p.domain_x==0 && p.domain_y==0 && p.domain_z==0) ||
        (MF_FINITE(p.domain_x) && MF_FINITE(p.domain_y) && MF_FINITE(p.domain_z) &&
         p.domain_x>4*p.radius && p.domain_y>4*p.radius && p.domain_z>4*p.radius);
    return p.n>0 && MF_FINITE(p.dt) && p.dt>0 && MF_FINITE(p.radius) && p.radius>0 &&
        MF_FINITE(p.young_modulus) && p.young_modulus>0 && MF_FINITE(p.poisson_ratio) &&
        p.poisson_ratio>-1 && p.poisson_ratio<0.5f &&
        mf_finite_nonnegative(p.normal_relaxation_time) && mf_finite_nonnegative(p.conductivity) &&
        MF_FINITE(p.cv) && p.cv>0 && domain;
}
struct MFContactEvaluation { MFContactResultV2 result; unsigned status; };
// One physical pair primitive used by both the reference and hashed traversal.
MF_FN MFContactEvaluation mf_contact_pair(MFParticleStateV2 a,MFParticleStateV2 b,MFContactParamsV2 p) {
    MFContactEvaluation out={};float mi=a.mass,mj=b.mass;
    if(!MF_FINITE(mi)||mi<=0||!MF_FINITE(mj)||mj<=0||!mf_finite_nonnegative(a.heat)||!mf_finite_nonnegative(b.heat)){out.status=MF_PHYSICS_BAD_STATE;return out;}
    float d[3],domain[3]={p.domain_x,p.domain_y,p.domain_z},dist2=0;
    for(unsigned k=0;k<3;++k){if(!MF_FINITE(a.x[k])||!MF_FINITE(b.x[k])||!MF_FINITE(a.v[k])||!MF_FINITE(b.v[k])){out.status=MF_PHYSICS_BAD_STATE;return out;}
        d[k]=a.x[k]-b.x[k];if(domain[k]>0)d[k]-=MF_FLOOR(d[k]/domain[k]+.5f)*domain[k];dist2+=d[k]*d[k];}
    if(dist2>=4*p.radius*p.radius)return out;
    if(dist2==0){out.status=MF_PHYSICS_COINCIDENT_CENTERS;return out;}
    float distance=MF_SQRT(dist2),delta=2*p.radius-distance,vn=0;
    for(unsigned k=0;k<3;++k){d[k]/=distance;vn+=(a.v[k]-b.v[k])*d[k];}
    float ee=p.young_modulus/(2*(1-p.poisson_ratio*p.poisson_ratio)),rr=.5f*p.radius;
    float coeff=(4.f/3.f)*ee*MF_SQRT(rr),elastic=coeff*delta*MF_SQRT(delta),stiffness=1.5f*coeff*MF_SQRT(delta);
    float normal=mf_max(0,elastic-p.normal_relaxation_time*stiffness*vn);
    float loss=(elastic-normal)*vn;
    float conductance=2*p.conductivity*MF_SQRT(rr*delta);
    out.result.dvx=p.dt*normal*d[0]/mi;out.result.dvy=p.dt*normal*d[1]/mi;out.result.dvz=p.dt*normal*d[2]/mi;
    out.result.heat_increment=p.dt*(.5f*loss+conductance*(b.heat/(mj*p.cv)-a.heat/(mi*p.cv)));
    out.result.elastic_energy=.2f*elastic*delta; // each particle owns half U_pair
    out.result.contact_rate=stiffness/mi;out.result.thermal_rate=conductance/(mi*p.cv);
    if(!MF_FINITE(out.result.dvx)||!MF_FINITE(out.result.dvy)||!MF_FINITE(out.result.dvz)||!MF_FINITE(out.result.heat_increment)||!MF_FINITE(out.result.elastic_energy))out.status=MF_PHYSICS_BAD_STATE;
    return out;
}
MF_FN MFContactEvaluation mf_contact_sum(MFContactEvaluation a,MFContactEvaluation b) {
    if(b.status){a.status=b.status;return a;}
    a.result.dvx+=b.result.dvx;a.result.dvy+=b.result.dvy;a.result.dvz+=b.result.dvz;
    a.result.heat_increment+=b.result.heat_increment;a.result.elastic_energy+=b.result.elastic_energy;
    a.result.contact_rate+=b.result.contact_rate;a.result.thermal_rate+=b.result.thermal_rate;return a;
}
MF_FN MFContactEvaluation mf_contact_finish(MFContactEvaluation out,float heat,float crossing_rate,MFContactParamsV2 p){
    float damping_rate=p.normal_relaxation_time*out.result.contact_rate;
    out.result.contact_rate=mf_max(crossing_rate,MF_SQRT(2*out.result.contact_rate)+2*damping_rate);
    if(p.dt*out.result.thermal_rate>.5f||p.dt*out.result.contact_rate>.2f)out.status=MF_PHYSICS_CFL;
    if(!MF_FINITE(out.result.contact_rate)||!MF_FINITE(out.result.thermal_rate)||heat+out.result.heat_increment<0||!MF_FINITE(out.result.heat_increment))out.status=MF_PHYSICS_NEGATIVE_UPDATE;
    return out;
}
MF_FN MFContactEvaluation mf_contact_evaluate(MF_PTR const MFParticleStateV2* state,unsigned i,MFContactParamsV2 p) {
    MFContactEvaluation out={};if(!mf_contact_params_valid(p) || i>=p.n){out.status=MF_PHYSICS_BAD_PARAMETERS;return out;}
    // Validate an isolated particle too: the pair loop may be empty.
    if(!MF_FINITE(state[i].mass)||state[i].mass<=0||!mf_finite_nonnegative(state[i].heat)){out.status=MF_PHYSICS_BAD_STATE;return out;}
    for(unsigned a=0;a<3;++a)if(!MF_FINITE(state[i].x[a])||!MF_FINITE(state[i].v[a])){out.status=MF_PHYSICS_BAD_STATE;return out;}
    float rate=0;for(unsigned j=0;j<p.n;++j)if(i!=j){out=mf_contact_sum(out,mf_contact_pair(state[i],state[j],p));if(out.status)return out;
        float speed2=0;for(unsigned a=0;a<3;++a){float v=state[i].v[a]-state[j].v[a];speed2+=v*v;}rate=mf_max(rate,MF_SQRT(speed2)/p.radius);}
    return mf_contact_finish(out,state[i].heat,rate,p);
}

struct MFContactStepResult { MFParticleStateV2 state; unsigned status; };
MF_FN MFContactStepResult mf_contact_stage(MF_PTR const MFParticleStateV2* initial,
                    MF_PTR const MFParticleStateV2* stage,unsigned i,MFContactParamsV2 p,unsigned second) {
    MFContactStepResult r={};
    MFContactEvaluation eval=mf_contact_evaluate(stage,i,p);
    r.status=eval.status;
    if (r.status) return r;
    r.state=stage[i];
    float dv[3]={eval.result.dvx,eval.result.dvy,eval.result.dvz};
    float domain[3]={p.domain_x,p.domain_y,p.domain_z};
    for (unsigned a=0;a<3;++a) {
        if (p.normal_relaxation_time==0) {
            /* Conservative mechanics uses velocity Verlet, not Heun (whose
               harmonic-oscillator amplification exceeds one for every dt).
               Stage one carries the half-kicked velocity and full new position.
               Conductive heat uses the independent two-stage explicit update. */
            r.state.v[a]=stage[i].v[a]+0.5f*dv[a];
            r.state.x[a]=second ? stage[i].x[a] : initial[i].x[a]+p.dt*r.state.v[a];
        } else {
            r.state.v[a]=second ? 0.5f*initial[i].v[a]+0.5f*(stage[i].v[a]+dv[a]) : stage[i].v[a]+dv[a];
            /* Use the unwrapped initial position in the RK average, so crossing
               a periodic face cannot teleport the midpoint by half a box. */
            r.state.x[a]=second ? initial[i].x[a]+0.5f*p.dt*(initial[i].v[a]+stage[i].v[a]) : initial[i].x[a]+p.dt*stage[i].v[a];
        }
        if (domain[a]>0) r.state.x[a]-=MF_FLOOR(r.state.x[a]/domain[a])*domain[a];
        if (!MF_FINITE(r.state.x[a]) || !MF_FINITE(r.state.v[a])) r.status=MF_PHYSICS_BAD_STATE;
    }
    r.state.heat=second ? 0.5f*initial[i].heat+0.5f*(stage[i].heat+eval.result.heat_increment) : stage[i].heat+eval.result.heat_increment;
    if (!mf_finite_nonnegative(r.state.heat)) r.status=MF_PHYSICS_NEGATIVE_UPDATE;
    return r;
}

#endif
