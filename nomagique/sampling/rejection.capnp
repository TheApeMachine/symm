@0xb2d043368b5d4743;

using Go = import "/go.capnp";
$Go.package("sampling");
$Go.import("github.com/theapemachine/symm/nomagique/sampling");

interface Rejection {
  write @0 (
    count :Int32,
    c :Float64,
    targetScale :Float64,
    proposalScale :Float64,
  ) -> stream;

  done @1 () -> (
    samples :List(Float64),
    proposed :Int32,
  );
}
