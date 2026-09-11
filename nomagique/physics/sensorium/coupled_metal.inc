// Generated from tests/generate_coupled.py; shared bodies are the physics source.
#include "shared/coupled_core.h"
#include "shared/conservative_remap.h"
#include "shared/pilot_checked.h"
#include "shared/pilot_temporal.h"
#include "shared/contact_hash.h"
#include "shared/gravity_compatible.h"
#include "shared/reciprocal_coherence.h"
#define MC_UINT_ADD(p,v) atomic_fetch_add_explicit(p,v,memory_order_relaxed)
#define MC_NATIVE_ADD(p,v) atomic_fetch_add_explicit(p,v,memory_order_relaxed)
#define MC_NULL_ACCEL nullptr
#define MC_DEVICE device
#define MC_DEVICE_BARRIER() threadgroup_barrier(mem_flags::mem_device)
kernel void mc_pic_deposit(
 device const float* pos [[buffer(0)]],
 device const float* vel [[buffer(1)]],
 device const float* mass [[buffer(2)]],
 device const float* heat [[buffer(3)]],
 device atomic_float* out [[buffer(4)]],
 device unsigned* status [[buffer(5)]],
 constant unsigned& particles [[buffer(6)]],
 constant MFHydroParamsV2& p [[buffer(7)]],
 uint gid [[thread_position_in_grid]]) {
#define MC_ADD(p,v) MC_NATIVE_ADD(p,v)
#include "shared/coupled_scatter.inc"
#undef MC_ADD
}

kernel void mc_pic_gather(
 device const float* pos [[buffer(0)]],
 device const float* mass [[buffer(1)]],
 device float* pos_out [[buffer(2)]],
 device float* vel_out [[buffer(3)]],
 device float* heat_out [[buffer(4)]],
 device const MFHydroStateV2* state [[buffer(5)]],
 device unsigned* status [[buffer(6)]],
 constant unsigned& particles [[buffer(7)]],
 constant MFHydroParamsV2& p [[buffer(8)]],
 uint gid [[thread_position_in_grid]]) {
#include "shared/coupled_gather.inc"
}

kernel void mc_hydro_export(
 device const MFHydroStateV2* state [[buffer(0)]],
 device float* rho [[buffer(1)]],
 device float* mom [[buffer(2)]],
 device float* thermal [[buffer(3)]],
 device unsigned* status [[buffer(4)]],
 constant MFHydroParamsV2& p [[buffer(5)]],
 uint gid [[thread_position_in_grid]]) {
if(gid>=p.n)return;
MFPrimitive v=mf_primitive(state[gid],p);status[gid]=v.status;if(v.status)return;
unsigned j=mc_display_index(gid,p);rho[j]=state[gid].q[0];thermal[j]=v.thermal;
for(unsigned a=0;a<3;++a)mom[3*j+a]=state[gid].q[1+a];
}

kernel void mc_hydro_budget(
 device const MFHydroStateV2* initial [[buffer(0)]],
 device const MFHydroStateV2* stage [[buffer(1)]],
 device float* ledger [[buffer(2)]],
 device unsigned* status [[buffer(3)]],
 constant MFHydroParamsV2& p [[buffer(4)]],
 uint gid [[thread_position_in_grid]]) {
if(gid>=p.n)return;
MFRhs a=mf_hydro_rhs(initial,MC_NULL_ACCEL,gid,p),b=mf_hydro_rhs(stage,MC_NULL_ACCEL,gid,p);
status[gid]=a.status?a.status:b.status;if(status[gid])return;
ledger[2*gid]=.5f*p.dt*(a.viscous_heating+b.viscous_heating);
ledger[2*gid+1]=.5f*p.dt*(a.conductive_source+b.conductive_source);
}

kernel void mc_poisson_init(
 device const MFHydroStateV2* state [[buffer(0)]],
 device MRComplex* field [[buffer(1)]],
 device unsigned* status [[buffer(2)]],
 constant MFHydroParamsV2& p [[buffer(3)]],
 constant float& G [[buffer(4)]],
 uint gid [[thread_position_in_grid]]) {
if(gid>=p.n)return;
float v=12.5663706143591729538f*G*state[gid].q[0];
status[gid]=(!MF_FINITE(v)||state[gid].q[0]<0)?MF_PHYSICS_BAD_STATE:0;
field[gid]={v,0};
}

