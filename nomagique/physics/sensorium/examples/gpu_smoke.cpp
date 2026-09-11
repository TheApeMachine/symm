// Link this against the REAL backend. No CPU kernel emulation in this driver.
#include "bridge.h"
#include <cmath>
#include <cstdio>
#include <cstring>
#include <stdexcept>
#include <vector>

static void require(bool ok,const char* message) { if(!ok) throw std::runtime_error(message); }
class Buffer {
    ManifoldBuffer* p_=nullptr;
public:
    Buffer(ManifoldContext* c,uint64_t bytes,const void* initial=nullptr):p_(manifold_create_buffer(c,bytes,initial)) {
        require(p_!=nullptr,"buffer allocation");
    }
    ~Buffer(){manifold_destroy_buffer(p_);}
    Buffer(const Buffer&)=delete;
    Buffer& operator=(const Buffer&)=delete;
    operator ManifoldBuffer*()const{return p_;}
    template<class T> const T* read(ManifoldContext* c)const {
        manifold_synchronize(c);
        const auto* v=static_cast<const T*>(manifold_get_buffer_pointer(p_));
        require(v!=nullptr,"buffer readback");return v;
    }
};
static void run(ManifoldContext* c) {
    require(manifold_physics_abi_version()==2,"ABI version");
    constexpr unsigned n=64;
    MFHydroParamsV2 p={n,4,4,4,1,0.001f,1.4f,1,0.1f,0,0.1f,0.001f,0.1f,0.4f,1,0};
    std::vector<MFHydroStateV2> initial(n);
    for(auto& s:initial) { s={{1,0.25f,0,0,2.53125f,2.5f}}; }
    Buffer in(c,n*sizeof(MFHydroStateV2),initial.data()),out(c,n*sizeof(MFHydroStateV2));
    Buffer w1(c,n*sizeof(MFHydroStateV2)),w2(c,n*sizeof(MFHydroStateV2)),status(c,n*sizeof(unsigned));
    require(manifold_hydro_step_v2(c,in,out,w1,w2,status,nullptr,&p),"uniform hydro GPU step");
    const auto* data=out.read<MFHydroStateV2>(c);
    for(unsigned i=0;i<n;++i) for(unsigned k=0;k<6;++k)
        require(std::abs(data[i].q[k]-initial[i].q[k])<1e-6f,"uniform GPU state invariant");
    std::vector<MFHydroStateV2> saved(data,data+n);
    p.dt=100;
    require(!manifold_hydro_step_v2(c,in,out,w1,w2,status,nullptr,&p),"CFL rejection");
    require(std::memcmp(saved.data(),out.read<MFHydroStateV2>(c),n*sizeof(MFHydroStateV2))==0,"rejected GPU step changed output");
    p.dt=0.001f;
    require(manifold_hydro_step_v2(c,in,out,w1,w2,status,nullptr,&p),"retry after numerical rejection");
    constexpr unsigned nw=15;
    std::vector<MFWaveValueV2> psi(nw);std::vector<float> V(nw,0);
    double N0=0;for(unsigned i=0;i<nw;++i){psi[i]={std::cos(float(i)),std::sin(float(i))};N0+=double(psi[i].re)*psi[i].re+double(psi[i].im)*psi[i].im;}
    Buffer wave(c,nw*sizeof(MFWaveValueV2),psi.data()),wo(c,nw*sizeof(MFWaveValueV2)),wa(c,nw*sizeof(MFWaveValueV2)),wb(c,nw*sizeof(MFWaveValueV2));
    Buffer potential(c,nw*sizeof(float),V.data()),ws(c,nw*sizeof(unsigned));
    MFWaveParamsV2 wp={nw,5,3,1,1,0.01f,1,1,0.1f};
    for(unsigned k=0;k<100;++k) require(manifold_wave_step_v2(c,wave,wave,wa,wb,potential,ws,&wp),"odd-grid spatial GPE GPU step");
    const auto* z=wave.read<MFWaveValueV2>(c);double N=0;
    for(unsigned i=0;i<nw;++i)N+=double(z[i].re)*z[i].re+double(z[i].im)*z[i].im;
    require(std::abs(N-N0)<1e-3,"GPU wave norm drift");
    MFParticleStateV2 particles[2]={{{-0.45f,0,0},{0,0,0},1,1},{{0.45f,0,0},{0,0,0},2,2}};
    Buffer ci(c,sizeof particles,particles),co(c,sizeof particles),ca(c,sizeof particles),cb(c,sizeof particles),cs(c,2*sizeof(unsigned));
    MFContactParamsV2 cp={2,0.001f,0.5f,100,0.25f,0.01f,0.1f,1,0,0,0};
    require(manifold_contact_step_v2(c,ci,co,ca,cb,cs,&cp),"Hertz GPU step");
    const auto* q=co.read<MFParticleStateV2>(c);
    require(std::abs(q[0].mass*q[0].v[0]+q[1].mass*q[1].v[0])<1e-6f,"GPU contact momentum");
    std::printf("PASS GPU uniform hydro, transactional rejection/retry, odd-grid wave norm, and contact momentum\n");
}
int main(int argc,char** argv) {
    ManifoldContext* ctx=nullptr;
#ifdef MF_TEST_METAL
    if(argc!=2){std::fprintf(stderr,"usage: gpu_smoke path/to/manifold.metallib\n");return 2;}
    ctx=manifold_create_context(argv[1]);
#else
    (void)argc;(void)argv;char error[512]={};
    ctx=manifold_create_cuda_context(0,error,sizeof error);
    if(!ctx)std::fprintf(stderr,"%s\n",error);
#endif
    if(!ctx){std::fprintf(stderr,"GPU context unavailable\n");return 2;}
    int code=0;
    try{run(ctx);}catch(const std::exception& e){std::fprintf(stderr,"FAIL %s\n",e.what());code=1;}
    manifold_destroy_context(ctx);return code;
}
