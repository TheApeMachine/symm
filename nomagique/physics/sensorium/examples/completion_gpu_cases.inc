// Actual native API acceptance cases, NOT CPU protocol doubles. The independent
// finite-volume oracle is used only in this host test executable.
#include "../tests/independent_hydro.h"
static void parity(const char* name,const float* p,unsigned n){
    std::printf("PARITY %s",name);for(unsigned i=0;i<n;++i){check(std::isfinite(p[i]),"nonfinite parity value");std::printf(" %.9g",p[i]);}std::puts("");
}
static void completion_remap(ManifoldContext* c){
    auto p=coupled_params(4,3,2);unsigned n=p.n;std::vector<float>x(3*n),v(3*n),m(n),q(n),e(n);
    for(unsigned i=0;i<n;++i){x[3*i]=(i/(p.ny*p.nz)+.31f)*p.dx;x[3*i+1]=(i/p.nz%p.ny+.23f)*p.dx;x[3*i+2]=(i%p.nz+.43f)*p.dx;
        m[i]=.01f+.0002f*(i%5);q[i]=.03f+.002f*(i%7);v[3*i]=.1f*std::sin(float(i));v[3*i+1]=.2f;v[3*i+2]=-.1f;
        e[i]=q[i]+.5f*m[i]*(v[3*i]*v[3*i]+.05f);}
    Buffer X(c,x),V(c,v),M(c,m),Q(c,q),E(c,e),u(c,std::vector<MFHydroStateV2>(n)),f(c,std::vector<unsigned>(n)),vo(c,std::vector<float>(3*n)),qo(c,std::vector<float>(n)),eo(c,std::vector<float>(n)),report(c,std::vector<float>(13,7654));
    check(manifold_pic_deposit_material(c,X.p,V.p,M.p,Q.p,E.p,u.p,f.p,n,&p),"material deposit");sync(c);
    check(manifold_pic_remap_conservative(c,u.p,X.p,M.p,vo.p,qo.p,eo.p,report.p,f.p,n,&p,1,2e-5f,4096,0),"nonuniform conservative remap");sync(c);
    double before=0,after=0,p0[3]{},p1[3]{};for(unsigned i=0;i<n;++i){before+=e[i];after+=eo.data<float>()[i];check(qo.data<float>()[i]>=0,"remap auxiliary heat");for(unsigned a=0;a<3;++a){p0[a]+=double(m[i])*v[3*i+a];p1[a]+=double(m[i])*vo.data<float>()[3*i+a];}}
    check(std::abs(after-before)<2e-4*before,"native remap total energy");for(unsigned a=0;a<3;++a)check(std::abs(p0[a]-p1[a])<1e-5,"native remap momentum");check(report.data<float>()[12]==7654,"remap report overflow");
    parity("remap.velocity",vo.data<float>(),3*n);parity("remap.total_energy",eo.data<float>(),n);
    M.data<float>()[0]=-1;check(!manifold_pic_remap_conservative(c,u.p,X.p,M.p,vo.p,qo.p,eo.p,report.p,f.p,n,&p,1,2e-5f,4096,0),"bad remap mass accepted");sync(c);check(f.data<unsigned>()[0]==MF_PHYSICS_BAD_STATE,"remap failure reclassified as retryable");
    std::puts("PASS native nonuniform constrained remap, conservation, guard and fatal-state status");
}
static void completion_pilot(ManifoldContext* c){
    auto p=coupled_params(8,2,3);p.dt=.002f;const unsigned np=3;
    std::vector<float>pos={.17f,.13f,.11f,.6f,.22f,.31f,1.8f,.35f,.6f};
    Buffer x(c,pos),m(c,std::vector<float>(np,1)),old(c,std::vector<float>(np*3)),r0(c,std::vector<float>(p.n,1)),i0(c,std::vector<float>(p.n)),r1(c,std::vector<float>(p.n,.99995f)),i1(c,std::vector<float>(p.n,.01f)),xo(c,std::vector<float>(np*3)),vo(c,std::vector<float>(np*3)),report(c,std::vector<float>(np*4+1,7654)),f(c,std::vector<unsigned>(np));
    check(manifold_pilot_checked_time(c,x.p,m.p,old.p,r0.p,i0.p,r1.p,i1.p,xo.p,vo.p,report.p,f.p,np,&p,1,1e-5f,.25f),"checked space-time pilot");sync(c);
    for(unsigned i=0;i<3*np;++i){check(std::abs(xo.data<float>()[i]-pos[i])<1e-6,"constant spatial phase moved particle");check(vo.data<float>()[i]==0,"constant phase current");}check(report.data<float>()[4*np]==7654,"pilot report guard");parity("pilot.position",xo.data<float>(),3*np);
    std::fill(r0.data<float>(),r0.data<float>()+p.n,0);check(!manifold_pilot_checked_time(c,x.p,m.p,old.p,r0.p,i0.p,r1.p,i1.p,xo.p,vo.p,report.p,f.p,np,&p,1,1e-5f,.25f),"exact node accepted");sync(c);check(f.data<unsigned>()[0]!=0,"node failure missing");
    std::puts("PASS native space-time guidance and exact-node rejection");
}
static void completion_contact(ManifoldContext* c){
    auto p=coupled_params(8,3,3);MFContactParamsV2 cp{};cp.n=2;cp.dt=.0001f;cp.radius=.075f;cp.young_modulus=200;cp.poisson_ratio=.25f;cp.cv=1;cp.domain_x=2;cp.domain_y=cp.domain_z=.75f;
    Buffer x(c,std::vector<float>{.4f,.3f,.3f,.53f,.3f,.3f}),v(c,std::vector<float>(6)),m(c,std::vector<float>{1,2}),q(c,std::vector<float>{2,3}),vo(c,std::vector<float>(6)),qo(c,std::vector<float>(2)),d(c,std::vector<float>(15,7654)),f(c,std::vector<unsigned>(2));
    check(manifold_contact_hash_kick(c,x.p,v.p,m.p,q.p,vo.p,qo.p,d.p,f.p,&p,&cp,0),"native Hertz hash kick");sync(c);
    double overlap=2*double(cp.radius)-(double(.53f)-double(.4f)),E=cp.young_modulus/(2*(1-double(cp.poisson_ratio)*cp.poisson_ratio));double force=4./3*E*std::sqrt(cp.radius/2.)*std::pow(overlap,1.5);
    check(std::abs(vo.data<float>()[0]+cp.dt*force)<1e-7,"Hertz material force");check(std::abs(vo.data<float>()[3]-cp.dt*force/2)<1e-7,"Hertz own pair reaction");check(qo.data<float>()[0]==2&&qo.data<float>()[1]==3,"elastic contact directly heated");check(d.data<float>()[14]==7654,"contact report guard");parity("contact.velocity",vo.data<float>(),6);
    std::puts("PASS native compliant Hertz hash force and pair momentum");
}
static double periodic_gaussian(double x,double sigma,double L){double num=0,den=0;for(int k=-20;k<=20;++k){num+=std::exp(-std::pow((x+k*L)/(2*sigma),2));den+=std::exp(-std::pow(k*L/(2*sigma),2));}return num/den;}
static void completion_reciprocal(ManifoldContext* c){
    auto p=coupled_params(8,8,8);std::vector<float>positions{.7f,.6f,.5f,1.05f,.6f,.5f};float sigma=.2f,domega=.3f;
    Buffer x(c,positions),prev(c,positions),amp(c,std::vector<float>{0,2}),omega(c,std::vector<float>(2)),modes(c,std::vector<float>{0}),width(c,std::vector<float>{1}),idx(c,std::vector<unsigned>{0}),weights(c,std::vector<float>{1}),re(c,std::vector<float>{1}),im(c,std::vector<float>{0}),ledger(c,std::vector<float>{1,0,1,0,1,0}),force(c,std::vector<float>(6)),potential(c,std::vector<float>{0}),flags(c,std::vector<unsigned>(2));
    check(manifold_coherence_reciprocal(c,x.p,prev.p,amp.p,omega.p,modes.p,width.p,idx.p,weights.p,re.p,im.p,ledger.p,force.p,potential.p,flags.p,2,1,1,sigma,domega,&p),"reciprocal head");sync(c);
    const double dx=double(positions[3])-positions[0],epsilon=1e-5;double gradient=(periodic_gaussian(dx+epsilon,sigma,2)-periodic_gaussian(dx-epsilon,sigma,2))/(2*epsilon),expected=2*domega*gradient;
    check(std::abs(force.data<float>()[3]-expected)<2e-5,"reciprocal force independent potential derivative");check(std::abs(force.data<float>()[0]+expected)<2e-5,"anchor reaction missing");check(std::abs(potential.data<float>()[0]+2*periodic_gaussian(dx,sigma,2))<2e-5,"reference interaction potential");parity("reciprocal.force",force.data<float>(),6);
    std::puts("PASS native reciprocal force and anchor reaction against independent Hamiltonian gradient");
}
static void completion_geometry(ManifoldContext* c){
    const unsigned n=7;std::vector<float>re(n),im(n),w(n),V(n);double initial=0;for(unsigned j=0;j<n;++j){re[j]=.7f+.1f*std::cos(float(j));im[j]=.1f*std::sin(float(j));w[j]=.8f+.1f*(j%4);V[j]=.1f*(j%3);initial+=double(re[j])*re[j]+double(im[j])*im[j];}
    Buffer r(c,re),i(c,im),volume(c,w),pot(c,V),kr(c,std::vector<float>(n)),ki(c,std::vector<float>(n)),acc(c,std::vector<ManifoldCarrierAccumulator>(n)),num(c,std::vector<unsigned>{n});SpectralModeParams p{};p.max_carriers=n;GPEParams gp{};gp.hbar_eff=.8f;gp.mass_eff=1.2f;gp.dt=.003f;gp.inv_domega2=4;gp.g_interaction=.5f;
    auto step=[&](){manifold_coherence_gpe_step_geometry(c,nullptr,nullptr,nullptr,r.p,i.p,nullptr,nullptr,kr.p,ki.p,nullptr,nullptr,acc.p,num.p,nullptr,p,gp,pot.p,volume.p);sync(c);};
    for(unsigned k=0;k<40;++k)step();double norm=0;for(unsigned j=0;j<n;++j)norm+=double(r.data<float>()[j])*r.data<float>()[j]+double(i.data<float>()[j])*i.data<float>()[j];check(std::abs(norm-initial)<3e-5*initial,"metric physical norm");parity("geometry.real",r.data<float>(),n);parity("geometry.imag",i.data<float>(),n);
    gp.dt=-gp.dt;for(unsigned k=0;k<40;++k)step();for(unsigned j=0;j<n;++j)check(std::hypot(r.data<float>()[j]-re[j],i.data<float>()[j]-im[j])<3e-5,"metric time reversal");
    std::puts("PASS native positive metric, real scalar potential, physical norm and reversal");
}
static void completion_hydro_oracle(ManifoldContext* c){
    auto p=coupled_params(4,3,5);p.mu=.03f;p.bulk_viscosity=.01f;p.k_thermal=.02f;std::vector<MFHydroStateV2>u(p.n);std::vector<oracle::State>r(p.n);
    for(unsigned i=0;i<p.n;++i){double rho=1+.1*std::sin(double(i)),v[3]={.2*std::sin(.3*i),.1*std::cos(.7*i),.03},q=2+.1*std::cos(double(i)),k=0;u[i].q[0]=float(rho);for(unsigned a=0;a<3;++a){u[i].q[a+1]=float(rho*v[a]);k+=.5*rho*v[a]*v[a];}u[i].q[4]=float(q+k);u[i].q[5]=float(q);for(unsigned a=0;a<6;++a)r[i][a]=u[i].q[a];}
    oracle::Params op{p.nx,p.ny,p.nz,p.dx,p.gamma,p.cv,p.mu,p.bulk_viscosity,p.k_thermal,p.eta_pressure,p.eta_sync,true,false};auto expected=oracle::advance(r,p.dt,op,std::vector<oracle::Vec>(p.n));
    Buffer in(c,u),out(c,std::vector<MFHydroStateV2>(p.n)),stage(c,std::vector<MFHydroStateV2>(p.n)),work(c,std::vector<MFHydroStateV2>(p.n)),flags(c,std::vector<unsigned>(p.n));check(manifold_hydro_step_v2(c,in.p,out.p,stage.p,work.p,flags.p,nullptr,&p),"native complete NSF step");sync(c);double err=0;
    auto actual=out.data<MFHydroStateV2>();for(unsigned i=0;i<p.n;++i)for(unsigned a=0;a<6;++a)err=std::max(err,std::abs(actual[i].q[a]-expected[i][a]));check(err<3e-6,"native vs independent full hydro RK2");parity("hydro.state",reinterpret_cast<float*>(actual),6*p.n);std::printf("PASS native complete NSF against independent RHS/RK2, max_abs_error %.9g\n",err);
}
static void completion_native(ManifoldContext* c){completion_remap(c);completion_pilot(c);completion_contact(c);completion_reciprocal(c);completion_geometry(c);completion_hydro_oracle(c);}