kernel void mc_poisson_spectral(
 device MRComplex* field [[buffer(0)]],
 device unsigned* status [[buffer(1)]],
 constant MFHydroParamsV2& p [[buffer(2)]],
 uint gid [[thread_position_in_grid]]) {
if(gid>=p.n)return;
if(gid==0){field[gid]={0,0};return;}
float lambda=mc_poisson_eigenvalue(gid,p);field[gid]=mr_scale(field[gid],1/lambda);
if(!MF_FINITE(field[gid].r)||!MF_FINITE(field[gid].i))status[gid]=MF_PHYSICS_BAD_STATE;
}

kernel void mc_poisson_export(
 device const MRComplex* field [[buffer(0)]],
 device float* potential [[buffer(1)]],
 device float* acceleration [[buffer(2)]],
 device unsigned* status [[buffer(3)]],
 constant MFHydroParamsV2& p [[buffer(4)]],
 uint gid [[thread_position_in_grid]]) {
if(gid>=p.n)return;
potential[mc_display_index(gid,p)]=field[gid].r;
if(!MF_FINITE(field[gid].r)||!MF_FINITE(field[gid].i))status[gid]=MF_PHYSICS_BAD_STATE;
for(unsigned a=0;a<3;++a){
 float g=-(field[mf_hneighbor(gid,a,+1,p)].r-field[mf_hneighbor(gid,a,-1,p)].r)/(2*p.dx);
 acceleration[3*gid+a]=g;if(!MF_FINITE(g))status[gid]=MF_PHYSICS_BAD_STATE;
}
}

kernel void mc_gravity_kick(
 device MFHydroStateV2* state [[buffer(0)]],
 device const float* acceleration [[buffer(1)]],
 device unsigned* status [[buffer(2)]],
 constant MFHydroParamsV2& p [[buffer(3)]],
 uint gid [[thread_position_in_grid]]) {
if(gid>=p.n)return;
MFStepResult r=mc_gravity_increment(state[gid],acceleration[3*gid],acceleration[3*gid+1],acceleration[3*gid+2],p.dt,p);
status[gid]=r.status;if(!r.status)state[gid]=r.state;
}

kernel void mc_poisson_axis(
 device MRComplex* field [[buffer(0)]],
 device MRComplex* work [[buffer(1)]],
 constant MFHydroParamsV2& p [[buffer(2)]],
 constant unsigned& axis [[buffer(3)]],
 constant int& sign [[buffer(4)]],
 constant unsigned& workspace [[buffer(5)]],
 uint lane [[thread_index_in_threadgroup]],
 uint lanes [[threads_per_threadgroup]],
 uint line [[threadgroup_position_in_grid]]) {
#define MC_MEM MC_DEVICE
#define MC_BARRIER() MC_DEVICE_BARRIER()
#define MC_TRANSFORM mr_transform_global
#include "shared/coupled_poisson_axis.inc"
#undef MC_MEM
#undef MC_BARRIER
#undef MC_TRANSFORM
}

kernel void mc_pic_deposit_material(
 device const float* pos [[buffer(0)]],
 device const float* vel [[buffer(1)]],
 device const float* mass [[buffer(2)]],
 device const float* heat [[buffer(3)]],
 device const float* total [[buffer(4)]],
 device atomic_float* out [[buffer(5)]],
 device unsigned* status [[buffer(6)]],
 constant unsigned& particles [[buffer(7)]],
 constant MFHydroParamsV2& p [[buffer(8)]],
 uint gid [[thread_position_in_grid]]) {
if(gid>=particles)return;
status[gid]=mc_particle_valid(pos,vel,mass[gid],heat[gid],gid);
if(status[gid]||!mf_finite_nonnegative(total[gid])){status[gid]=MF_PHYSICS_BAD_STATE;return;}
MFCIC sh=mc_cic(pos[3*gid],pos[3*gid+1],pos[3*gid+2],p);if(sh.status){status[gid]=sh.status;return;}
float inv=1/(p.dx*p.dx*p.dx),v[6]={mass[gid],mass[gid]*vel[3*gid],mass[gid]*vel[3*gid+1],mass[gid]*vel[3*gid+2],total[gid],heat[gid]};
for(unsigned a=0;a<6;++a)if(!MF_FINITE(v[a]*inv)){status[gid]=MF_PHYSICS_BAD_STATE;return;}
for(unsigned k=0;k<8;++k)for(unsigned a=0;a<6;++a)MC_NATIVE_ADD(&out[6*sh.index[k]+a],v[a]*(sh.weight[k]*inv));

}

