@0xd73ad60f8917911b;

using Go = import "/go.capnp";
$Go.package("distribution");
$Go.import("github.com/theapemachine/symm/nomagique/distribution");

interface Uniform {
  write @0 (
    min :List(Float64),
    max :List(Float64),
    dim :Int32,
    x :List(Float64),
    p :List(Float64),
  ) -> stream;

  done @1 () -> (
    prob :Float64,
    logProb :Float64,
    entropy :Float64,
    cdf :List(Float64),
    mean :List(Float64),
    quantile :List(Float64),
    rand :List(Float64),
  );
}
