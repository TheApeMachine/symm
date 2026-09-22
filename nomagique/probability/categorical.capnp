@0xa9c3f63fdd4bcad7;

using Go = import "/go.capnp";
$Go.package("probability");
$Go.import("github.com/theapemachine/symm/nomagique/probability");

interface Categorical {
  write @0 (
    weights :List(Float64),
    x :Float64,
  ) -> stream;

  done @1 () -> (
    prob :Float64,
    logProb :Float64,
    cdf :Float64,
    mean :Float64,
    entropy :Float64,
    rand :Float64,
  );
}
