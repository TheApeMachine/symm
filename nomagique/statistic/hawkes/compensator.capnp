@0xb993c4f59dc54bfd;

using Go = import "/go.capnp";
$Go.package("hawkes");
$Go.import("github.com/theapemachine/symm/nomagique/statistic/hawkes");

# Compensator evaluates the integrated conditional intensity of each
# component over the observation window: the term the log-likelihood
# subtracts, and the quantity an observed count is compared against to say
# whether the process produced more or fewer arrivals than it accounted for.
interface Compensator {
  write @0 (
    baseline   :List(Float64),
    excitation :List(Float64),
    support    :List(Float64),
    span       :Float64,
    decay      :Float64,
    dimension  :Int32,
  ) -> stream;

  done @1 () -> (
    compensator :List(Float64),
  );
}
