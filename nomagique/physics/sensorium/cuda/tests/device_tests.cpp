// Tests invoke the real C bridge and CUDA kernels. No mock runtime is linked.
#include "../../bridge.h"
#include <cuda_runtime_api.h>
#include <algorithm>
#include <array>
#include <cmath>
#include <cstdio>
#include <cstdlib>
#include <stdexcept>
#include <string>
#include <vector>

static void require(bool ok,const char* what) { if (!ok) throw std::runtime_error(what); }
static void near(double a,double b,double tol,const char* what) {
    if (!std::isfinite(a) || !std::isfinite(b) || std::fabs(a-b)>tol) {
        std::fprintf(stderr,"%s: actual=%.12g expected=%.12g tolerance=%.3g\n",what,a,b,tol);
        throw std::runtime_error(what);
    }
}
struct Context {
    ManifoldContext* p=nullptr;
    explicit Context(int device) { char error[512]{};p=manifold_create_cuda_context(device,error,sizeof(error));if(!p)throw std::runtime_error(error); }
    ~Context(){manifold_destroy_context(p);}
    void sync(){if(!manifold_synchronize_checked(p))throw std::runtime_error(manifold_last_error(p));}
};
template<class T> struct Buffer {
    ManifoldBuffer* p=nullptr; size_t size; Context& c;
    Buffer(Context& c,size_t n):size(n),c(c){p=manifold_create_buffer(c.p,std::max(n,size_t(1))*sizeof(T),nullptr);if(!p)throw std::runtime_error(manifold_last_error(c.p));}
    Buffer(Context& c,std::initializer_list<T> values):Buffer(c,values.size()){std::copy(values.begin(),values.end(),data());}
    Buffer(const Buffer&)=delete;Buffer& operator=(const Buffer&)=delete;
    ~Buffer(){manifold_destroy_buffer(p);}
    T* data(){auto* v=static_cast<T*>(manifold_get_buffer_pointer(p));if(!v)throw std::runtime_error(manifold_last_error(c.p));return v;}
    void fill(T v){auto* d=data();std::fill(d,d+size,v);}
};
struct Accum { float fr,fi,ws,wos,wo2s,was;unsigned score,index; };
static_assert(sizeof(Accum)==32);

