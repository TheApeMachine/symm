@0xa40041abb517b50e;

using Go = import "/go.capnp";
$Go.package("linalg");
$Go.import("github.com/theapemachine/symm/nomagique/linalg");

using import "matrix.capnp".Matrix;
using import "matrix.capnp".Vector;

interface SolveVec {
  write @0 (
    a :Matrix,
    b :Vector,
  ) -> stream;

  done @1 () -> (
    x :Vector,
  );
}
