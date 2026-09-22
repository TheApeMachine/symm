@0x90c0b7f704493065;

using Go = import "/go.capnp";
$Go.package("linalg");
$Go.import("github.com/theapemachine/symm/nomagique/linalg");

using import "matrix.capnp".Matrix;

interface Solve {
  write @0 (
    a :Matrix,
    b :Matrix,
  ) -> stream;

  done @1 () -> (
    x :Matrix,
  );
}
