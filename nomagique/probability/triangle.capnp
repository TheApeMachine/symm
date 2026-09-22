@0x8b6b8494f576c4af;

using Go = import "/go.capnp";
$Go.package("probability");
$Go.import("github.com/theapemachine/symm/nomagique/probability");

interface Triangle {
  write @0 (
    a :Float64,
    b :Float64,
    c :Float64,
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
