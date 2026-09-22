@0xbc4eed0869b6db47;

using Go = import "/go.capnp";
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

interface Correlation {
  write @0 (
    x :Float64,
    y :Float64,
  ) -> stream;

  done @1 () -> (
    correlation :Float64,
  );
}