kernel void mc_remap_rows(
 device const MFHydroStateV2* state [[buffer(0)]],
 device const float* pos [[buffer(1)]],
 device const float* columns [[buffer(2)]],
 device float* rows [[buffer(3)]],
 device unsigned* status [[buffer(4)]],
 constant unsigned& particles [[buffer(5)]],
 constant MFHydroParamsV2& p [[buffer(6)]],
 constant float& width [[buffer(7)]],
 constant float& mass_scale [[buffer(8)]],
 uint gid [[thread_position_in_grid]]) {
if(gid>=p.n)return;
status[gid]=0;if(state[gid].q[0]==0){rows[gid]=0;return;}
MCLogSum s={};for(unsigned i=0;i<particles;++i)mc_log_add(s,columns[i]-mc_transport_cost(gid,pos,i,p,width));
float target=mc_row_mass(state[gid],p,mass_scale);
rows[gid]=MC_LOG(target)-mc_log_total(s);
if(!(target>0)||!MF_FINITE(rows[gid]))status[gid]=MF_PHYSICS_BAD_STATE;

}

kernel void mc_remap_columns(
 device const MFHydroStateV2* state [[buffer(0)]],
 device const float* pos [[buffer(1)]],
 device const float* mass [[buffer(2)]],
 device const float* rows [[buffer(3)]],
 device float* columns [[buffer(4)]],
 device unsigned* status [[buffer(5)]],
 constant unsigned& particles [[buffer(6)]],
 constant MFHydroParamsV2& p [[buffer(7)]],
 constant float& width [[buffer(8)]],
 uint gid [[thread_position_in_grid]]) {
if(gid>=particles)return;
MCLogSum s={};for(unsigned c=0;c<p.n;++c)if(state[c].q[0]>0)mc_log_add(s,rows[c]-mc_transport_cost(c,pos,gid,p,width));
columns[gid]=MC_LOG(mass[gid])-mc_log_total(s);
status[gid]=(!s.count||!(mass[gid]>0)||!MF_FINITE(columns[gid]))?MF_PHYSICS_BAD_STATE:0;

}

kernel void mc_remap_residual(
 device const MFHydroStateV2* state [[buffer(0)]],
 device const float* pos [[buffer(1)]],
 device const float* rows [[buffer(2)]],
 device const float* columns [[buffer(3)]],
 device float* residual [[buffer(4)]],
 device unsigned* status [[buffer(5)]],
 constant unsigned& particles [[buffer(6)]],
 constant MFHydroParamsV2& p [[buffer(7)]],
 constant float& width [[buffer(8)]],
 constant float& mass_scale [[buffer(9)]],
 uint gid [[thread_position_in_grid]]) {
if(gid>=p.n)return;
status[gid]=0;residual[gid]=0;if(state[gid].q[0]==0)return;
MCLogSum s={};for(unsigned i=0;i<particles;++i)mc_log_add(s,columns[i]-mc_transport_cost(gid,pos,i,p,width));
float ratio=MR_EXP(rows[gid]+mc_log_total(s)-MC_LOG(mc_row_mass(state[gid],p,mass_scale)));
residual[gid]=mf_abs(ratio-1);if(!MF_FINITE(ratio))status[gid]=MF_PHYSICS_BAD_STATE;

}

