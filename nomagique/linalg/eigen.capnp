@0xd6ef09e7577dbed5;

using Go = import "/go.capnp";
$Go.package("linalg");
$Go.import("github.com/theapemachine/symm/nomagique/linalg");

using import "matrix.capnp".Matrix;
using import "matrix.capnp".Vector;

interface Eigen {
  write @0 (
    a :Matrix,
  ) -> stream;

  done @1 () -> (
    values :Vector,
    vectors :Matrix,
  );
}
