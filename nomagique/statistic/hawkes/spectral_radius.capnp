@0xeb3ef3b883f139dd;

using Go = import "/go.capnp";
$Go.package("hawkes");
$Go.import("github.com/theapemachine/symm/nomagique/statistic/hawkes");

# SpectralRadius returns the largest eigenvalue modulus of a square matrix.
# For a branching matrix this is the process's criticality: below one every
# cascade dies out and the process is stationary, at or above one the
# expected descendants of a single arrival diverge.
interface SpectralRadius {
  write @0 (
    matrix    :List(Float64),
    dimension :Int32,
  ) -> stream;

  done @1 () -> (
    radius :Float64,
  );
}
