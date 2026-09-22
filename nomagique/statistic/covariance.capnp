@0x9c6c37ad44fcabd6;

using Go = import "/go.capnp";
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

interface Covariance {
  write @0 (
    x :Float64,
    y :Float64,
  ) -> stream;

  done @1 () -> (
    covariance :Float64,
  );
}
