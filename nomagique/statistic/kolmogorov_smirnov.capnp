@0xc94a0c08a7a5c48f;

using Go = import "/go.capnp";
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

interface KolmogorovSmirnov {
  write @0 (
    x :List(Float64),
    y :List(Float64),
  ) -> stream;

  done @1 () -> (
    statistic :Float64,
  );
}
