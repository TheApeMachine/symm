#ifndef MANIFOLD_SCAN_PLAN_H
#define MANIFOLD_SCAN_PLAN_H
#include <stdint.h>
#include <stddef.h>
struct ManifoldScanLevel { uint32_t n,groups; size_t sums,prefix; };
struct ManifoldScanPlan {
    ManifoldScanLevel level[5]{}; unsigned count=0; size_t words=0;
    bool make(uint32_t n) {
        count=0;words=0;
        while(n) {
            if(count==5) return false;
            uint32_t groups=n/256u+(n%256u!=0u);
            level[count++]={n,groups,words,words+groups}; words+=2ull*groups;
            if(groups==1) break;
            n=groups;
        }
        return true;
    }
};
#endif
