@0xb905f5d6c85d206a;

using Go = import "/go.capnp";
$Go.package("probability");
$Go.import("github.com/theapemachine/symm/nomagique/probability");

interface AlphaStable {
  write @0 (
    alpha :Float64,
    beta :Float64,
    c :Float64,
    mu :Float64,
  ) -> stream;

  done @1 () -> (
    mean :Float64,
    variance :Float64,
    stdDev :Float64,
    median :Float64,
    mode :Float64,
    exKurtosis :Float64,
    skewness :Float64,
    rand :Float64,
  );
}
