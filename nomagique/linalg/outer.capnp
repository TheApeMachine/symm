@0x921f30fa84b70404;

using Go = import "/go.capnp";
$Go.package("linalg");
$Go.import("github.com/theapemachine/symm/nomagique/linalg");

using import "matrix.capnp".Matrix;
using import "matrix.capnp".Vector;

interface Outer {
  write @0 (
    alpha :Float64,
    u :Vector,
    v :Vector,
  ) -> stream;

  done @1 () -> (
    c :Matrix,
  );
}
