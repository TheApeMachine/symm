@0xd116d8f464846259;
using Go = import "/go.capnp";
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");
using PricePoint = import "../store/relations.capnp".PricePoint;

# LagSearch evaluates all lags supported by the two retained paths. Spacing is
# their smaller median observation interval; span is min(price counts)-2.
# Native values: contemporaneous correlation, selected signed correlation,
# lag index, lag seconds, absolute correlation gain, lag/span fraction,
# span in indices, nonzero candidate count, prominence, curvature,
# selected overlap support, left/right return counts, search noise scale.
interface LagSearch {
 write @0 (left :List(PricePoint), right :List(PricePoint)) -> stream;
 done @1 () -> (values :List(Float64), present :List(Bool));
}
