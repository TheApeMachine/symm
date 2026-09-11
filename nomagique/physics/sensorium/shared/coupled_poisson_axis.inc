const unsigned length=mc_line_length(axis,p);
if(line>=p.n/length)return;
// All threads in a block execute the same branch and all FFT barriers.
MC_MEM MRComplex* local=work+line*workspace;
for(unsigned k=lane;k<length;k+=lanes)local[k]=field[mc_line_index(line,k,axis,p)];
MC_BARRIER();
MC_TRANSFORM(local,length,sign,lane,lanes);
for(unsigned k=lane;k<length;k+=lanes)field[mc_line_index(line,k,axis,p)]=local[k];
