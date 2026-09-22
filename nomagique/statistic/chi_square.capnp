@0xc57137e04055c664;

using Go = import "/go.capnp";
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

interface ChiSquare {
  write @0 (
    observed :List(Float64),
    expected :List(Float64),
  ) -> stream;

  done @1 () -> (
    chiSquare :Float64,
  );
}
