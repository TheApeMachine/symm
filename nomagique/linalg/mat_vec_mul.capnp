@0xc9a502ef9ba07148;

using Go = import "/go.capnp";
$Go.package("linalg");
$Go.import("github.com/theapemachine/symm/nomagique/linalg");

using import "matrix.capnp".Matrix;
using import "matrix.capnp".Vector;

interface MatVecMul {
  write @0 (
    a :Matrix,
    x :Vector,
  ) -> stream;

  done @1 () -> (
    y :Vector,
  );
}
