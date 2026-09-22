@0xafbde64230d80df9;

using Go = import "/go.capnp";
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

interface Kurtosis {
  write @0 (
    value :Float64,
  ) -> stream;

  done @1 () -> (
    kurtosis :Float64,
  );
}
