// Reconstruct the same periodic thermal overlap used by oscillator coupling.
// CIC point splats leave most material particles outside the wave's support;
// they cannot define a pilot field for a resident population. One thread owns
// one grid cell, so this projection requires no atomics or density floors.
unsigned cells=p.grid_x*p.grid_y*p.grid_z;
if(gid>=cells)return;
unsigned coordinates[3]={gid/(p.grid_y*p.grid_z),(gid/p.grid_z)%p.grid_y,gid%p.grid_z};
unsigned dimensions[3]={p.grid_x,p.grid_y,p.grid_z};
float real=0,imaginary=0;
for(unsigned mode=0;mode<p.num_modes;++mode){
    float weighted=0,weight_sum=0;
    for(unsigned slot=0;slot<p.anchors_per_mode;++slot){
        unsigned offset=mode*p.anchors_per_mode+slot;
        unsigned anchor=mode_anchor_idx[offset];
        float weight=mode_anchor_weight[offset];
        if(anchor==0xffffffffu||anchor>=p.num_particles||weight==0)continue;
        float overlap=1;
        for(unsigned axis=0;axis<3;++axis){
            float distance=float(coordinates[axis])*p.grid_spacing-particle_pos[3*anchor+axis];
            overlap*=mc_periodic_gaussian(distance,float(dimensions[axis])*p.grid_spacing,p.sigma);
        }
        weighted+=weight*overlap;weight_sum+=weight;
    }
    if(weight_sum==0)continue;
    float overlap=weighted/weight_sum;
    real+=mode_psi_real[mode]*overlap;
    imaginary+=mode_psi_imag[mode]*overlap;
}
psi_re_field[gid]=real;
psi_im_field[gid]=imaginary;
