using Go = import "/go.capnp";
@0xb3d1e5a47c9f2018;
$Go.package("graph");
$Go.import("github.com/theapemachine/symm/nomagique/graph");

# Edges reads a square matrix of relationships as the graph it describes.
#
# Every relationship measured between a set of quantities arrives as a matrix,
# and every question asked about the structure of those relationships is asked
# of an edge list. This is the one turning into the other: a pair whose
# relationship reaches the cut becomes an edge carrying its strength.
#
# The cut is supplied rather than chosen here, because what counts as a strong
# relationship is a property of the relationships measured, not of this
# operation.
interface Edges {
  write @0 (
    matrix    :List(Float64),
    dim       :Int32,
    threshold :Float64,
    signed    :Bool
  ) -> stream;
  done @1 () -> (
    fromNodes :List(Int64),
    toNodes   :List(Int64),
    weights   :List(Float64),
    count     :Int32,
    kept      :Float64
  );
}