kernel void mc_remap_gather(
 device const MFHydroStateV2* state [[buffer(0)]],
 device const float* pos [[buffer(1)]],
 device const float* mass [[buffer(2)]],
 device const float* rows [[buffer(3)]],
 device float* velocity [[buffer(4)]],
 device float* heat [[buffer(5)]],
 device float* total [[buffer(6)]],
 device float* mixing [[buffer(7)]],
 device unsigned* status [[buffer(8)]],
 constant unsigned& particles [[buffer(9)]],
 constant MFHydroParamsV2& p [[buffer(10)]],
 constant float& width [[buffer(11)]],
 constant float& mass_scale [[buffer(12)]],
 uint gid [[thread_position_in_grid]]) {
if(gid>=particles)return;
status[gid]=0;MCLogSum ls={};unsigned reference=0;float peak=-3.402823466e38f;
for(unsigned c=0;c<p.n;++c)if(state[c].q[0]>0){float z=rows[c]-mc_transport_cost(c,pos,gid,p,width);mc_log_add(ls,z);if(z>peak){peak=z;reference=c;}}
if(!ls.count){status[gid]=MF_PHYSICS_BAD_STATE;return;}
float ref[3];for(unsigned a=0;a<3;++a)ref[a]=state[reference].q[a+1]/(state[reference].q[0]*mass_scale);
MCTransportMoments m={};
for(unsigned c=0;c<p.n;++c)if(state[c].q[0]>0){
 float fraction=MR_EXP(rows[c]-mc_transport_cost(c,pos,gid,p,width)-ls.maximum)/ls.sum;
 if(fraction==0)continue;
 MFPrimitive v=mf_primitive(state[c],p);if(v.status){status[gid]=v.status;return;}
 float r=state[c].q[0]*mass_scale;
 for(unsigned a=0;a<3;++a)mc_compensated_add(m.delta[a],m.correction[a],fraction*(state[c].q[a+1]/r-ref[a]));
 mc_compensated_add(m.thermal,m.correction[3],fraction*(v.thermal/r));
 mc_compensated_add(m.total,m.correction[4],fraction*(state[c].q[4]/r));
}
float out[3];for(unsigned a=0;a<3;++a)out[a]=ref[a]+m.delta[a];
for(unsigned c=0;c<p.n;++c)if(state[c].q[0]>0){
 float fraction=MR_EXP(rows[c]-mc_transport_cost(c,pos,gid,p,width)-ls.maximum)/ls.sum;
 if(fraction==0)continue;
 float v2=0;for(unsigned a=0;a<3;++a){float dv=(state[c].q[a+1]/(state[c].q[0]*mass_scale)-ref[a])-m.delta[a];v2+=dv*dv;}
 mc_compensated_add(m.variance,m.correction[5],.5f*fraction*v2);
}
float q=mass[gid]*(m.thermal+m.variance),e=mass[gid]*m.total;
if(!mf_finite_nonnegative(q)||!mf_finite_nonnegative(e)){status[gid]=MF_PHYSICS_NEGATIVE_UPDATE;return;}
for(unsigned a=0;a<3;++a)if(!MF_FINITE(out[a])){status[gid]=MF_PHYSICS_BAD_STATE;return;}
for(unsigned a=0;a<3;++a)velocity[3*gid+a]=out[a];heat[gid]=q;total[gid]=e;mixing[gid]=mass[gid]*m.variance;

}

kernel void mc_pilot_checked_step(
 device const float* pos [[buffer(0)]],
 device const float* mass [[buffer(1)]],
 device const float* prior [[buffer(2)]],
 device const float* re [[buffer(3)]],
 device const float* im [[buffer(4)]],
 device float* position [[buffer(5)]],
 device float* guide [[buffer(6)]],
 device float* report [[buffer(7)]],
 device unsigned* status [[buffer(8)]],
 constant unsigned& particles [[buffer(9)]],
 constant MFHydroParamsV2& p [[buffer(10)]],
 constant float& hbar [[buffer(11)]],
 constant float& tolerance [[buffer(12)]],
 constant float& maxcells [[buffer(13)]],
 uint gid [[thread_position_in_grid]]) {
if(gid>=particles)return;
float x[3],old[3];for(unsigned a=0;a<3;++a){x[a]=pos[3*gid+a];old[a]=prior[3*gid+a];if(!MF_FINITE(old[a])){status[gid]=MF_PHYSICS_BAD_STATE;return;}}
auto v=mc_pilot_checked(re,im,x,old,mass[gid],hbar,p.dt,tolerance,maxcells,p);status[gid]=v.status;
if(v.status)return;
for(unsigned a=0;a<3;++a){position[3*gid+a]=v.x[a];guide[3*gid+a]=v.guide[a];}
report[4*gid]=v.density;report[4*gid+1]=v.error;report[4*gid+2]=v.pathcells;report[4*gid+3]=v.speed;

}