static void diagnostics(Context& c){
    Buffer<float> input(c,513),stats(c,4),stats2(c,4);auto* x=input.data();double sum=0,sq=0,abs=0;
    for(unsigned j=0;j<513;++j){x[j]=float(int(j%13)-6);sum+=x[j];sq+=x[j]*x[j];abs+=std::fabs(x[j]);}
    manifold_thermo_reduce_energy_stats(c.p,input.p,stats.p);
    // Queue another use of the same cached scratch before synchronizing.
    manifold_thermo_reduce_energy_stats(c.p,input.p,stats2.p);c.sync();
    auto* s=stats.data();near(s[0],abs/513,1e-5,"mean absolute");near(s[1],sum/513,1e-6,"mean");near(s[2],std::sqrt(sq/513-(sum/513)*(sum/513)),1e-5,"standard deviation");near(s[3],513,0,"reduction count");
    for(unsigned j=0;j<4;++j)near(stats2.data()[j],s[j],0,"scratch lifetime/reuse");
    manifold_clear_field(c.p,input.p);c.sync();for(unsigned j=0;j<513;++j)near(input.data()[j],0,0,"clear tail");
}
static void scan(Context& c,ManifoldBuffer* in,ManifoldBuffer* out,unsigned n){
    if(n==0){manifold_exclusive_scan_u32_finalize_total(c.p,in,out,0);return;}
    unsigned groups=(n+255)/256;Buffer<unsigned> sums(c,groups),prefix(c,groups);
    manifold_exclusive_scan_u32_pass1(c.p,in,out,sums.p,n);
    if(groups>1){scan(c,sums.p,prefix.p,groups);manifold_exclusive_scan_u32_add_block_offsets(c.p,out,prefix.p,n);}
    c.sync();
}
static void scans(Context& c){
    for(unsigned n:{0u,1u,255u,256u,257u,65539u}){
        Buffer<unsigned> input(c,std::max(1u,n)),out(c,size_t(n)+1);auto* x=input.data();
        for(unsigned j=0;j<n;++j)x[j]=j%5;
        scan(c,input.p,out.p,n);
        manifold_exclusive_scan_u32_finalize_total(c.p,input.p,out.p,n);c.sync();
        auto* got=out.data();unsigned expected=0;
        for(unsigned j=0;j<n;++j){require(got[j]==expected,"hierarchical exclusive scan");expected+=j%5;}
        require(got[n]==expected,"scan terminal total");
    }
}
static void scatter_and_gather(Context& c){
    constexpr unsigned n=2,gx=4,gy=5,gz=6,cells=gx*gy*gz;
    Buffer<float> pos(c,{.125f,.375f,.625f,.9375f,.0625f,.8125f}),vel(c,{1,2,3,-2,.5f,4}),mass(c,{2,3}),heat(c,{4,7}),osc(c,{100,200});
    Buffer<unsigned> idx(c,n),counts(c,cells),starts(c,cells+1),offsets(c,cells),original(c,n);
    Buffer<float> sp(c,n*3),sv(c,n*3),sm(c,n),sh(c,n),se(c,n),rho(c,cells),mom(c,cells*3),e(c,cells);
    manifold_scatter_compute_cell_idx(c.p,pos.p,idx.p,gx,gy,gz,.25f);
    manifold_scatter_count_cells(c.p,idx.p,counts.p,gx,gy,gz,.25f);
    scan(c,counts.p,starts.p,cells);
    manifold_scatter_reorder_particles(c.p,pos.p,vel.p,mass.p,heat.p,osc.p,idx.p,starts.p,offsets.p,sp.p,sv.p,sm.p,sh.p,se.p,original.p,gx,gy,gz,.25f);
    manifold_scatter_sorted(c.p,sp.p,sv.p,sm.p,sh.p,se.p,rho.p,mom.p,e.p,gx,gy,gz,.25f);c.sync();
    double rsum=0,esum=0,xsum=0,ysum=0,zsum=0;auto* r=rho.data();auto* ee=e.data();auto* mm=mom.data();
    for(unsigned j=0;j<cells;++j){rsum+=r[j];esum+=ee[j];xsum+=mm[j*3];ysum+=mm[j*3+1];zsum+=mm[j*3+2];}
    near(rsum/64,5,1e-5,"scatter mass");near(esum/64,11,1e-5,"scatter thermal excludes oscillator energy");near(xsum/64,-4,1e-5,"scatter momentum x");near(ysum/64,5.5,1e-5,"scatter momentum y");near(zsum/64,18,1e-5,"scatter momentum z");
    auto* order=original.data();require(order[0]!=order[1] && order[0]<n && order[1]<n,"sort permutation");
    Buffer<float> out(c,n*3),vout(c,n*3),qout(c,n),gravity(c,cells);Buffer<unsigned> head(c,1),events(c,256*6);
    for(float density:{0.f,2.f,0.f,1.f}){
        rho.fill(density);e.fill(density);mom.fill(0);head.fill(0);
        manifold_pic_gather_update_particles(c.p,pos.p,mass.p,out.p,vout.p,qout.p,rho.p,mom.p,e.p,gravity.p,head.p,events.p,256,gx,gy,gz,.25f,.001f,1,1.25f,1.5f,1.4f,.4f,1,1e-3f,1e-3f,0);c.sync();
        require(head.data()[0]==0,"valid gather debug buffer");
        near(qout.data()[0],density?2:0,1e-6,"gather heat mass 2");near(qout.data()[1],density?3:0,1e-6,"gather heat mass 3");
    }
    rho.fill(1);e.fill(-1);head.fill(0);
    manifold_pic_gather_update_particles(c.p,pos.p,mass.p,out.p,vout.p,qout.p,rho.p,mom.p,e.p,gravity.p,head.p,events.p,256,gx,gy,gz,.25f,.001f,1,1.25f,1.5f,1.4f,.4f,1,1e-3f,1e-3f,0);c.sync();
    require(head.data()[0]>0 && events.data()[0]==0x04,"negative energy debug rejection");
}
static void gas(Context& c){
    constexpr unsigned n=3*4*5;Buffer<float> r(c,n),m(c,n*3),e(c,n),r1(c,n),m1(c,n*3),e1(c,n),r2(c,n),m2(c,n*3),e2(c,n),kr(c,n),km(c,n*3),ke(c,n);
    Buffer<unsigned> head(c,1),words(c,256*6);r.fill(1);e.fill(2.5f);
    manifold_gas_rk2_stage1(c.p,r.p,m.p,e.p,r1.p,m1.p,e1.p,kr.p,km.p,ke.p,head.p,words.p,256,3,4,5,.2f,.001f,1.4f,1,1e-3f,1e-3f,1e-4f,1e-4f);
    manifold_gas_rk2_stage2(c.p,r.p,m.p,e.p,r1.p,m1.p,e1.p,kr.p,km.p,ke.p,r2.p,m2.p,e2.p,head.p,words.p,256,3,4,5,.2f,.001f,1.4f,1,1e-3f,1e-3f,1e-4f,1e-4f);c.sync();
    require(head.data()[0]==0,"uniform gas accepted");for(unsigned j=0;j<n;++j){near(r2.data()[j],1,0,"uniform density");near(e2.data()[j],2.5,0,"uniform energy");}
    e.data()[0]=-1;head.fill(0);
    manifold_gas_rk2_stage1(c.p,r.p,m.p,e.p,r1.p,m1.p,e1.p,kr.p,km.p,ke.p,head.p,words.p,256,3,4,5,.2f,.001f,1.4f,1,1e-3f,1e-3f,1e-4f,1e-4f);c.sync();
    require(head.data()[0]>0 && std::isnan(r1.data()[0]),"inadmissible gas fails visibly");
}
static void projection_pilot(Context& c){
    constexpr unsigned gx=8,gy=6,gz=4,n=gx*gy*gz;
    Buffer<float> mr(c,{2}),mi(c,{3}),aw(c,8),p(c,{.125f,.25f,.375f}),re(c,n),im(c,n);Buffer<unsigned> ai(c,8);ai.fill(UINT32_MAX);ai.data()[0]=0;aw.data()[0]=1;
    manifold_project_modes_to_spatial_psi(c.p,mr.p,mi.p,ai.p,aw.p,p.p,re.p,im.p,8,gx,gy,gz,.125f);c.sync();
    unsigned at=1*gy*gz+2*gz+3;near(re.data()[at],2,0,"noncubic projection index");near(im.data()[at],3,0,"noncubic projection imaginary");
    auto* r=re.data();auto* i=im.data();
    for(unsigned x=0;x<gx;++x)for(unsigned y=0;y<gy;++y)for(unsigned z=0;z<gz;++z){double phase=2*std::acos(-1.)*y/gy;unsigned j=x*gy*gz+y*gz+z;r[j]=float(std::cos(phase));i[j]=float(std::sin(phase));}
    Buffer<float> pos(c,{.5f,.375f,.25f,.5f,.375f,.25f}),mass(c,{1,2}),out(c,6),vel(c,6);
    auto run=[&]{manifold_pic_gather_pilot_wave(c.p,pos.p,mass.p,out.p,vel.p,re.p,im.p,2,gx,gy,gz,.125f,2,1,.75f,.5f,1,1e-8f,1e-6f);c.sync();};run();
    double expected=8*std::sin(2*std::acos(-1.)/gy);near(vel.data()[1],expected,4e-5,"pilot y velocity");near(vel.data()[4],expected/2,4e-5,"pilot inverse mass");require(out.data()[1]>=0 && out.data()[1]<.75f,"multiple-box wrap");
    i=im.data();for(unsigned j=0;j<n;++j)i[j]=-i[j];run();near(vel.data()[1],-expected,4e-5,"conjugate current");
    re.fill(0);im.fill(0);run();for(unsigned j=0;j<6;++j)near(out.data()[j],pos.data()[j],0,"zero wave unchanged position");
}
static void forces_and_phases(Context& c){
    constexpr unsigned n=257;Buffer<float> phase(c,n),omega(c,n),amp(c,n),pos(c,n*3),heat(c,n),cw(c,{0}),width(c,{1}),weight(c,8),mr(c,{0}),mi(c,{1});Buffer<unsigned> anchor(c,8),count(c,{1}),starts(c,{0,1}),index(c,{0});Buffer<float> bin(c,{0,1});Buffer<Accum> acc(c,1);
    amp.fill(1);heat.fill(1);anchor.fill(UINT32_MAX);anchor.data()[0]=0;weight.data()[0]=1;
    manifold_coherence_accumulate_forces(c.p,phase.p,omega.p,amp.p,pos.p,cw.p,width.p,anchor.p,weight.p,acc.p,starts.p,index.p,bin.p,1,heat.p,n,count.p,1,.2f,.5f,.25f,4,1e-6f,1,1,1,1);c.sync();
    auto a=acc.data()[0];near(a.fr,n,1e-3,"two-block coherence phasor sum");near(a.ws,n,1e-3,"coherence support");near(a.was,n,1e-3,"coherence amplitude mass");for(unsigned j=0;j<n;++j)near(heat.data()[j],.9,1e-6,"metabolic heat debit");
    SpectralModeParams p{};p.num_osc=n;p.max_carriers=1;p.dt=.01;p.coupling_scale=1;p.domain_x=p.domain_y=p.domain_z=1;p.spatial_sigma=1;
    manifold_coherence_update_oscillator_phases(c.p,phase.p,omega.p,amp.p,mr.p,mi.p,cw.p,width.p,anchor.p,weight.p,count.p,p,starts.p,index.p,bin.p,1,pos.p);c.sync();
    for(unsigned j=0;j<n;++j)near(phase.data()[j],.01,1e-6,"phase synchronization torque");
}
static void gpe(Context& c){
    constexpr unsigned n=16;Buffer<float> re(c,n),im(c,n),tr(c,n),ti(c,n),omega(c,n),width(c,n),op(c,1),oa(c,1),pos(c,3),aw(c,n*8);Buffer<unsigned> ai(c,n*8),count(c,{n});Buffer<Accum> acc(c,n);ai.fill(UINT32_MAX);width.fill(1);re.data()[n/2]=1;
    SpectralModeParams p{};p.max_carriers=n;p.offender_weight_floor=1e-6;
    GPEParams g{};g.dt=.002;g.hbar_eff=1;g.mass_eff=1;g.inv_domega2=4;g.energy_decay=.8;g.anchors=8;
    for(unsigned step=0;step<100;++step)manifold_coherence_gpe_step(c.p,op.p,omega.p,oa.p,re.p,im.p,omega.p,width.p,tr.p,ti.p,ai.p,aw.p,acc.p,count.p,pos.p,p,g,nullptr);c.sync();
    double norm=0;auto* rr=re.data();auto* ii=im.data();for(unsigned j=0;j<n;++j)norm+=double(rr[j])*rr[j]+double(ii[j])*ii[j];near(norm,std::exp(-2*double(g.energy_decay)*g.dt*100),1e-4,"GPE kinetic norm and damping");
    re.fill(0);im.fill(0);auto* a=acc.data();a[3].fr=2;a[3].was=2;g.energy_decay=0;g.mass_eff=0;
    manifold_coherence_gpe_step(c.p,op.p,omega.p,oa.p,re.p,im.p,omega.p,width.p,tr.p,ti.p,ai.p,aw.p,acc.p,count.p,pos.p,p,g,nullptr);c.sync();near(re.data()[3],g.dt,1e-7,"GPE source creates nonzero field");
}
static void collisions_and_hash(Context& c){
    Buffer<float> pos(c,{.02f,0,0,.98f,0,0}),vel(c,{-1,0,0,1,0,0}),vin(c,{-1,0,0,1,0,0}),mass(c,{1,1}),heat(c,2),hin(c,2),exc(c,2);
    manifold_particle_interactions(c.p,pos.p,vel.p,exc.p,mass.p,heat.p,vin.p,hin.p,.001f,.1f,0,0,1,1,1,1,1);c.sync();
    // Source-parity test: preserves the uploaded half-impulse behavior.
    near(vel.data()[0],0,1e-6,"source pair collision v0");near(vel.data()[3],0,1e-6,"source pair collision v1");
    std::copy(vin.data(),vin.data()+6,vel.data());heat.fill(0);
    Buffer<unsigned> cells(c,2),counts(c,64),starts(c,65),offsets(c,64),sorted(c,2);
    manifold_spatial_hash_assign(c.p,pos.p,cells.p,counts.p,4,4,4,.25f,0,0,0);scan(c,counts.p,starts.p,64);manifold_exclusive_scan_u32_finalize_total(c.p,counts.p,starts.p,64);c.sync();
    std::copy(starts.data(),starts.data()+64,offsets.data());
    manifold_spatial_hash_scatter(c.p,cells.p,sorted.p,offsets.p,2);
    manifold_spatial_hash_collisions(c.p,pos.p,vel.p,exc.p,mass.p,heat.p,sorted.p,starts.p,cells.p,vin.p,hin.p,4,4,4,.25f,0,0,0,.001f,.1f,0,0,1,1);c.sync();
    near(vel.data()[0],0,1e-6,"hash minimum-image collision v0");near(vel.data()[3],0,1e-6,"hash minimum-image collision v1");
}
static void binning_and_generation(Context& c){
    Buffer<float> omega(c,{-1,0,1}),bin(c,2);Buffer<unsigned> count(c,{3}),lo(c,{UINT32_MAX}),hi(c,{0}),counts(c,4),starts(c,5),offset(c,4),idx(c,3);
    manifold_coherence_reduce_omega_minmax_keys(c.p,omega.p,count.p,lo.p,hi.p);manifold_coherence_compute_bin_params(c.p,lo.p,hi.p,count.p,bin.p,.1f);manifold_coherence_bin_count(c.p,omega.p,count.p,counts.p,bin.p,4);scan(c,counts.p,starts.p,4);manifold_exclusive_scan_u32_finalize_total(c.p,counts.p,starts.p,4);c.sync();
    require(starts.data()[4]==3,"frequency bin population");std::copy(starts.data(),starts.data()+4,offset.data());manifold_coherence_bin_scatter(c.p,omega.p,count.p,offset.p,bin.p,4,idx.p);c.sync();std::vector<unsigned> got(idx.data(),idx.data()+3);std::sort(got.begin(),got.end());require(got==std::vector<unsigned>({0,1,2}),"frequency bin permutation");
    constexpr unsigned n=17;Buffer<float> p(c,n*3),v(c,n*3),e(c,n),h(c,n),x(c,n),m(c,n),rp(c,n*3),rr(c,n*4);rp.fill(.5f);rr.fill(.5f);
    ParticleGenParams prm{};prm.num_particles=n;prm.grid_x=prm.grid_y=prm.grid_z=8;prm.pattern=3;prm.energy_scale=1;
    manifold_generate_particles(c.p,p.p,v.p,e.p,h.p,x.p,m.p,rp.p,rr.p,prm);c.sync();
    for(unsigned j=0;j<n;++j){near(p.data()[3*j],4,0,"particle generation position");near(v.data()[3*j],0,0,"particle generation center");near(e.data()[j],.75,0,"particle generation energy");near(m.data()[j],.75,0,"particle generation mass");}
}
static void ownership(int device){
    Context other(device);Buffer<float> b(other,{1,2,3});
    // Closing a context before its buffers must not create a dangling owner.
    manifold_destroy_context(other.p);other.p=nullptr;
    require(manifold_get_buffer_pointer(b.p)==nullptr,"closed context denies host view");
}
int main(int argc,char** argv){
    int count=0;cudaError_t availability=cudaGetDeviceCount(&count);
    if(availability!=cudaSuccess || count==0){std::fprintf(stderr,"SKIP: CUDA runtime/device unavailable: %s\n",cudaGetErrorString(availability));return 77;}
    int device=argc>1?std::atoi(argv[1]):0;
    try{
        Context c(device);
        const std::pair<const char*,void(*)(Context&)> tests[]={
            {"diagnostics and scratch lifetime",diagnostics},{"hierarchical scans",scans},
            {"sort, PIC scatter and gather",scatter_and_gather},{"gas RK2",gas},
            {"projection and pilot wave",projection_pilot},{"coherence accumulation and phases",forces_and_phases},
            {"four-stage GPE",gpe},{"periodic collisions and hash",collisions_and_hash},
            {"frequency bins and generation",binning_and_generation}};
        for(auto [name,f]:tests){f(c);std::printf("PASS %s\n",name);}
        ownership(device);std::puts("PASS context/buffer close ordering");
        std::puts("10 CUDA device test groups passed.");return 0;
    }catch(const std::exception& e){std::fprintf(stderr,"FAIL: %s\n",e.what());return 1;}
}
