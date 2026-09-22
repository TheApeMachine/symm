@0xf84dcd44ef5eeed7;

using Go = import "/go.capnp";
$Go.package("distribution");
$Go.import("github.com/theapemachine/symm/nomagique/distribution");

interface NormalWasserstein {
  write @0 (
    muL :List(Float64),
    sigmaL :List(Float64),
    muR :List(Float64),
    sigmaR :List(Float64),
    dim :Int32,
  ) -> stream;

  done @1 () -> (
    wasserstein :Float64,
  );
}