kernel void mc_contact_pack(
 device const float* pos [[buffer(0)]],
 device const float* vel [[buffer(1)]],
 device const float* mass [[buffer(2)]],
 device const float* heat [[buffer(3)]],
 device MFParticleStateV2* state [[buffer(4)]],
 device unsigned* cell [[buffer(5)]],
 device atomic_uint* count [[buffer(6)]],
 device unsigned* status [[buffer(7)]],
 constant unsigned& particles [[buffer(8)]],
 constant MFHydroParamsV2& p [[buffer(9)]],
 uint gid [[thread_position_in_grid]]) {
if(gid>=particles)return;
status[gid]=mc_particle_valid(pos,vel,mass[gid],heat[gid],gid);if(status[gid])return;
MFParticleStateV2 v={};for(unsigned a=0;a<3;++a){v.x[a]=pos[3*gid+a];v.v[a]=vel[3*gid+a];}v.mass=mass[gid];v.heat=heat[gid];state[gid]=v;
auto sh=mc_cic(v.x[0],v.x[1],v.x[2],p);status[gid]=sh.status;if(sh.status)return;cell[gid]=sh.index[0];MC_UINT_ADD(&count[cell[gid]],1u);

}

kernel void mc_contact_sort(
 device const unsigned* cell [[buffer(0)]],
 device const unsigned* starts [[buffer(1)]],
 device atomic_uint* offset [[buffer(2)]],
 device unsigned* sorted [[buffer(3)]],
 device unsigned* status [[buffer(4)]],
 constant unsigned& particles [[buffer(5)]],
 constant MFHydroParamsV2& p [[buffer(6)]],
 uint gid [[thread_position_in_grid]]) {
if(gid>=particles)return;
unsigned c=cell[gid];if(c>=p.n){status[gid]=MF_PHYSICS_BAD_STATE;return;}
unsigned dest=starts[c]+MC_UINT_ADD(&offset[c],1u);if(dest>=particles){status[gid]=MF_PHYSICS_BAD_STATE;return;}sorted[dest]=gid;

}

kernel void mc_contact_hash_kick(
 device const MFParticleStateV2* initial [[buffer(0)]],
 device const MFParticleStateV2* stage [[buffer(1)]],
 device const unsigned* starts [[buffer(2)]],
 device const unsigned* sorted [[buffer(3)]],
 device MFParticleStateV2* out [[buffer(4)]],
 device MFContactResultV2* diagnostic [[buffer(5)]],
 device unsigned* status [[buffer(6)]],
 constant unsigned& particles [[buffer(7)]],
 constant MFHydroParamsV2& p [[buffer(8)]],
 constant MFContactParamsV2& contact [[buffer(9)]],
 constant unsigned& second [[buffer(10)]],
 constant float& max_speed [[buffer(11)]],
 uint gid [[thread_position_in_grid]]) {
if(gid>=particles)return;
auto f=mc_contact_hash_evaluate(stage,starts,sorted,gid,contact,p,max_speed);diagnostic[gid]=f.result;
auto o=mc_contact_kick_state(initial[gid],stage[gid],f,second);status[gid]=o.status;if(!o.status)out[gid]=o.state;

}

kernel void mc_contact_unpack(
 device const MFParticleStateV2* state [[buffer(0)]],
 device float* vel [[buffer(1)]],
 device float* heat [[buffer(2)]],
 constant unsigned& particles [[buffer(3)]],
 constant MFHydroParamsV2& p [[buffer(4)]],
 uint gid [[thread_position_in_grid]]) {
(void)p;if(gid>=particles)return;
for(unsigned a=0;a<3;++a)vel[3*gid+a]=state[gid].v[a];heat[gid]=state[gid].heat;

}

