#include <cfloat>
#include <limits>
/* Shared host contract. Kernels are generated from the same bodies for both GPUs. */
static bool mc_buffers(ManifoldContext* c,std::initializer_list<ManifoldBuffer*> bs) {
    return mf_v2_ready(c,bs) && mf_v2_distinct(bs);
}
extern "C" {
bool manifold_pic_deposit_dual(ManifoldContext* c,ManifoldBuffer* x,ManifoldBuffer* v,ManifoldBuffer* m,ManifoldBuffer* q,
        ManifoldBuffer* out,ManifoldBuffer* flags,unsigned np,const MFHydroParamsV2* pp) {
    if(!pp||!mf_hydro_params_valid(*pp)||np>0x1fffffffu||!mc_buffers(c,{x,v,m,q,out,flags}))return false;
    uint64_t n=np;if(!mf_v2_enough(x,n*12)||!mf_v2_enough(v,n*12)||!mf_v2_enough(m,n*4)||!mf_v2_enough(q,n*4)||
        !mf_v2_enough(out,uint64_t(pp->n)*24)||!mf_v2_enough(flags,n*4))return false;
    manifold_clear_field(c,out);
    if(np && !mc_launch_mc_pic_deposit(c,x,v,m,q,out,flags,np,*pp))return false;
    return mf_v2_status_ok(c,flags,np);
}
bool manifold_pic_gather_dual(ManifoldContext* c,ManifoldBuffer* x,ManifoldBuffer* m,ManifoldBuffer* xo,ManifoldBuffer* vo,
        ManifoldBuffer* qo,ManifoldBuffer* u,ManifoldBuffer* flags,unsigned np,const MFHydroParamsV2* pp) {
    if(!pp||!mf_hydro_params_valid(*pp)||np>0x1fffffffu||!mc_buffers(c,{x,m,xo,vo,qo,u,flags}))return false;
    uint64_t n=np;if(!mf_v2_enough(x,n*12)||!mf_v2_enough(xo,n*12)||!mf_v2_enough(vo,n*12)||!mf_v2_enough(m,n*4)||
        !mf_v2_enough(qo,n*4)||!mf_v2_enough(u,uint64_t(pp->n)*24)||!mf_v2_enough(flags,n*4))return false;
    if(np && !mc_launch_mc_pic_gather(c,x,m,xo,vo,qo,u,flags,np,*pp))return false;
    return mf_v2_status_ok(c,flags,np);
}
bool manifold_hydro_export_dual(ManifoldContext* c,ManifoldBuffer* u,ManifoldBuffer* rho,ManifoldBuffer* mom,
        ManifoldBuffer* e,ManifoldBuffer* flags,const MFHydroParamsV2* pp) {
    if(!pp||!mf_hydro_params_valid(*pp)||!mc_buffers(c,{u,rho,mom,e,flags}))return false;
    uint64_t n=pp->n;if(!mf_v2_enough(u,n*24)||!mf_v2_enough(rho,n*4)||!mf_v2_enough(mom,n*12)||!mf_v2_enough(e,n*4)||!mf_v2_enough(flags,n*4))return false;
    return mc_launch_mc_hydro_export(c,u,rho,mom,e,flags,*pp)&&mf_v2_status_ok(c,flags,pp->n);
}
bool manifold_gravity_kick(ManifoldContext* c,ManifoldBuffer* u,ManifoldBuffer* g,ManifoldBuffer* flags,const MFHydroParamsV2* pp) {
    if(!pp||!mf_hydro_params_valid(*pp)||!mc_buffers(c,{u,g,flags}))return false;
    uint64_t n=pp->n;if(!mf_v2_enough(u,n*24)||!mf_v2_enough(g,n*12)||!mf_v2_enough(flags,n*4))return false;
    return mc_launch_mc_gravity_kick(c,u,g,flags,*pp)&&mf_v2_status_ok(c,flags,pp->n);
}
bool manifold_periodic_poisson(ManifoldContext* c,ManifoldBuffer* u,ManifoldBuffer* spectrum,ManifoldBuffer* phi,
        ManifoldBuffer* g,ManifoldBuffer* flags,const MFHydroParamsV2* pp,float G) {
    if(!pp||!mf_hydro_params_valid(*pp)||!MF_FINITE(G)||G<0||!mc_buffers(c,{u,spectrum,phi,g,flags}))return false;
    uint64_t n=pp->n;if(!mf_v2_enough(u,n*24)||!mf_v2_enough(spectrum,n*8)||!mf_v2_enough(phi,n*4)||!mf_v2_enough(g,n*12)||!mf_v2_enough(flags,n*4))return false;
    auto p=*pp;uint64_t words=0;
    for(unsigned a=0;a<3;++a){unsigned len=mc_line_length(a,p),w=mr_workspace(len);if(!w)return false;words=std::max(words,uint64_t(p.n/len)*w);}
    if(words>UINT32_MAX || words>SIZE_MAX/sizeof(MRComplex))return false;
    ManifoldBuffer* work=manifold_create_buffer(c,words*sizeof(MRComplex),nullptr);if(!work)return false;
    bool ok=mc_launch_mc_poisson_init(c,u,spectrum,flags,p,G)&&mf_v2_status_ok(c,flags,p.n);
    for(unsigned a=0;a<3 && ok;++a)if(mc_line_length(a,p)>1)
        ok=mc_launch_mc_poisson_axis(c,spectrum,work,p,a,-1,mr_workspace(mc_line_length(a,p)));
    if(ok)ok=mc_launch_mc_poisson_spectral(c,spectrum,flags,p);
    for(int a=2;a>=0 && ok;--a)if(mc_line_length(unsigned(a),p)>1)
        ok=mc_launch_mc_poisson_axis(c,spectrum,work,p,unsigned(a),+1,mr_workspace(mc_line_length(unsigned(a),p)));
    if(ok)ok=mc_launch_mc_poisson_export(c,spectrum,phi,g,flags,p)&&mf_v2_status_ok(c,flags,p.n);
    // Ensure no queued consumer outlives the temporary line workspace on failure.
    mf_v2_sync(c);manifold_destroy_buffer(work);return ok;
}
}

