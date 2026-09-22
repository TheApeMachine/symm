@0xcf6e10e70426bd2e;

using Go = import "/go.capnp";
$Go.package("distribution");
$Go.import("github.com/theapemachine/symm/nomagique/distribution");

interface NormalCrossEntropy {
  write @0 (
    muL :List(Float64),
    sigmaL :List(Float64),
    muR :List(Float64),
    sigmaR :List(Float64),
    dim :Int32,
  ) -> stream;

  done @1 () -> (
    crossEntropy :Float64,
  );
}
