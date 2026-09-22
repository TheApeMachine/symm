@0xe8a32e9c0a1b2928;

using Go = import "/go.capnp";
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

interface StdDev {
  write @0 (
    value :Float64,
  ) -> stream;

  done @1 () -> (
    stdDev :Float64,
  );
}
