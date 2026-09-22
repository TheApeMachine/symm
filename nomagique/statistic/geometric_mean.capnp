@0x8af3c5da2d1f0c7b;

using Go = import "/go.capnp";
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

interface GeometricMean {
  write @0 (
    value :Float64,
  ) -> stream;

  done @1 () -> (
    geometricMean :Float64,
  );
}
