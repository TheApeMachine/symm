if(gid>=particles)return;
MFCIC shape=mc_cic(pos[3*gid],pos[3*gid+1],pos[3*gid+2],p);
if(shape.status||!MF_FINITE(mass[gid])||mass[gid]<=0){status[gid]=MF_PHYSICS_BAD_STATE;return;}
MFHydroStateV2 sample=mc_sample(state,shape);MFPrimitive v=mf_primitive(sample,p);
if(v.status||!(v.rho>0)){status[gid]=MF_PHYSICS_BAD_STATE;return;}
float q=mass[gid]*(v.thermal/v.rho);
if(!MF_FINITE(q)||q<0){status[gid]=MF_PHYSICS_BAD_STATE;return;}
unsigned n[3]={p.nx,p.ny,p.nz};
for(unsigned a=0;a<3;++a){float x=pos[3*gid+a]+p.dt*v.u[a];if(!MF_FINITE(x)){status[gid]=MF_PHYSICS_BAD_STATE;return;}}
for(unsigned a=0;a<3;++a){pos_out[3*gid+a]=mc_wrap(pos[3*gid+a]+p.dt*v.u[a],float(n[a])*p.dx);vel_out[3*gid+a]=v.u[a];}
heat_out[gid]=q;status[gid]=0;
