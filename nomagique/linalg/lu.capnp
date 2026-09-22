@0x8c3bc996aeade94c;

using Go = import "/go.capnp";
$Go.package("linalg");
$Go.import("github.com/theapemachine/symm/nomagique/linalg");

using import "matrix.capnp".Matrix;

interface LU {
  write @0 (
    a :Matrix,
  ) -> stream;

  done @1 () -> (
    l :Matrix,
    u :Matrix,
  );
}
