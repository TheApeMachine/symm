@0xf763f2e02bf7a5d0;

using Go = import "/go.capnp";
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

interface Quantile {
  write @0 (
    values :List(Float64),
    p :Float64,
  ) -> stream;

  done @1 () -> (
    quantile :Float64,
  );
}
