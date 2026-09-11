/* Included after backend-specific checked launch/synchronization adapters.
   This SAME transactional schedule is exercised by the CPU regression tests. */
static bool mf_v2_enough(ManifoldBuffer* b,uint64_t bytes) {
    return b && bytes<=b->size_bytes;
}
static bool mf_v2_distinct(std::initializer_list<ManifoldBuffer*> list) {
    for (auto a=list.begin();a!=list.end();++a)
        for(auto b=a+1;b!=list.end();++b) if(*a==*b) return false;
    return true;
}
static bool mf_v2_workspace(ManifoldContext* ctx,ManifoldBuffer* in,ManifoldBuffer* out,
                            ManifoldBuffer* w1,ManifoldBuffer* w2,ManifoldBuffer* status,
                            unsigned n,uint64_t stride) {
    if (!n || n>0x1fffffffu || !mf_v2_ready(ctx,{in,out,w1,w2,status})) return false;
    if(!mf_v2_distinct({in,w1,w2,status}) || !mf_v2_distinct({out,w1,w2,status})) return false;
    uint64_t bytes=uint64_t(n)*stride;
    return mf_v2_enough(in,bytes) && mf_v2_enough(out,bytes) && mf_v2_enough(w1,bytes) &&
           mf_v2_enough(w2,bytes) && mf_v2_enough(status,uint64_t(n)*sizeof(unsigned));
}
extern "C" {
unsigned manifold_physics_abi_version(void) { return MF_PHYSICS_ABI_VERSION; }
bool manifold_hydro_pack_v2(ManifoldContext* ctx,ManifoldBuffer* rho,ManifoldBuffer* mom,
                ManifoldBuffer* e,ManifoldBuffer* state,ManifoldBuffer* status,const MFHydroParamsV2* pp) {
    if(!pp || !mf_hydro_params_valid(*pp) || !mf_v2_ready(ctx,{rho,mom,e,state,status})) return false;
    auto p=*pp; uint64_t n=p.n;
    if(n>0x1fffffffu || !mf_v2_enough(rho,n*4) || !mf_v2_enough(mom,n*12) ||
       !mf_v2_enough(e,n*4) || !mf_v2_enough(state,n*sizeof(MFHydroStateV2)) || !mf_v2_enough(status,n*4) ||
       !mf_v2_distinct({state,status,rho}) || !mf_v2_distinct({state,status,mom}) || !mf_v2_distinct({state,status,e})) return false;
    return mf_launch_mf_hydro_pack_v2(ctx,rho,mom,e,state,status,p) && mf_v2_status_ok(ctx,status,p.n);
}
bool manifold_hydro_unpack_v2(ManifoldContext* ctx,ManifoldBuffer* state,ManifoldBuffer* rho,
                ManifoldBuffer* mom,ManifoldBuffer* e,ManifoldBuffer* status,const MFHydroParamsV2* pp) {
    if(!pp || !mf_hydro_params_valid(*pp) || !mf_v2_ready(ctx,{state,rho,mom,e,status})) return false;
    auto p=*pp; uint64_t n=p.n;
    if(n>0x1fffffffu || !mf_v2_enough(rho,n*4) || !mf_v2_enough(mom,n*12) || !mf_v2_enough(e,n*4) ||
       !mf_v2_enough(state,n*sizeof(MFHydroStateV2)) || !mf_v2_enough(status,n*4) ||
       !mf_v2_distinct({state,rho,mom,e,status})) return false;
    return mf_launch_mf_hydro_unpack_v2(ctx,state,rho,mom,e,status,p) && mf_v2_status_ok(ctx,status,p.n);
}
bool manifold_hydro_step_v2(ManifoldContext* ctx,ManifoldBuffer* in,ManifoldBuffer* out,
                ManifoldBuffer* w1,ManifoldBuffer* w2,ManifoldBuffer* status,
                ManifoldBuffer* acceleration,const MFHydroParamsV2* pp) {
    if(!pp || !mf_hydro_params_valid(*pp)) return false;
    auto p=*pp;
    if(!mf_v2_workspace(ctx,in,out,w1,w2,status,p.n,sizeof(MFHydroStateV2))) return false;
    if(p.gravity && (!mf_v2_ready(ctx,{acceleration}) || !mf_v2_enough(acceleration,uint64_t(p.n)*12) ||
       acceleration==in || acceleration==out || acceleration==w1 || acceleration==w2 || acceleration==status)) return false;
    /* Bind a valid unused buffer when gravity is off; never dereferenced. */
    ManifoldBuffer* g=p.gravity ? acceleration : in;
    if(!mf_launch_mf_hydro_stage_v2(ctx,in,in,g,w1,status,p,0u) || !mf_v2_status_ok(ctx,status,p.n)) return false;
    if(!mf_launch_mf_hydro_stage_v2(ctx,in,w1,g,w2,status,p,1u) || !mf_v2_status_ok(ctx,status,p.n)) return false;
    return mf_launch_mf_hydro_copy_v2(ctx,w2,out,p.n) && mf_v2_sync(ctx);
}
bool manifold_hydro_diagnostics_v2(ManifoldContext* ctx,ManifoldBuffer* in,ManifoldBuffer* acceleration,
                ManifoldBuffer* diag,ManifoldBuffer* status,const MFHydroParamsV2* pp) {
    if(!pp || !mf_hydro_params_valid(*pp) || !mf_v2_ready(ctx,{in,diag,status})) return false;
    auto p=*pp;
    if(p.n>0x1fffffffu || !mf_v2_enough(in,uint64_t(p.n)*sizeof(MFHydroStateV2)) ||
       !mf_v2_enough(diag,uint64_t(p.n)*sizeof(MFHydroDiagnosticV2)) || !mf_v2_enough(status,uint64_t(p.n)*4) ||
       !mf_v2_distinct({in,diag,status})) return false;
    if(p.gravity && (!mf_v2_ready(ctx,{acceleration}) || !mf_v2_enough(acceleration,uint64_t(p.n)*12) ||
       !mf_v2_distinct({acceleration,in,diag,status}))) return false;
    return mf_launch_mf_hydro_diagnostics_v2(ctx,in,p.gravity ? acceleration : in,diag,status,p) && mf_v2_status_ok(ctx,status,p.n);
}
bool manifold_wave_step_v2(ManifoldContext* ctx,ManifoldBuffer* in,ManifoldBuffer* out,
                ManifoldBuffer* w1,ManifoldBuffer* w2,ManifoldBuffer* potential,
                ManifoldBuffer* status,const MFWaveParamsV2* pp) {
    if(!pp || !mf_wave_params_valid(*pp)) return false;
    auto p=*pp;
    if(!mf_v2_workspace(ctx,in,out,w1,w2,status,p.n,sizeof(MFWaveValueV2)) ||
       !mf_v2_ready(ctx,{potential}) || !mf_v2_enough(potential,uint64_t(p.n)*4) ||
       potential==in || potential==out || potential==w1 || potential==w2 || potential==status) return false;
    float h=0.5f*p.dt;
    if(!mf_launch_mf_wave_local_v2(ctx,in,w1,potential,status,p,h,1u)) return false;
    ManifoldBuffer* a=w1; ManifoldBuffer* b=w2;
    unsigned dims[3]={p.nx,p.ny,p.nz};
    for(unsigned axis=0;axis<3;++axis) if(dims[axis]>1) {
        unsigned colors=dims[axis]%2 ? 3u : 2u;
        for(unsigned c=0;c<colors;++c) {
            if(!mf_launch_mf_wave_bond_v2(ctx,a,b,status,p,axis,c,h)) return false;
            auto tmp=a;a=b;b=tmp;
        }
    }
    for(int axis=2;axis>=0;--axis) if(dims[axis]>1) {
        int colors=dims[axis]%2 ? 3 : 2;
        for(int c=colors-1;c>=0;--c) {
            if(!mf_launch_mf_wave_bond_v2(ctx,a,b,status,p,unsigned(axis),unsigned(c),h)) return false;
            auto tmp=a;a=b;b=tmp;
        }
    }
    if(!mf_launch_mf_wave_local_v2(ctx,a,b,potential,status,p,h,0u) || !mf_v2_status_ok(ctx,status,p.n)) return false;
    return mf_launch_mf_wave_copy_v2(ctx,b,out,p.n) && mf_v2_sync(ctx);
}
bool manifold_wave_diagnostics_v2(ManifoldContext* ctx,ManifoldBuffer* in,ManifoldBuffer* potential,
                ManifoldBuffer* diag,ManifoldBuffer* status,const MFWaveParamsV2* pp) {
    if(!pp || !mf_wave_params_valid(*pp) || !mf_v2_ready(ctx,{in,potential,diag,status})) return false;
    auto p=*pp;
    if(p.n>0x1fffffffu || !mf_v2_enough(in,uint64_t(p.n)*sizeof(MFWaveValueV2)) ||
       !mf_v2_enough(potential,uint64_t(p.n)*4) || !mf_v2_enough(diag,uint64_t(p.n)*sizeof(MFWaveDiagnosticV2)) ||
       !mf_v2_enough(status,uint64_t(p.n)*4) || !mf_v2_distinct({in,potential,diag,status})) return false;
    return mf_launch_mf_wave_diagnostics_v2(ctx,in,potential,diag,status,p) && mf_v2_status_ok(ctx,status,p.n);
}
bool manifold_contact_step_v2(ManifoldContext* ctx,ManifoldBuffer* in,ManifoldBuffer* out,
                ManifoldBuffer* w1,ManifoldBuffer* w2,ManifoldBuffer* status,const MFContactParamsV2* pp) {
    if(!pp || !mf_contact_params_valid(*pp)) return false;
    auto p=*pp;
    if(!mf_v2_workspace(ctx,in,out,w1,w2,status,p.n,sizeof(MFParticleStateV2))) return false;
    if(!mf_launch_mf_contact_stage_v2(ctx,in,in,w1,status,p,0u) || !mf_v2_status_ok(ctx,status,p.n)) return false;
    if(!mf_launch_mf_contact_stage_v2(ctx,in,w1,w2,status,p,1u) || !mf_v2_status_ok(ctx,status,p.n)) return false;
    return mf_launch_mf_contact_copy_v2(ctx,w2,out,p.n) && mf_v2_sync(ctx);
}
bool manifold_contact_diagnostics_v2(ManifoldContext* ctx,ManifoldBuffer* in,
                ManifoldBuffer* diag,ManifoldBuffer* status,const MFContactParamsV2* pp) {
    if(!pp || !mf_contact_params_valid(*pp) || !mf_v2_ready(ctx,{in,diag,status})) return false;
    auto p=*pp;
    if(p.n>0x1fffffffu || !mf_v2_enough(in,uint64_t(p.n)*sizeof(MFParticleStateV2)) ||
       !mf_v2_enough(diag,uint64_t(p.n)*sizeof(MFContactResultV2)) || !mf_v2_enough(status,uint64_t(p.n)*4) ||
       !mf_v2_distinct({in,diag,status})) return false;
    return mf_launch_mf_contact_diagnostics_v2(ctx,in,diag,status,p) && mf_v2_status_ok(ctx,status,p.n);
}
}
