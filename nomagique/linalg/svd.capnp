@0xfba889891d18a5d2;

using Go = import "/go.capnp";
$Go.package("linalg");
$Go.import("github.com/theapemachine/symm/nomagique/linalg");

using import "matrix.capnp".Matrix;
using import "matrix.capnp".Vector;

interface SVD {
  write @0 (
    a :Matrix,
  ) -> stream;

  done @1 () -> (
    u :Matrix,
    s :Vector,
    v :Matrix,
  );
}
