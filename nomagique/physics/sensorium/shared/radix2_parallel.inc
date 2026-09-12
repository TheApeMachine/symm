/* Intentional multi-include: instantiate with MR_IMPL, MR_MEM, MR_BARRIER,
   MR_RADIX, MR_TRANSFORM. Every lane must call; barriers are unconditional. */
MR_IMPL void MR_RADIX(MR_MEM MRComplex* x,unsigned n,int sign,unsigned lane,unsigned lanes) {
    // Bit-reversal permutation consists of disjoint swaps.
    for (unsigned i=lane;i<n;i+=lanes) {
        unsigned j=mr_bit_reverse(i,n);
        if (i<j) { MRComplex t=x[i];x[i]=x[j];x[j]=t; }
    }
    MR_BARRIER();
    for (unsigned len=2;len<=n;len<<=1) {
        unsigned half_len=len>>1;
        for (unsigned butterfly=lane;butterfly<n/2;butterfly+=lanes) {
            unsigned j=butterfly%half_len;
            unsigned a=(butterfly/half_len)*len+j;
            // Compute bounded-angle twiddles directly; no drifting recurrence.
            MRComplex w=mr_cis(float(sign)*6.28318530717958647692f*(float(j)/float(len)));
            MRComplex u=x[a],v=mr_mul(w,x[a+half_len]);
            x[a]=mr_add(u,v);x[a+half_len]=mr_sub(u,v);
        }
        MR_BARRIER();
        if (len==n) break; // avoid wrap if used with a maximal power of two
    }
    if (sign>0) {
        for(unsigned i=lane;i<n;i+=lanes) x[i]=mr_scale(x[i],1.0f/float(n));
        MR_BARRIER();
    }
}
MR_IMPL void MR_TRANSFORM(MR_MEM MRComplex* work,unsigned n,int sign,unsigned lane,unsigned lanes) {
    if (mr_power2(n)) { MR_RADIX(work,n,sign,lane,lanes);return; }
    // Bluestein: n-point DFT = chirp * linear convolution. Padding belongs to
    // the convolution workspace ONLY; the modeled periodic lattice remains n.
    unsigned l=mr_next_power2(2*n-1);
    MR_MEM MRComplex* a=work+n;
    MR_MEM MRComplex* b=a+l;
    for (unsigned j=lane;j<l;j+=lanes) {
        a[j]=(j<n)?mr_mul(work[j],mr_chirp(j,n,sign)):MRComplex{0,0};
        unsigned q=(j<n)?j:((j>l-n)?l-j:n);
        b[j]=(q<n)?mr_chirp(q,n,-sign):MRComplex{0,0};
    }
    MR_BARRIER();
    MR_RADIX(a,l,-1,lane,lanes);
    MR_RADIX(b,l,-1,lane,lanes);
    for(unsigned j=lane;j<l;j+=lanes) a[j]=mr_mul(a[j],b[j]);
    MR_BARRIER();
    MR_RADIX(a,l,+1,lane,lanes);
    for(unsigned j=lane;j<n;j+=lanes) {
        work[j]=mr_mul(a[j],mr_chirp(j,n,sign));
        if(sign>0) work[j]=mr_scale(work[j],1.0f/float(n));
    }
    MR_BARRIER();
}
