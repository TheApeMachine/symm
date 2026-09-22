@0xa3b44fd4bf48b709;

using Go = import "/go.capnp";
$Go.package("probability");
$Go.import("github.com/theapemachine/symm/nomagique/probability");

interface Exponential {
  write @0 (
    rate :Float64,
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
    entropy :Float64,
    rand :Float64,
  );
}
