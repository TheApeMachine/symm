@0xf4c35b2ce79536cf;

using Go = import "/go.capnp";
$Go.package("hawkes");
$Go.import("github.com/theapemachine/symm/nomagique/statistic/hawkes");

# Branching forms the branching matrix of the process: the excitation matrix
# divided by the decay rate. Entry (k, j) is the expected number of
# first-generation component-k arrivals triggered by one component-j arrival,
# which is the integral of that kernel over all future time.
interface Branching {
  write @0 (
    excitation :List(Float64),
    decay      :Float64,
    dimension  :Int32,
  ) -> stream;

  done @1 () -> (
    branching :List(Float64),
  );
}