kernel void mc_gravity_compatible(
 device const MFHydroStateV2* flux_initial [[buffer(0)]],
 device const MFHydroStateV2* flux_stage [[buffer(1)]],
 device const MFHydroStateV2* state [[buffer(2)]],
 device const float* phi0 [[buffer(3)]],
 device const float* phi1 [[buffer(4)]],
 device const float* kick_work [[buffer(5)]],
 device MFHydroStateV2* out [[buffer(6)]],
 device unsigned* status [[buffer(7)]],
 constant MFHydroParamsV2& p [[buffer(8)]],
 uint gid [[thread_position_in_grid]]) {
if(gid>=p.n)return;
auto w=mc_gravity_flux_work(flux_initial,flux_stage,phi0,phi1,gid,p);status[gid]=w.status;if(w.status)return;
auto u=state[gid];u.q[4]+=w.density-kick_work[gid];u=mf_sync_aux(u,p);status[gid]=mf_primitive(u,p).status;
if(!status[gid])out[gid]=u;

}

kernel void mc_gravity_remap(
 device const MFHydroStateV2* state [[buffer(0)]],
 device const float* pos [[buffer(1)]],
 device const float* mass [[buffer(2)]],
 device const float* rows [[buffer(3)]],
 device const float* phi0 [[buffer(4)]],
 device const float* phi1 [[buffer(5)]],
 device float* energy [[buffer(6)]],
 device float* work [[buffer(7)]],
 device unsigned* status [[buffer(8)]],
 constant unsigned& particles [[buffer(9)]],
 constant MFHydroParamsV2& p [[buffer(10)]],
 constant float& width [[buffer(11)]],
 uint gid [[thread_position_in_grid]]) {
if(gid>=particles)return;
auto w=mc_gravity_remap_work(state,pos,mass,rows,phi0,phi1,gid,width,p);status[gid]=w.status;if(w.status)return;
float next=energy[gid]+w.density;if(!mf_finite_nonnegative(next)){status[gid]=MF_PHYSICS_NEGATIVE_UPDATE;return;}
work[gid]=next-energy[gid];energy[gid]=next;

}

kernel void mc_reciprocal_potential(
 device const float* pos [[buffer(0)]],
 device const float* amplitude [[buffer(1)]],
 device const float* omega [[buffer(2)]],
 device const float* mode_omega [[buffer(3)]],
 device const float* linewidth [[buffer(4)]],
 device const unsigned* indices [[buffer(5)]],
 device const float* weights [[buffer(6)]],
 device float* potential [[buffer(7)]],
 device unsigned* status [[buffer(8)]],
 constant unsigned& particles [[buffer(9)]],
 constant unsigned& modes [[buffer(10)]],
 constant unsigned& anchors [[buffer(11)]],
 constant float& sigma [[buffer(12)]],
 constant MFHydroParamsV2& p [[buffer(13)]],
 uint gid [[thread_position_in_grid]]) {
if(gid>=modes)return;
potential[gid]=mc_coherence_potential(pos,amplitude,omega,mode_omega,linewidth,indices,weights,gid,particles,anchors,sigma,p);
status[gid]=MF_FINITE(potential[gid])?0:MF_PHYSICS_BAD_STATE;

}