extern "C" bool manifold_hydro_budget(ManifoldContext* ctx,ManifoldBuffer* initial,ManifoldBuffer* stage,ManifoldBuffer* out,ManifoldBuffer* flags,const MFHydroParamsV2* pp){
    if(!pp||!mf_hydro_params_valid(*pp)||pp->gravity||!mf_v2_ready(ctx,{initial,stage,out,flags})||!mf_v2_distinct({initial,stage,out,flags}))return false;
    auto p=*pp;uint64_t n=p.n;
    if(n>0x1fffffffu||!mf_v2_enough(initial,n*24)||!mf_v2_enough(stage,n*24)||!mf_v2_enough(out,n*8)||!mf_v2_enough(flags,n*4))return false;
    return mc_launch_mc_hydro_budget(ctx,initial,stage,out,flags,p)&&mf_v2_status_ok(ctx,flags,p.n);
}

extern "C" bool manifold_pic_deposit_material(ManifoldContext* c,ManifoldBuffer* x,ManifoldBuffer* v,ManifoldBuffer* m,
    ManifoldBuffer* q,ManifoldBuffer* e,ManifoldBuffer* out,ManifoldBuffer* flags,unsigned np,const MFHydroParamsV2* pp){
    if(!pp||!mf_hydro_params_valid(*pp)||!np||np>0x1fffffffu||!mc_buffers(c,{x,v,m,q,e,out,flags}))return false;
    uint64_t n=np;if(!mf_v2_enough(x,n*12)||!mf_v2_enough(v,n*12)||!mf_v2_enough(m,n*4)||!mf_v2_enough(q,n*4)||!mf_v2_enough(e,n*4)||!mf_v2_enough(out,uint64_t(pp->n)*24)||!mf_v2_enough(flags,n*4))return false;
    manifold_clear_field(c,out);
    return mc_launch_mc_pic_deposit_material(c,x,v,m,q,e,out,flags,np,*pp)&&mf_v2_status_ok(c,flags,np);
}
// Candidate-only API: no input is mutated. Callers commit output buffers ONLY
// on success. The actual workspace additionally snapshots the complete substep.
extern "C" bool manifold_pic_remap_conservative(ManifoldContext* c,ManifoldBuffer* u,ManifoldBuffer* x,ManifoldBuffer* m,
    ManifoldBuffer* vo,ManifoldBuffer* qo,ManifoldBuffer* eo,ManifoldBuffer* report,ManifoldBuffer* flags,
    unsigned np,const MFHydroParamsV2* pp,float width,float tolerance,unsigned iterations,float gravity_G){
    if(!pp||!mf_hydro_params_valid(*pp)||!MF_FINITE(gravity_G)||gravity_G<0||!np||np>0x1fffffffu||!MF_FINITE(width)||width<=0||
       !MF_FINITE(tolerance)||tolerance<8*1.1920928955078125e-7f||tolerance>1e-3f||!iterations||
       !mc_buffers(c,{u,x,m,vo,qo,eo,report,flags}))return false;
    auto p=*pp;uint64_t n=np;
    if(!mf_v2_enough(u,uint64_t(p.n)*24)||!mf_v2_enough(x,n*12)||!mf_v2_enough(m,n*4)||
       !mf_v2_enough(vo,n*12)||!mf_v2_enough(qo,n*4)||!mf_v2_enough(eo,n*4)||!mf_v2_enough(report,48)||!mf_v2_enough(flags,n*4))return false;
    if(!mf_v2_sync(c))return false;
    auto state=static_cast<const MFHydroStateV2*>(manifold_get_buffer_pointer(u));
    auto mass=static_cast<const float*>(manifold_get_buffer_pointer(m));
    auto pos=static_cast<const float*>(manifold_get_buffer_pointer(x));
    auto diag=static_cast<float*>(manifold_get_buffer_pointer(report));
    auto fs=static_cast<unsigned*>(manifold_get_buffer_pointer(flags));
    if(!state||!mass||!pos||!diag||!fs)return false;
    double cells_mass=0,particles_mass=0,grid_e=0,grid_p[3]={},abs_p[3]={};
    double vol=double(p.dx)*p.dx*p.dx;
    for(unsigned i=0;i<np;++i){
        if(!MF_FINITE(mass[i])||mass[i]<=0){fs[i]=MF_PHYSICS_BAD_STATE;return false;}
        for(unsigned a=0;a<3;++a)if(!MF_FINITE(pos[3*i+a])){fs[i]=MF_PHYSICS_BAD_STATE;return false;}
        particles_mass+=mass[i];fs[i]=0;
    }
    for(unsigned j=0;j<p.n;++j){if(mf_primitive(state[j],p).status){fs[0]=MF_PHYSICS_BAD_STATE;return false;}
        cells_mass+=double(state[j].q[0])*vol;grid_e+=double(state[j].q[4])*vol;
        for(unsigned a=0;a<3;++a){double value=double(state[j].q[a+1])*vol;grid_p[a]+=value;abs_p[a]+=std::abs(value);}}
    for(unsigned i=0;i<12;++i)diag[i]=0;
    // Balance only the scalar mass discrepancy of floating point deposition/FV
    // conservation. A physically different total mass is a fatal input error.
    if(!(cells_mass>0)||std::abs(cells_mass-particles_mass)>double(tolerance)*particles_mass){fs[0]=MF_PHYSICS_BAD_STATE;return false;}
    float scale=float(particles_mass/cells_mass);diag[2]=scale-1;
    ManifoldBuffer* rows=manifold_create_buffer(c,uint64_t(p.n)*4,nullptr);
    ManifoldBuffer* cols=manifold_create_buffer(c,n*4,nullptr);
    ManifoldBuffer* errors=manifold_create_buffer(c,uint64_t(p.n)*4,nullptr);
    ManifoldBuffer* cellflags=manifold_create_buffer(c,uint64_t(p.n)*4,nullptr);
    ManifoldBuffer* mix=manifold_create_buffer(c,n*4,nullptr);
    bool ok=rows&&cols&&errors&&cellflags&&mix,converged=false;
    if(ok){manifold_clear_field(c,cols);manifold_clear_field(c,cellflags);}
    for(unsigned it=0;it<iterations&&ok;++it){
        ok=mc_launch_mc_remap_rows(c,u,x,cols,rows,cellflags,np,p,width,scale)&&
           mc_launch_mc_remap_columns(c,u,x,m,rows,cols,flags,np,p,width);
        if((it+1)%8==0||it+1==iterations){
            ok=ok&&mc_launch_mc_remap_residual(c,u,x,rows,cols,errors,cellflags,np,p,width,scale)&&
               mf_v2_status_ok(c,cellflags,p.n)&&mf_v2_status_ok(c,flags,np);
            if(!ok)break;
            float worst=0;auto er=static_cast<const float*>(manifold_get_buffer_pointer(errors));
            if(!er){ok=false;break;}
            for(unsigned j=0;j<p.n;++j)worst=std::max(worst,er[j]);
            diag[0]=worst;diag[1]=float(it+1);
            if(worst<=tolerance){converged=true;break;}
        }
    }
    if(ok&&converged)ok=mc_launch_mc_remap_gather(c,u,x,m,rows,vo,qo,eo,mix,flags,np,p,width,scale)&&mf_v2_status_ok(c,flags,np);
    double gravity_work=0;
    if(ok&&converged&&gravity_G>0){
        ManifoldBuffer* remapped=manifold_create_buffer(c,uint64_t(p.n)*24,nullptr);
        ManifoldBuffer* spectral=manifold_create_buffer(c,uint64_t(p.n)*8,nullptr);
        ManifoldBuffer* phi0=manifold_create_buffer(c,uint64_t(p.n)*4,nullptr);
        ManifoldBuffer* phi1=manifold_create_buffer(c,uint64_t(p.n)*4,nullptr);
        ManifoldBuffer* accel=manifold_create_buffer(c,uint64_t(p.n)*12,nullptr);
        ManifoldBuffer* work=manifold_create_buffer(c,n*4,nullptr);
        ok=remapped&&spectral&&phi0&&phi1&&accel&&work;
        ok=ok&&manifold_pic_deposit_material(c,x,vo,m,qo,eo,remapped,flags,np,&p)&&
           manifold_periodic_poisson(c,u,spectral,phi0,accel,cellflags,&p,gravity_G)&&
           manifold_periodic_poisson(c,remapped,spectral,phi1,accel,cellflags,&p,gravity_G)&&
           mc_launch_mc_gravity_remap(c,u,x,m,rows,phi0,phi1,eo,work,flags,np,p,width)&&mf_v2_status_ok(c,flags,np);
        if(ok){
            auto a=static_cast<const float*>(manifold_get_buffer_pointer(phi0));
            auto b=static_cast<const float*>(manifold_get_buffer_pointer(phi1));
            auto u1=static_cast<const MFHydroStateV2*>(manifold_get_buffer_pointer(remapped));
            auto w=static_cast<const float*>(manifold_get_buffer_pointer(work));
            double oldU=0,newU=0;for(unsigned j=0;j<p.n;++j){unsigned k=mc_display_index(j,p);oldU+=.5*vol*state[j].q[0]*a[k];newU+=.5*vol*u1[j].q[0]*b[k];}
            for(unsigned i=0;i<np;++i)gravity_work+=w[i];
            diag[8]=float(gravity_work);diag[9]=float(oldU);diag[10]=float(newU);diag[11]=float(gravity_work+newU-oldU);
            // Finite marginal, Poisson and float addition errors all contribute.
            // This is an acceptance gate, not a redistribution of its residual.
            if(std::abs(gravity_work+newU-oldU)>16*double(tolerance)*(std::abs(oldU)+std::abs(newU)+std::abs(grid_e)))ok=false;
        }
        mf_v2_sync(c);for(auto z:{remapped,spectral,phi0,phi1,accel,work})if(z)manifold_destroy_buffer(z);
    }
    // No diagnostic-only declaration of conservation: reject outputs that do
    // not satisfy the actual integrated momentum/total-energy constraints.
    if(ok&&converged){
        auto v=static_cast<const float*>(manifold_get_buffer_pointer(vo));auto e=static_cast<const float*>(manifold_get_buffer_pointer(eo));
        auto heatmix=static_cast<const float*>(manifold_get_buffer_pointer(mix));double et=0,pt[3]={},mixed=0;
        for(unsigned i=0;i<np;++i){et+=e[i];mixed+=heatmix[i];for(unsigned a=0;a<3;++a)pt[a]+=double(mass[i])*v[3*i+a];}
        diag[3]=float(mixed);diag[4]=float(et-grid_e-gravity_work);
        double tol=8*double(tolerance);
        if(std::abs(et-grid_e-gravity_work)>tol*std::max(std::abs(grid_e),1e-30))ok=false;
        for(unsigned a=0;a<3;++a){diag[5+a]=float(pt[a]-grid_p[a]);if(std::abs(pt[a]-grid_p[a])>tol*std::max(abs_p[a],1e-30))ok=false;}
    }
    // Preserve the cause of failure. A bad state/parameter is not a CFL
    // rejection, and a failed stream synchronization must not touch host views.
    bool synchronized=mf_v2_sync(c);
    unsigned failure=MF_PHYSICS_OK;
    if(synchronized){
        for(unsigned i=0;i<np;++i)if(fs[i]){failure=fs[i];break;}
        if(rows&&cols&&errors&&cellflags&&mix){
            auto cf=static_cast<const unsigned*>(manifold_get_buffer_pointer(cellflags));
            if(cf)for(unsigned j=0;j<p.n;++j)if(cf[j]){failure=cf[j];break;}
        }
        if((!ok||!converged)&&failure==MF_PHYSICS_OK)failure=MF_PHYSICS_CFL;
        if(failure)fs[0]=failure;
    }
    for(auto b:{rows,cols,errors,cellflags,mix})if(b)manifold_destroy_buffer(b);
    return synchronized&&ok&&converged&&failure==MF_PHYSICS_OK;
}

