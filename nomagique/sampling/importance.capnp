@0xd60dda6931d7629f;

using Go = import "/go.capnp";
$Go.package("sampling");
$Go.import("github.com/theapemachine/symm/nomagique/sampling");

interface Importance {
  write @0 (
    count :Int32,
    targetMu :Float64,
    targetSigma :Float64,
    propMu :Float64,
    propSigma :Float64,
  ) -> stream;

  done @1 () -> (
    samples :List(Float64),
    weights :List(Float64),
  );
}
