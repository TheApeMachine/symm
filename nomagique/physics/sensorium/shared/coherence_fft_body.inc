/* Shared body of both backend kernels, and both memory-placement variants.
   n/count validation is uniform BEFORE any barrier. Only one block owns a
   lattice. No grid-wide synchronization is assumed. */
unsigned n=num_modes_in?num_modes_in[0]:0u;
#ifdef MR_EMIT_LEDGER
MRComplex zero={0,0};
for(unsigned k=lane;k<p.max_carriers;k+=lanes){MR_EMIT_LEDGER(k,zero,zero,zero);}
#endif
if (!n) return;
unsigned needed=mr_workspace(n);
bool kinetic=gp.inv_domega2>0.0f;
bool valid=n<=p.max_carriers && needed && needed<=scratch_complex &&
    MR_FINITE(gp.dt) && MR_FINITE(gp.hbar_eff) && gp.hbar_eff>0 &&
    MR_FINITE(gp.mass_eff) && gp.mass_eff>=0 && (!kinetic || gp.mass_eff>0) &&
    MR_FINITE(gp.inv_domega2) && gp.inv_domega2>=0 &&
    MR_FINITE(gp.g_interaction) && MR_FINITE(gp.chemical_potential) &&
    MR_FINITE(gp.energy_decay) && gp.energy_decay>=0 &&
    MR_FINITE(p.offender_weight_floor) && p.offender_weight_floor>=0;
if (!valid) {
    // Invalid host/kernel contracts never read outside the allocated state.
    if (lane==0 && p.max_carriers) {
        mode_real[0]=qnan_f();mode_imag[0]=qnan_f();
#ifdef MR_EMIT_LEDGER
        MRComplex bad={qnan_f(),qnan_f()};MR_EMIT_LEDGER(0,bad,bad,bad);
#endif
    }
    return;
}
for (unsigned k=lane;k<n;k+=lanes) {
    MRComplex z={mode_real[k],mode_imag[k]};
    float ws=MR_ACC_READ(accums[k].w_sum);
    float local_g=gp.g_interaction;
#ifdef MR_GEOMETRY
    if(geometry_flags&1u)ws-=extra_potential[k];
    if(geometry_flags&2u)local_g=(metric_volume[k]>0 && MR_FINITE(metric_volume[k]))?local_g/metric_volume[k]:mr_nan();
#endif
    scratch[k]=mr_local_half(z,ws,local_g,gp.chemical_potential,gp.dt,gp.hbar_eff);
}
MR_FFT_BARRIER();
if(kinetic) {
#ifdef MR_GEOMETRY
    if(geometry_flags&2u) {
        unsigned colors=(n&1u)?3u:2u;
        // Palindromic edge-color splitting; every lane reaches every barrier.
        // Nonconstant metrics are not diagonal in the Fourier basis.
        for(unsigned stage=0;stage<2*colors-1;++stage) {
            unsigned color=stage<colors?stage:2*colors-2-stage;
            float bond_dt=gp.dt*(stage==colors-1?1.0f:.5f);
            for(unsigned i=lane;i<n;i+=lanes)if(mg_edge_color(i,n,color)) {
                unsigned j=i+1==n?0:i+1;
                MGPair z=mg_bond(scratch[i],scratch[j],metric_volume[i],metric_volume[j],
                    gp.hbar_eff,gp.mass_eff,gp.inv_domega2,bond_dt);
                scratch[i]=z.left;scratch[j]=z.right;
            }
            MR_FFT_BARRIER();
        }
        // Observable kinetic buffers retain their Fourier-coefficient contract.
        // A temporary transform does not replace the real-space metric flow.
        for(unsigned i=lane;i<n;i+=lanes){kinetic_real[i]=scratch[i].r;kinetic_imag[i]=scratch[i].i;}
        MR_FFT_BARRIER();
        MR_FFT_TRANSFORM(scratch,n,-1,lane,lanes);
        for(unsigned i=lane;i<n;i+=lanes){
            MRComplex original={kinetic_real[i],kinetic_imag[i]};
            kinetic_real[i]=scratch[i].r;kinetic_imag[i]=scratch[i].i;scratch[i]=original;
        }
        MR_FFT_BARRIER();
    } else
#endif
    {
    MR_FFT_TRANSFORM(scratch,n,-1,lane,lanes);
    for(unsigned k=lane;k<n;k+=lanes) {
        float phase=mr_kinetic_phase(k,n,gp.inv_domega2,gp.hbar_eff,gp.mass_eff,gp.dt);
        scratch[k]=mr_mul(scratch[k],mr_cis(phase));
        // Preserve the old API's observable kinetic coefficient buffers.
        kinetic_real[k]=scratch[k].r;kinetic_imag[k]=scratch[k].i;
    }
    MR_FFT_BARRIER();
    MR_FFT_TRANSFORM(scratch,n,+1,lane,lanes);
    }
}
for (unsigned k=lane;k<n;k+=lanes) {
    float ws=MR_ACC_READ(accums[k].w_sum);
    float local_g=gp.g_interaction;
#ifdef MR_GEOMETRY
    if(geometry_flags&1u)ws-=extra_potential[k];
    if(geometry_flags&2u)local_g=(metric_volume[k]>0 && MR_FINITE(metric_volume[k]))?local_g/metric_volume[k]:mr_nan();
#endif
    MRComplex z=mr_local_half(scratch[k],ws,local_g,gp.chemical_potential,gp.dt,gp.hbar_eff);
    #ifdef MR_EMIT_LEDGER
    MRComplex conservative=z;
    MRComplex damped=mr_open_step(z,0,0,0,p.offender_weight_floor,gp.energy_decay,gp.dt);
    #endif
    z=mr_open_step(z,MR_ACC_READ(accums[k].force_r),MR_ACC_READ(accums[k].force_i),
        MR_ACC_READ(accums[k].w_amp_sum),p.offender_weight_floor,gp.energy_decay,gp.dt);
#ifdef MR_EMIT_LEDGER
    MR_EMIT_LEDGER(k,conservative,damped,z);
#endif
    mode_real[k]=z.r;mode_imag[k]=z.i;
}
