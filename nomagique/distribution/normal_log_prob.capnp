@0xffeb8d49fa4084cd;

using Go = import "/go.capnp";
$Go.package("distribution");
$Go.import("github.com/theapemachine/symm/nomagique/distribution");

interface NormalLogProb {
  write @0 (
    x :List(Float64),
    mu :List(Float64),
    cholData :List(Float64),
    dim :Int32,
  ) -> stream;

  done @1 () -> (
    logProb :Float64,
  );
}
