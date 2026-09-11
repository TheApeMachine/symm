// Included by runtime_gpu_smoke.cpp. Actual native APIs; independent analytic
// oracles. These acceptance cases are supplied, NOT claimed hardware-executed.
static MFHydroParamsV2 coupled_params(unsigned x,unsigned y,unsigned z){
    MFHydroParamsV2 p{};p.n=x*y*z;p.nx=x;p.ny=y;p.nz=z;p.dx=.25f;p.dt=.0001f;
    p.gamma=1.4f;p.cv=1;p.eta_pressure=.001f;p.eta_sync=.1f;p.cfl=.4f;p.reconstruction=1;return p;
}
static void coupled_native(ManifoldContext* c){
    auto p=coupled_params(3,5,7);const double pi=std::acos(-1.);const float G=.125f;
    std::vector<MFHydroStateV2> initial(p.n);
    for(unsigned x=0;x<p.nx;x++)for(unsigned y=0;y<p.ny;y++)for(unsigned z=0;z<p.nz;z++){
        unsigned i=z+p.nz*(y+p.ny*x);initial[i].q[0]=float(1+.2*std::cos(2*pi*(double(x)/p.nx+double(y)/p.ny+double(z)/p.nz)));
        initial[i].q[4]=initial[i].q[5]=2;
    }
    Buffer u(c,initial),spectrum(c,std::vector<float>(2*p.n)),phi(c,std::vector<float>(p.n)),g(c,std::vector<float>(3*p.n)),flags(c,std::vector<uint32_t>(p.n));
    check(manifold_periodic_poisson(c,u.p,spectrum.p,phi.p,g.p,flags.p,&p,G),"coupled Poisson submission");sync(c);
    double lambda=0;for(auto d:{p.nx,p.ny,p.nz})lambda-=4*std::pow(std::sin(pi/d),2)/(p.dx*p.dx);
    double A=4*pi*G*.2/lambda,err=0;
    auto potential=phi.data<float>();auto accel=g.data<float>();
    for(unsigned x=0;x<p.nx;x++)for(unsigned y=0;y<p.ny;y++)for(unsigned z=0;z<p.nz;z++){
        unsigned i=z+p.nz*(y+p.ny*x),j=x+p.nx*(y+p.ny*z);double a=2*pi*(double(x)/p.nx+double(y)/p.ny+double(z)/p.nz);
        err=std::max(err,std::abs(potential[j]-A*std::cos(a)));
        unsigned dims[3]={p.nx,p.ny,p.nz};for(unsigned d=0;d<3;d++)check(std::abs(accel[3*i+d]-A*std::sin(a)*std::sin(2*pi/dims[d])/p.dx)<3e-6,"Poisson acceleration/index layout");
    }
    check(err<3e-6,"native Poisson Fourier solution");
    check(manifold_gravity_kick(c,u.p,g.p,flags.p,&p),"gravity kick");sync(c);auto state=u.data<MFHydroStateV2>();
    for(unsigned i=0;i<p.n;i++){double kinetic=0;for(unsigned a=0;a<3;a++){double m=initial[i].q[0]*p.dt*accel[3*i+a];check(std::abs(state[i].q[1+a]-m)<2e-7,"gravity momentum work");kinetic+=m*m/(2*initial[i].q[0]);}check(std::abs(state[i].q[4]-2-kinetic)<2e-6&&state[i].q[5]==2,"gravity must not directly heat");}
    check(manifold_periodic_poisson(c,u.p,spectrum.p,phi.p,g.p,flags.p,&p,0),"zero gravity");sync(c);for(unsigned i=0;i<3*p.n;i++)check(g.data<float>()[i]==0,"disabled gravity must clear force");
    std::printf("PASS native noncubic Poisson/kick, maximum potential error %.9g\n",err);
    std::vector<float> pos(3*p.n),vel(3*p.n),mass(p.n,.03125f),heat(p.n,.0625f);
    for(unsigned x=0;x<p.nx;x++)for(unsigned y=0;y<p.ny;y++)for(unsigned z=0;z<p.nz;z++){
        unsigned i=z+p.nz*(y+p.ny*x);pos[3*i]=x*p.dx;pos[3*i+1]=y*p.dx;pos[3*i+2]=z*p.dx;vel[3*i]=.25f;vel[3*i+1]=-.5f;vel[3*i+2]=.125f;
    }
    Buffer x(c,pos),v(c,vel),m(c,mass),q(c,heat),xo(c,std::vector<float>(3*p.n)),vo(c,std::vector<float>(3*p.n)),qo(c,std::vector<float>(p.n));
    check(manifold_pic_deposit_dual(c,x.p,v.p,m.p,q.p,u.p,flags.p,p.n,&p),"dual PIC deposit");sync(c);state=u.data<MFHydroStateV2>();
    for(unsigned i=0;i<p.n;i++){check(std::abs(state[i].q[0]-2)<1e-6,"PIC density");check(std::abs(state[i].q[5]-4)<1e-6,"PIC heat");check(std::abs(state[i].q[4]-(4+.25*.25+.5*.5+.125*.125))<1e-6,"PIC total energy");}
    check(manifold_pic_gather_dual(c,x.p,m.p,xo.p,vo.p,qo.p,u.p,flags.p,p.n,&p),"dual PIC gather");sync(c);
    for(unsigned i=0;i<p.n;i++){check(std::abs(qo.data<float>()[i]-heat[i])<1e-6,"PIC roundtrip heat");for(unsigned a=0;a<3;a++)check(std::abs(vo.data<float>()[3*i+a]-vel[3*i+a])<1e-6,"PIC roundtrip velocity");}
    std::puts("PASS native dual-energy PIC uniform roundtrip");
}
static void coupled_native_wave_ledger(ManifoldContext* c){
    constexpr unsigned n=7;std::vector<ManifoldCarrierAccumulator> accum(n);for(auto& a:accum){a.force_r=.2f;a.force_i=.1f;a.w_amp_sum=1;}
    Buffer r(c,std::vector<float>(n,1)),im(c,std::vector<float>(n)),kr(c,std::vector<float>(n)),ki(c,std::vector<float>(n)),a(c,accum),count(c,std::vector<uint32_t>{n}),ledger(c,std::vector<float>(6*n));
    SpectralModeParams p{};p.max_carriers=n;GPEParams gp{};gp.hbar_eff=1;gp.mass_eff=1;gp.dt=.1f;gp.energy_decay=.3f;gp.inv_domega2=4;
    manifold_coherence_gpe_step(c,nullptr,nullptr,nullptr,r.p,im.p,nullptr,nullptr,kr.p,ki.p,nullptr,nullptr,a.p,count.p,nullptr,p,gp,nullptr);sync(c);
    check(manifold_coherence_copy_ledger(c,ledger.p,n),"native wave ledger read");sync(c);const float* l=ledger.data<float>();double damp=std::exp(-gp.energy_decay*gp.dt);
    for(unsigned j=0;j<n;j++){check(std::abs(l[6*j]-1)<3e-6&&std::abs(l[6*j+1])<3e-6,"conservative stage ledger");check(std::abs(l[6*j+2]-damp)<3e-6&&std::abs(l[6*j+3])<3e-6,"amplitude damping ledger");check(std::abs(l[6*j+4]-(damp+.02))<3e-6&&std::abs(l[6*j+5]-.01)<3e-6,"drive work ledger");}
    std::puts("PASS native actual conservative/damping/drive state ledger");
}
