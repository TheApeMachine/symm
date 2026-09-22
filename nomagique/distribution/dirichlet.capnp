@0xb36e4314d479f55e;

using Go = import "/go.capnp";
$Go.package("distribution");
$Go.import("github.com/theapemachine/symm/nomagique/distribution");

interface Dirichlet {
  write @0 (
    alpha :List(Float64),
    x :List(Float64),
  ) -> stream;

  done @1 () -> (
    prob :Float64,
    logProb :Float64,
    mean :List(Float64),
    rand :List(Float64),
  );
}
