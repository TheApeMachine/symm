if(gid>=particles)return;
unsigned bad=mc_particle_valid(pos,vel,mass[gid],heat[gid],gid);
MFCIC shape=mc_cic(pos[3*gid],pos[3*gid+1],pos[3*gid+2],p);
if(bad||shape.status){status[gid]=MF_PHYSICS_BAD_STATE;return;}
float extensive[6]={mass[gid],mass[gid]*vel[3*gid],mass[gid]*vel[3*gid+1],mass[gid]*vel[3*gid+2],heat[gid],heat[gid]};
for(unsigned a=0;a<3;++a)extensive[4]+=.5f*mass[gid]*vel[3*gid+a]*vel[3*gid+a];
float invvol=1/(p.dx*p.dx*p.dx);
for(unsigned j=0;j<6;++j)if(!MF_FINITE(extensive[j]*invvol)){status[gid]=MF_PHYSICS_BAD_STATE;return;}
status[gid]=0;
for(unsigned k=0;k<8;++k)for(unsigned j=0;j<6;++j)MC_ADD(out+6*shape.index[k]+j,extensive[j]*invvol*shape.weight[k]);