extern "C" bool manifold_pilot_checked(ManifoldContext* c,ManifoldBuffer* x,ManifoldBuffer* m,ManifoldBuffer* prior,
    ManifoldBuffer* re,ManifoldBuffer* im,ManifoldBuffer* xo,ManifoldBuffer* guide,ManifoldBuffer* report,ManifoldBuffer* flags,
    unsigned particles,const MFHydroParamsV2* pp,float hbar,float tolerance,float maxcells){
    if(!pp||!mf_hydro_params_valid(*pp)||!MF_FINITE(hbar)||hbar<=0||!MF_FINITE(tolerance)||tolerance<8*FLT_EPSILON||tolerance>.01f||
       !MF_FINITE(maxcells)||maxcells<=0||maxcells>.5f||!mc_buffers(c,{x,m,prior,re,im,xo,guide,report,flags}))return false;
    uint64_t n=particles;if(!mf_v2_enough(x,12*n)||!mf_v2_enough(m,4*n)||!mf_v2_enough(prior,12*n)||!mf_v2_enough(xo,12*n)||
       !mf_v2_enough(guide,12*n)||!mf_v2_enough(report,16*n)||!mf_v2_enough(flags,4*n)||!mf_v2_enough(re,4ull*pp->n)||!mf_v2_enough(im,4ull*pp->n))return false;
    if(!particles)return true;
    return mc_launch_mc_pilot_checked_step(c,x,m,prior,re,im,xo,guide,report,flags,particles,*pp,hbar,tolerance,maxcells)&&mf_v2_status_ok(c,flags,particles);
}
// rates_only evaluates stiffness and thermal rates without advancing state.
// Cell topology is rebuilt at the current positions before each kick; a frozen
// position kick cannot invalidate the list. All scratch has stream-safe lifetime.
extern "C" bool manifold_contact_hash_kick(ManifoldContext* ctx,ManifoldBuffer* pos,ManifoldBuffer* vel,ManifoldBuffer* mass,
    ManifoldBuffer* heat,ManifoldBuffer* vel_out,ManifoldBuffer* heat_out,ManifoldBuffer* report,ManifoldBuffer* flags,
    const MFHydroParamsV2* gp,const MFContactParamsV2* cp,unsigned rates_only){
    if(!gp||!cp||!mf_hydro_params_valid(*gp)||!mf_contact_params_valid(*cp)||rates_only>1||
       !mc_buffers(ctx,{pos,vel,mass,heat,vel_out,heat_out,report,flags}))return false;
    auto p=*gp;auto c=*cp;uint64_t n=c.n,cells=p.n;
    if(n>0x1fffffffu||!mf_v2_enough(pos,n*12)||!mf_v2_enough(vel,n*12)||!mf_v2_enough(mass,n*4)||!mf_v2_enough(heat,n*4)||
       !mf_v2_enough(vel_out,n*12)||!mf_v2_enough(heat_out,n*4)||!mf_v2_enough(report,n*28)||!mf_v2_enough(flags,n*4))return false;
    float domains[3]={c.domain_x,c.domain_y,c.domain_z},extent[3]={p.nx*p.dx,p.ny*p.dx,p.nz*p.dx};
    for(unsigned a=0;a<3;++a)if(std::abs(domains[a]-extent[a])>8*FLT_EPSILON*extent[a])return false;
    if(!mf_v2_sync(ctx))return false;
    auto v=static_cast<const float*>(manifold_get_buffer_pointer(vel));if(!v)return false;
    double vmin[3]={v[0],v[1],v[2]},vmax[3]={v[0],v[1],v[2]};
    for(unsigned i=0;i<c.n;++i)for(unsigned a=0;a<3;++a){if(!MF_FINITE(v[3*i+a]))return false;vmin[a]=std::min(vmin[a],double(v[3*i+a]));vmax[a]=std::max(vmax[a],double(v[3*i+a]));}
    double vsq=0;for(unsigned a=0;a<3;++a)vsq+=(vmax[a]-vmin[a])*(vmax[a]-vmin[a]);
    float speed=float(std::sqrt(vsq));if(!MF_FINITE(speed))return false;
    if(rates_only)c.dt=std::numeric_limits<float>::min();
    ManifoldBuffer* initial=manifold_create_buffer(ctx,n*32,nullptr);
    ManifoldBuffer* trial=manifold_create_buffer(ctx,n*32,nullptr);
    ManifoldBuffer* out=manifold_create_buffer(ctx,n*32,nullptr);
    ManifoldBuffer* cell=manifold_create_buffer(ctx,n*4,nullptr);
    ManifoldBuffer* count=manifold_create_buffer(ctx,cells*4,nullptr);
    ManifoldBuffer* starts=manifold_create_buffer(ctx,(cells+1)*4,nullptr);
    ManifoldBuffer* offset=manifold_create_buffer(ctx,cells*4,nullptr);
    ManifoldBuffer* sorted=manifold_create_buffer(ctx,n*4,nullptr);
    bool ok=initial&&trial&&out&&cell&&count&&starts&&offset&&sorted;
    if(ok){manifold_clear_field(ctx,count);manifold_clear_field(ctx,offset);}
    ok=ok&&mc_launch_mc_contact_pack(ctx,pos,vel,mass,heat,initial,cell,count,flags,c.n,p)&&mf_v2_status_ok(ctx,flags,c.n)&&
       manifold_exclusive_scan_u32(ctx,count,starts,p.n)&&
       mc_launch_mc_contact_sort(ctx,cell,starts,offset,sorted,flags,c.n,p)&&mf_v2_status_ok(ctx,flags,c.n)&&
       mc_launch_mc_contact_hash_kick(ctx,initial,initial,starts,sorted,trial,report,flags,c.n,p,c,0,speed)&&mf_v2_status_ok(ctx,flags,c.n);
    if(ok&&!rates_only){
        // Update relative-speed bound for the predictor. No list rebuild needed:
        // this operator has not moved any particle.
        auto s=static_cast<const MFParticleStateV2*>(manifold_get_buffer_pointer(trial));
        for(unsigned a=0;a<3;++a)vmin[a]=vmax[a]=s[0].v[a];
        for(unsigned i=0;i<c.n;++i)for(unsigned a=0;a<3;++a){vmin[a]=std::min(vmin[a],double(s[i].v[a]));vmax[a]=std::max(vmax[a],double(s[i].v[a]));}
        vsq=0;for(unsigned a=0;a<3;++a)vsq+=(vmax[a]-vmin[a])*(vmax[a]-vmin[a]);speed=float(std::sqrt(vsq));
        ok=mc_launch_mc_contact_hash_kick(ctx,initial,trial,starts,sorted,out,report,flags,c.n,p,c,1,speed)&&mf_v2_status_ok(ctx,flags,c.n)&&
           mc_launch_mc_contact_unpack(ctx,out,vel_out,heat_out,c.n,p)&&mf_v2_sync(ctx);
    }
    mf_v2_sync(ctx);for(auto b:{initial,trial,out,cell,count,starts,offset,sorted})if(b)manifold_destroy_buffer(b);
    return ok;
}

