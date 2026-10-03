#ifndef MANIFOLD_REMAP_ACCELERATION_H
#define MANIFOLD_REMAP_ACCELERATION_H
#include <vector>
#include <cmath>
#include <limits>

// Anderson multisecant acceleration of the log-domain Sinkhorn map. Eight
// stored secants bound solver scratch independently of grid size. This is a
// numerical memory budget, not a physical coupling or convergence tolerance.
// The caller accepts a proposal only after measuring its marginal residual.
struct MCRemapAcceleration {
    static constexpr unsigned history_limit=8;
    std::vector<std::vector<double>> outputs,deltas;
    void clear(){outputs.clear();deltas.clear();}
    std::vector<double> propose(const std::vector<double>& output,const std::vector<double>& delta){
        std::vector<double> candidate=output;
        std::vector<std::vector<double>> basis,changes;
        for(unsigned entry=0;entry<deltas.size();++entry){
            std::vector<double> column(delta.size()),change(delta.size());
            double original=0;
            for(unsigned i=0;i<delta.size();++i){
                column[i]=delta[i]-deltas[entry][i];
                change[i]=output[i]-outputs[entry][i];
                original+=column[i]*column[i];
            }
            // Reorthogonalize to keep nearly dependent transport modes from
            // amplifying roundoff. Apply the identical operations to delta G.
            for(unsigned pass=0;pass<2;++pass)for(unsigned j=0;j<basis.size();++j){
                double projection=0;for(unsigned i=0;i<delta.size();++i)projection+=column[i]*basis[j][i];
                for(unsigned i=0;i<delta.size();++i){column[i]-=projection*basis[j][i];change[i]-=projection*changes[j][i];}
            }
            double norm=0;for(double value:column)norm+=value*value;
            double epsilon=std::numeric_limits<double>::epsilon();
            if(norm<=epsilon*epsilon*original||norm==0)continue;
            norm=std::sqrt(norm);
            double projection=0;
            for(unsigned i=0;i<delta.size();++i){column[i]/=norm;change[i]/=norm;projection+=column[i]*delta[i];}
            for(unsigned i=0;i<delta.size();++i)candidate[i]-=projection*change[i];
            basis.push_back(std::move(column));changes.push_back(std::move(change));
        }
        outputs.push_back(output);deltas.push_back(delta);
        if(outputs.size()>history_limit){outputs.erase(outputs.begin());deltas.erase(deltas.begin());}
        return candidate;
    }
};
#endif
