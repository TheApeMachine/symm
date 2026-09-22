@0x9aa6e5265d526239;

using Go = import "/go.capnp";
$Go.package("distribution");
$Go.import("github.com/theapemachine/symm/nomagique/distribution");

interface Normal {
  write @0 (
    mu :List(Float64),
    sigma :List(Float64),
    dim :Int32,
    x :List(Float64),
    p :List(Float64),
  ) -> stream;

  done @1 () -> (
    prob :Float64,
    logProb :Float64,
    entropy :Float64,
    mean :List(Float64),
    cov :List(Float64),
    quantile :List(Float64),
    rand :List(Float64),
  );
}
