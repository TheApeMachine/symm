using Go = import "/go.capnp";
@0x8866b6af078213ef;
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

# Concordance reads how pairs of elements move together.
#
# value holds dimensionless movements, one per element. fromNodes, toNodes and
# pairs name the pairs read now; prior and known are each pair's retained
# state in the same order, five numbers per pair: support, summed alignment,
# mean alignment, its second moment, and summed magnitude agreement. state and
# index are the updated records, to be written back.
#
# A pair where neither element moved says nothing. Where both moved their
# alignment is the product of their directions, so a consistent inverse pair
# is as concordant as a direct one, only oppositely oriented. Where one moved
# and the other did not, alignment is zero: an observed non-response.
#
# strength is |mean alignment| less its standard error, plus relative
# magnitude agreement weighted by that consistency, so an uncertain or
# contradictory relationship can go negative without a chosen cutoff. A pair
# that just broke its established orientation, including by not responding,
# reads as negative. orientation is +1 for direct and -1 for inverse.
# Only pairs with support are emitted.
interface Concordance {
  write @0 (
    value     :List(Float64),
    fromNodes :List(Int64),
    toNodes   :List(Int64),
    pairs     :List(Int64),
    prior     :List(Float64),
    known     :List(Bool)
  ) -> stream;
  done @1 () -> (
    fromNodes   :List(Int64),
    toNodes     :List(Int64),
    strength    :List(Float64),
    orientation :List(Float64),
    index       :List(Int64),
    state       :List(Float64)
  );
}
