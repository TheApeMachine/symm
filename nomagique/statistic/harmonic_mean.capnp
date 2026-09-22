@0xeb35d4635d154cb0;

using Go = import "/go.capnp";
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

interface HarmonicMean {
  write @0 (
    value :Float64,
  ) -> stream;

  done @1 () -> (
    harmonicMean :Float64,
  );
}
