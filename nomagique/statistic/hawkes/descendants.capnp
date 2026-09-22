@0xd10bcfe26a9ccba3;

using Go = import "/go.capnp";
$Go.package("hawkes");
$Go.import("github.com/theapemachine/symm/nomagique/statistic/hawkes");

# Descendants returns the expected total progeny of one arrival on each
# component, summed over every generation: the column sums of the inverse of
# (identity - branching), less the original arrival itself. It is defined
# only for a subcritical process.
interface Descendants {
  write @0 (
    branching :List(Float64),
    dimension :Int32,
  ) -> stream;

  done @1 () -> (
    descendants :List(Float64),
    defined     :Bool,
  );
}
