@0xd9dbc3dc8ffa4aa0;

using Go = import "/go.capnp";
$Go.package("probability");
$Go.import("github.com/theapemachine/symm/nomagique/probability");

interface NoncentralT {
  write @0 (
    nu :Float64,
    mu :Float64,
    x :Float64,
    p :Float64,
  ) -> stream;

  done @1 () -> (
    prob :Float64,
    logProb :Float64,
    cdf :Float64,
    quantile :Float64,
    mean :Float64,
    variance :Float64,
  );
}