extern "C" bool manifold_gravity_compatible(ManifoldContext* ctx,ManifoldBuffer* initial,ManifoldBuffer* stage,ManifoldBuffer* state,
 ManifoldBuffer* phi0,ManifoldBuffer* phi1,ManifoldBuffer* kick,ManifoldBuffer* out,ManifoldBuffer* flags,const MFHydroParamsV2* pp){
 if(!pp||!mf_hydro_params_valid(*pp)||!mc_buffers(ctx,{initial,stage,state,phi0,phi1,kick,out,flags}))return false;
 uint64_t n=pp->n;if(!mf_v2_enough(initial,n*24)||!mf_v2_enough(stage,n*24)||!mf_v2_enough(state,n*24)||!mf_v2_enough(out,n*24)||
 !mf_v2_enough(phi0,n*4)||!mf_v2_enough(phi1,n*4)||!mf_v2_enough(kick,n*4)||!mf_v2_enough(flags,n*4))return false;
 return mc_launch_mc_gravity_compatible(ctx,initial,stage,state,phi0,phi1,kick,out,flags,*pp)&&mf_v2_status_ok(ctx,flags,pp->n);
}

// Both source and anchor impulses are accumulated. The caller clears force
// once before a multi-head interaction; the per-head occupations already
// include domega and the two conservative half-step quadratures.
extern "C" bool manifold_coherence_reciprocal(ManifoldContext* c,
    ManifoldBuffer* x,ManifoldBuffer* previous_x,ManifoldBuffer* amp,ManifoldBuffer* omega,
    ManifoldBuffer* mode_omega,ManifoldBuffer* linewidth,ManifoldBuffer* indices,ManifoldBuffer* weights,
    ManifoldBuffer* old_re,ManifoldBuffer* old_im,ManifoldBuffer* ledger,ManifoldBuffer* force,
    ManifoldBuffer* reference_potential,ManifoldBuffer* flags,
    unsigned np,unsigned modes,unsigned anchors,float sigma,float domega,const MFHydroParamsV2* pp) {
    if(!pp||!mf_hydro_params_valid(*pp)||!np||!modes||!anchors||anchors>32||
       !std::isfinite(sigma)||sigma<0||!std::isfinite(domega)||domega<=0||
       np>0x1fffffffu||modes>0x1fffffffu||
       !mc_buffers(c,{x,previous_x,amp,omega,mode_omega,linewidth,indices,weights,old_re,old_im,ledger,force,reference_potential,flags}))return false;
    uint64_t n=np,m=modes;
    if(!mf_v2_enough(x,n*12)||!mf_v2_enough(previous_x,n*12)||!mf_v2_enough(force,n*12)||
       !mf_v2_enough(amp,n*4)||!mf_v2_enough(omega,n*4)||!mf_v2_enough(mode_omega,m*4)||
       !mf_v2_enough(linewidth,m*4)||!mf_v2_enough(indices,m*anchors*4)||!mf_v2_enough(weights,m*anchors*4)||
       !mf_v2_enough(old_re,m*4)||!mf_v2_enough(old_im,m*4)||!mf_v2_enough(ledger,m*24)||
       !mf_v2_enough(reference_potential,m*4)||!mf_v2_enough(flags,(n>m?n:m)*4))return false;
    ManifoldBuffer* occupation=manifold_create_buffer(c,m*4,nullptr);if(!occupation)return false;
    bool ok=mc_launch_mc_reciprocal_occupation(c,old_re,old_im,ledger,occupation,flags,modes,domega,*pp)&&mf_v2_status_ok(c,flags,modes);
    ok=ok&&mc_launch_mc_reciprocal_potential(c,previous_x,amp,omega,mode_omega,linewidth,indices,weights,reference_potential,flags,np,modes,anchors,sigma,*pp)&&mf_v2_status_ok(c,flags,modes);
    ok=ok&&mc_launch_mc_reciprocal_force(c,x,amp,omega,mode_omega,linewidth,indices,weights,occupation,force,flags,np,modes,anchors,sigma,*pp)&&mf_v2_status_ok(c,flags,np);
    bool synced=manifold_synchronize_checked(c);manifold_destroy_buffer(occupation);return ok&&synced;
}

