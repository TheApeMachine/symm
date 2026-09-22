@0xd5d2834727c52d05;

using Go = import "/go.capnp";
$Go.package("sampling");
$Go.import("github.com/theapemachine/symm/nomagique/sampling");

interface MetropolisHastings {
  write @0 (
    count :Int32,
    initial :Float64,
    burnIn :Int32,
    rate :Int32,
  ) -> stream;

  done @1 () -> (
    samples :List(Float64),
  );
}
