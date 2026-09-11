// Native acceptance driver: link with the actual Metal or CUDA host implementation.
// Never links the syntax-only test doubles. This file was not GPU-executed here.
#include "bridge.h"
#include <algorithm>
#include <cmath>
#include <cstdint>
#include <cstring>
#include <limits>
#include <cstdio>
#include <stdexcept>
#include <vector>
struct Buffer {
    ManifoldBuffer* p;
    template<class T> Buffer(ManifoldContext* c,const std::vector<T>& x):p(manifold_create_buffer(c,std::max<size_t>(1,x.size())*sizeof(T),x.empty()?nullptr:x.data())) {if(!p)throw std::runtime_error("buffer allocation");}
    ~Buffer(){manifold_destroy_buffer(p);}
    Buffer(const Buffer&)=delete;Buffer& operator=(const Buffer&)=delete;
    template<class T>T* data(){return static_cast<T*>(manifold_get_buffer_pointer(p));}
};
static void sync(ManifoldContext* c){if(!manifold_synchronize_checked(c))throw std::runtime_error(manifold_last_error(c));}
static void check(bool b,const char* s){if(!b)throw std::runtime_error(s);}
static void scans(ManifoldContext* c){
    for(unsigned n:{0u,1u,7u,255u,256u,257u,65537u}){
        std::vector<uint32_t> x(n);for(unsigned i=0;i<n;i++)x[i]=(i*7)%13;
        Buffer in(c,x),out(c,std::vector<uint32_t>(n+2,0xabcddcba));
        check(manifold_exclusive_scan_u32(c,in.p,out.p,n),"scan submission");sync(c);
        auto* y=out.data<uint32_t>();uint32_t sum=0;
        for(unsigned i=0;i<n;i++){check(y[i]==sum,"exclusive prefix mismatch");sum+=x[i];}
        check(y[n]==sum&&y[n+1]==0xabcddcba,"scan total/canary mismatch");
    }
    std::puts("PASS native hierarchical scan and output guard");
}
static uint32_t ordered_key(float x){
    uint32_t bits;std::memcpy(&bits,&x,sizeof(bits));
    return (bits&0x80000000u)?~bits:(bits^0x80000000u);
}
static void omega_extrema(ManifoldContext* c){
    const float nan=std::numeric_limits<float>::quiet_NaN(),inf=std::numeric_limits<float>::infinity();
    struct Case{std::vector<float> values;uint32_t active;};
    std::vector<Case> cases={{{},0},{{nan,inf,-inf},3},{{3},1},{{-3},1},
        {{+0.0f,-0.0f},2},{{2,-3,9,-1e35f,1e35f,nan},3}};
    std::vector<float> large(65554);
    for(unsigned j=0;j<65537;j++)large[j]=float(int(j%2001)-1000)*.125f;
    large[3]=nan;large[19]=inf;large[65537]=-1e35f;large[65538]=1e35f;
    cases.push_back({large,65537});
    for(const auto& test:cases){
        uint32_t expected_min=UINT32_MAX,expected_max=0;
        for(uint32_t j=0;j<test.active;j++)if(std::isfinite(test.values[j])){
            expected_min=std::min(expected_min,ordered_key(test.values[j]));
            expected_max=std::max(expected_max,ordered_key(test.values[j]));
        }
        Buffer omega(c,test.values),num(c,std::vector<uint32_t>{test.active});
        for(unsigned repeat=0;repeat<4;repeat++){
            Buffer low(c,std::vector<uint32_t>{UINT32_MAX,0xdeadbeef}),high(c,std::vector<uint32_t>{0,0xdeadbeef});
            manifold_coherence_reduce_omega_minmax_keys(c,omega.p,num.p,low.p,high.p);sync(c);
            const auto* lo=low.data<uint32_t>();const auto* hi=high.data<uint32_t>();
            check(lo[0]==expected_min&&hi[0]==expected_max,"omega extrema/filter/count mismatch");
            check(lo[1]==0xdeadbeef&&hi[1]==0xdeadbeef,"omega reduction output guard overwritten");
        }
    }
    std::puts("PASS native finite omega reduction, signed zero, empty/all-invalid input, active-tail guards and reset cycles");
}
static void fft(ManifoldContext* c){
    const double pi=std::acos(-1.);
    for(unsigned n:{1u,8u,127u,256u,257u,4093u,4096u}){
        const unsigned mode=n==1?0:3;std::vector<float> re(n),im(n);
        for(unsigned j=0;j<n;j++){double a=2*pi*mode*j/n;re[j]=std::cos(a);im[j]=std::sin(a);}
        Buffer r(c,re),i(c,im),kr(c,std::vector<float>(n)),ki(c,std::vector<float>(n));
        Buffer a(c,std::vector<ManifoldCarrierAccumulator>(n)),num(c,std::vector<uint32_t>{n});
        SpectralModeParams p{};p.max_carriers=n;GPEParams gp{};gp.hbar_eff=.7f;gp.mass_eff=1.3f;gp.dt=.025f;gp.inv_domega2=4;
        manifold_coherence_gpe_step(c,nullptr,nullptr,nullptr,r.p,i.p,nullptr,nullptr,kr.p,ki.p,nullptr,nullptr,a.p,num.p,nullptr,p,gp,nullptr);sync(c);
        auto* rr=r.data<float>();auto* ii=i.data<float>();double err=0,norm=0;
        double sn=std::sin(pi*mode/n);double phase=gp.hbar_eff/(2*gp.mass_eff)*(-4*sn*sn*gp.inv_domega2)*gp.dt;
        for(unsigned j=0;j<n;j++){double angle=2*pi*mode*j/n+phase;err=std::max(err,std::hypot(rr[j]-std::cos(angle),ii[j]-std::sin(angle)));norm+=double(rr[j])*rr[j]+double(ii[j])*ii[j];}
        std::printf("native FFT N=%u max_error=%.9g relative_norm_error=%.9g\n",n,err,std::abs(norm/n-1));
        check(err<3e-5&&std::abs(norm/n-1)<1e-5,"FFT plane wave dispersion/norm mismatch");
    }
}
static void accumulations(ManifoldContext* c){
    constexpr unsigned n=512;std::vector<float> amps(n,1);amps[60]=amps[300]=3;
    double expected_w=0,expected_force=0;for(float x:amps){expected_w+=x;expected_force+=x*x;}
    for(unsigned m:{1u,257u}){
        std::vector<uint32_t> indices(m);for(unsigned j=0;j<m;j++)indices[j]=j;
        std::vector<uint32_t> anchors(8*m,UINT32_MAX);std::vector<float> weights(8*m,0);
        for(unsigned j=0;j<m;j++){anchors[8*j]=0;weights[8*j]=1;}
        Buffer phase(c,std::vector<float>(n)),omega(c,std::vector<float>(n)),amp(c,amps),pos(c,std::vector<float>(3*n));
        Buffer modes(c,std::vector<float>(m)),gates(c,std::vector<float>(m,1)),ai(c,anchors),aw(c,weights);
        Buffer a(c,std::vector<ManifoldCarrierAccumulator>(m)),starts(c,std::vector<uint32_t>{0,m}),idx(c,indices);
        Buffer bp(c,std::vector<float>{0,1}),heat(c,std::vector<float>(n)),num(c,std::vector<uint32_t>{m});
        for(unsigned repeat=0;repeat<20;repeat++){
            manifold_clear_field(c,a.p);
            manifold_coherence_accumulate_forces(c,phase.p,omega.p,amp.p,pos.p,modes.p,gates.p,ai.p,aw.p,a.p,starts.p,idx.p,bp.p,1,heat.p,n,num.p,m,0,0,1,1,0,10,10,10,1);
            sync(c);auto* rows=a.data<ManifoldCarrierAccumulator>();
            for(unsigned j=0;j<m;j++){
                check(rows[j].w_sum==expected_w&&rows[j].force_r==expected_force,"native atomic sum mismatch");
                check(manifold_offender_index(rows[j].packed_offender)==60&&manifold_offender_weight(rows[j].packed_offender)==3,"native packed offender mismatch");
            }
        }
    }
    std::puts("PASS native local/global float atomics and packed offender contention");
}
static void legacy_pair(ManifoldContext* c){
    Buffer pos(c,std::vector<float>{.5f,0,0,1.25f,0,0}),vin(c,std::vector<float>{1,0,0,-1,0,0});
    Buffer hin(c,std::vector<float>(2)),vout(c,std::vector<float>(6)),hout(c,std::vector<float>(2));
    Buffer mass(c,std::vector<float>(2,1)),ex(c,std::vector<float>(2));
    manifold_particle_interactions(c,pos.p,vout.p,ex.p,mass.p,hout.p,vin.p,hin.p,.001f,.5f,1000,0,1,1,10,10,10);sync(c);
    auto* v=vout.data<float>();check(std::abs(v[0]+1)<1e-6&&std::abs(v[3]-1)<1e-6,"legacy impulse still double-applies spring");
    std::puts("PASS native isolated restitution pair (nonzero legacy Young modulus)");
}
#include "coupled_gpu_cases.inc"
#include "completion_gpu_cases.inc"
int main(int argc,char** argv){
    ManifoldContext* c=nullptr;
#ifdef MF_TEST_METAL
    if(argc!=2){std::fprintf(stderr,"usage: runtime_gpu_smoke path/to/manifold.metallib\n");return 2;}
    c=manifold_create_context(argv[1]);
#else
    (void)argc;(void)argv;char error[512]{};c=manifold_create_cuda_context(0,error,sizeof(error));
    if(!c)std::fprintf(stderr,"%s\n",error);
#endif
    if(!c){std::fprintf(stderr,"GPU context creation failed\n");return 2;}
    int result=0;try{omega_extrema(c);scans(c);fft(c);accumulations(c);legacy_pair(c);coupled_native(c);coupled_native_wave_ledger(c);completion_native(c);}catch(const std::exception& e){std::fprintf(stderr,"FAIL %s\n",e.what());result=1;}
    if(result==0){
#ifdef MF_TEST_METAL
        std::puts("NATIVE_ACCEPTANCE_BACKEND Metal");
#else
        std::puts("NATIVE_ACCEPTANCE_BACKEND CUDA");
#endif
    }
    manifold_destroy_context(c);return result;
}