extern "C" bool manifold_pilot_checked_time(ManifoldContext* c,ManifoldBuffer* x,ManifoldBuffer* m,ManifoldBuffer* prior,
    ManifoldBuffer* re,ManifoldBuffer* im,ManifoldBuffer* re1,ManifoldBuffer* im1,ManifoldBuffer* xo,ManifoldBuffer* guide,ManifoldBuffer* report,ManifoldBuffer* flags,
    unsigned particles,const MFHydroParamsV2* pp,float hbar,float tolerance,float maxcells){
    if(!pp||!mf_hydro_params_valid(*pp)||!MF_FINITE(hbar)||hbar<=0||!MF_FINITE(tolerance)||tolerance<8*FLT_EPSILON||tolerance>.01f||
       !MF_FINITE(maxcells)||maxcells<=0||maxcells>.5f||particles>0x1fffffffu||!mc_buffers(c,{x,m,prior,re,im,re1,im1,xo,guide,report,flags}))return false;
    uint64_t n=particles;if(!mf_v2_enough(x,12*n)||!mf_v2_enough(m,4*n)||!mf_v2_enough(prior,12*n)||!mf_v2_enough(xo,12*n)||
       !mf_v2_enough(guide,12*n)||!mf_v2_enough(report,16*n)||!mf_v2_enough(flags,4*n)||!mf_v2_enough(re,4ull*pp->n)||!mf_v2_enough(im,4ull*pp->n)||!mf_v2_enough(re1,4ull*pp->n)||!mf_v2_enough(im1,4ull*pp->n))return false;
    if(!particles)return true;
    return mc_launch_mc_pilot_checked_time_step(c,x,m,prior,re,im,re1,im1,xo,guide,report,flags,particles,*pp,hbar,tolerance,maxcells)&&mf_v2_status_ok(c,flags,particles);
}
