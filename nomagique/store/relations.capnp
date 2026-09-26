@0xe179a5d4de579060;
using Go = import "/go.capnp";
$Go.package("store");
$Go.import("github.com/theapemachine/symm/nomagique/store");

struct PricePoint { at @0 :Int64; value @1 :Float64; }

# Relations owns the adaptive spot price paths and current cross-asset relation
# facts. It emits only the selected pair's paths, never the entire cohort.
# Values: correlation, covariance, support, left/right energy, left/right median
# energy rate, left/right return count, shared seconds, overlap density,
# weighted signed/absolute correlation, peer energy, peer count, effective peer
# count, Fisher dispersion. Present marks defined facts without fabricated zeros.
interface Relations {
  write @0 (key :Text, price :Float64, timestamp :Float64,
            epoch :Int64, sequence :Int64) -> stream;
  done @1 () -> (values :List(Float64), present :List(Bool),
    left :List(PricePoint), right :List(PricePoint), peer :Text,
    epoch :Int64, sequence :Int64, peerSequence :Int64);
}
