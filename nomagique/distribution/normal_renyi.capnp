@0x9f1ecb2784e13c59;

using Go = import "/go.capnp";
$Go.package("distribution");
$Go.import("github.com/theapemachine/symm/nomagique/distribution");

interface NormalRenyi {
  write @0 (
    alpha :Float64,
    muL :List(Float64),
    sigmaL :List(Float64),
    muR :List(Float64),
    sigmaR :List(Float64),
    dim :Int32,
  ) -> stream;

  done @1 () -> (
    renyi :Float64,
  );
}