kernel void mc_reciprocal_force(
 device const float* pos [[buffer(0)]],
 device const float* amplitude [[buffer(1)]],
 device const float* omega [[buffer(2)]],
 device const float* mode_omega [[buffer(3)]],
 device const float* linewidth [[buffer(4)]],
 device const unsigned* indices [[buffer(5)]],
 device const float* weights [[buffer(6)]],
 device const float* occupation [[buffer(7)]],
 device atomic_float* force [[buffer(8)]],
 device unsigned* status [[buffer(9)]],
 constant unsigned& particles [[buffer(10)]],
 constant unsigned& modes [[buffer(11)]],
 constant unsigned& anchors [[buffer(12)]],
 constant float& sigma [[buffer(13)]],
 constant MFHydroParamsV2& p [[buffer(14)]],
 uint gid [[thread_position_in_grid]]) {
if(gid>=particles)return;
status[gid]=0;
if(!mf_finite_nonnegative(amplitude[gid])){status[gid]=MF_PHYSICS_BAD_STATE;return;}
for(unsigned k=0;k<modes;++k){
 auto norm=mc_anchor_norm(indices,weights,k,particles,anchors);if(norm.status){status[gid]=norm.status;return;}
 float r=mc_resonance(omega[gid],mode_omega[k],linewidth[k]);
 if(!MF_FINITE(r)||!mf_finite_nonnegative(occupation[k])){status[gid]=MF_PHYSICS_BAD_STATE;return;}
 if(norm.weight==0)continue;
 for(unsigned a=0;a<anchors;++a){unsigned j=indices[k*anchors+a];if(j==0xffffffffu||j==gid||weights[k*anchors+a]==0)continue;
  auto pair=mc_coherence_pair(pos,gid,j,sigma,p);if(pair.status){status[gid]=pair.status;return;}
  float scale=occupation[k]*amplitude[gid]*r*(weights[k*anchors+a]/norm.weight);
  for(unsigned axis=0;axis<3;++axis){float value=scale*pair.gradient[axis];
   if(!MF_FINITE(value)){status[gid]=MF_PHYSICS_BAD_STATE;return;}
   MC_NATIVE_ADD(&force[3*gid+axis],value);MC_NATIVE_ADD(&force[3*j+axis],-value);
  }
 }
}

}

kernel void mc_reciprocal_occupation(
 device const float* old_re [[buffer(0)]],
 device const float* old_im [[buffer(1)]],
 device const float* ledger [[buffer(2)]],
 device float* occupation [[buffer(3)]],
 device unsigned* status [[buffer(4)]],
 constant unsigned& modes [[buffer(5)]],
 constant float& domega [[buffer(6)]],
 constant MFHydroParamsV2& p [[buffer(7)]],
 uint gid [[thread_position_in_grid]]) {
(void)p;if(gid>=modes)return;
float r=old_re[gid],i=old_im[gid],cr=ledger[6*gid],ci=ledger[6*gid+1];
occupation[gid]=(.5f*domega)*(r*r+i*i+cr*cr+ci*ci);
status[gid]=mf_finite_nonnegative(occupation[gid])?0:MF_PHYSICS_BAD_STATE;

}

kernel void mc_pilot_checked_time_step(
 device const float* pos [[buffer(0)]],
 device const float* mass [[buffer(1)]],
 device const float* prior [[buffer(2)]],
 device const float* re0 [[buffer(3)]],
 device const float* im0 [[buffer(4)]],
 device const float* re1 [[buffer(5)]],
 device const float* im1 [[buffer(6)]],
 device float* position [[buffer(7)]],
 device float* guide [[buffer(8)]],
 device float* report [[buffer(9)]],
 device unsigned* status [[buffer(10)]],
 constant unsigned& particles [[buffer(11)]],
 constant MFHydroParamsV2& p [[buffer(12)]],
 constant float& hbar [[buffer(13)]],
 constant float& tolerance [[buffer(14)]],
 constant float& maxcells [[buffer(15)]],
 uint gid [[thread_position_in_grid]]) {
if(gid>=particles)return;
float x[3],old[3];for(unsigned a=0;a<3;++a){x[a]=pos[3*gid+a];old[a]=prior[3*gid+a];if(!MF_FINITE(old[a])){status[gid]=MF_PHYSICS_BAD_STATE;return;}}
MCPilotField field={re0,im0,re1,im1};
auto out=mc_pilot_checked_time(field,x,old,mass[gid],hbar,p.dt,tolerance,maxcells,p);status[gid]=out.status;
if(out.status)return;
for(unsigned a=0;a<3;++a){position[3*gid+a]=out.x[a];guide[3*gid+a]=out.guide[a];}
report[4*gid]=out.density;report[4*gid+1]=out.error;report[4*gid+2]=out.pathcells;report[4*gid+3]=out.speed;

}

#undef MC_NULL_ACCEL
#undef MC_NATIVE_ADD
#undef MC_DEVICE
#undef MC_DEVICE_BARRIER