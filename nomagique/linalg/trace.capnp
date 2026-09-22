@0x8bbcafb01a3d33cf;

using Go = import "/go.capnp";
$Go.package("linalg");
$Go.import("github.com/theapemachine/symm/nomagique/linalg");

using import "matrix.capnp".Matrix;

interface Trace {
  write @0 (
    a :Matrix,
  ) -> stream;

  done @1 () -> (
    trace :Float64,
  );
}
