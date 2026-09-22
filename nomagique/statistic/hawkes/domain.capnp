@0xbb34b37fe2c2ef6c;

using Go = import "/go.capnp";
$Go.package("hawkes");
$Go.import("github.com/theapemachine/symm/nomagique/statistic/hawkes");

# Domain derives the search region an optimizer may look for parameters in,
# and a starting point inside it, from the observed window alone. Every bound
# is stated in a scale the data itself carries: the observed inter-arrival
# gaps set the decay range, the window's span and arrival count set the
# baseline range, and stability sets the branching ceiling.
#
# Coordinates are unconstrained and ordered as the rest of this package
# orders parameters: the baselines, the excitation matrix row-major, then the
# decay rate. They are the coordinates a search moves in, not the parameters
# themselves; Parameters maps between the two and this node inverts that map
# to place the seed.
#
# When coupled is not positive the region describes a restricted process in
# which no component excites any other: the off-diagonal excitation is
# pinned and a search cannot move it. Fitting in that region and in the
# unrestricted one gives the two likelihoods a nested comparison needs.
interface Domain {
  write @0 (
    times      :List(Float64),
    components :List(Float64),
    origin     :Float64,
    horizon    :Float64,
    dimension  :Int32,
    coupled    :Float64,
  ) -> stream;

  done @1 () -> (
    lower   :List(Float64),
    upper   :List(Float64),
    seed    :List(Float64),
    defined :Bool,
  );
}
