@0xd24f8311346260e3;

using Go = import "/go.capnp";
$Go.package("probability");
$Go.import("github.com/theapemachine/symm/nomagique/probability");

interface Binomial {
  write @0 (
    n :Float64,
    p :Float64,
    x :Float64,
  ) -> stream;

  done @1 () -> (
    prob :Float64,
    logProb :Float64,
    cdf :Float64,
    survival :Float64,
    mean :Float64,
    variance :Float64,
    stdDev :Float64,
    rand :Float64,
  );
}
