@0xe1c5cc4b5925a7a7;

using Go = import "/go.capnp";
$Go.package("probability");
$Go.import("github.com/theapemachine/symm/nomagique/probability");

interface F {
  write @0 (
    d1 :Float64,
    d2 :Float64,
    x :Float64,
    p :Float64,
  ) -> stream;

  done @1 () -> (
    prob :Float64,
    logProb :Float64,
    cdf :Float64,
    quantile :Float64,
    survival :Float64,
    mean :Float64,
    variance :Float64,
    stdDev :Float64,
    rand :Float64,
  );
}
