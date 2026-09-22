@0xef50cc0084c7a93a;

using Go = import "/go.capnp";
$Go.package("probability");
$Go.import("github.com/theapemachine/symm/nomagique/probability");

interface StudentsT {
  write @0 (
    mu :Float64,
    sigma :Float64,
    nu :Float64,
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
