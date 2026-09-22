@0xa3bb8f07b4d4e1f0;

using Go = import "/go.capnp";
$Go.package("distribution");
$Go.import("github.com/theapemachine/symm/nomagique/distribution");

interface NormalHellinger {
  write @0 (
    muL :List(Float64),
    sigmaL :List(Float64),
    muR :List(Float64),
    sigmaR :List(Float64),
    dim :Int32,
  ) -> stream;

  done @1 () -> (
    hellinger :Float64,
  );
}
