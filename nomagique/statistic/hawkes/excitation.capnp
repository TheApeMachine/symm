@0xbd6e1984726fabc3;

using Go = import "/go.capnp";
$Go.package("hawkes");
$Go.import("github.com/theapemachine/symm/nomagique/statistic/hawkes");

# Excitation evaluates the exponential kernel's support at the horizon:
# for each component j, the sum of exp(-decay * (horizon - t)) over that
# component's arrivals at or before the horizon. This is the quantity the
# conditional intensity is linear in, so it is worth computing once.
interface Excitation {
  write @0 (
    times      :List(Float64),
    components :List(Float64),
    horizon    :Float64,
    decay      :Float64,
    dimension  :Int32,
  ) -> stream;

  done @1 () -> (
    support :List(Float64),
  );
}
