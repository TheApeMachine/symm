@0xd62a1b08e6268d2b;

using Go = import "/go.capnp";
$Go.package("distribution");
$Go.import("github.com/theapemachine/symm/nomagique/distribution");

interface NormalBhattacharyya {
  write @0 (
    muL :List(Float64),
    sigmaL :List(Float64),
    muR :List(Float64),
    sigmaR :List(Float64),
    dim :Int32,
  ) -> stream;

  done @1 () -> (
    bhattacharyya :Float64,
  );
}
