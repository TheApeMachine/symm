@0xa1c1ad21ecfcd65a;

using Go = import "/go.capnp";
$Go.package("linalg");
$Go.import("github.com/theapemachine/symm/nomagique/linalg");

using import "matrix.capnp".Matrix;

interface Cholesky {
  write @0 (
    a :Matrix,
  ) -> stream;

  done @1 () -> (
    l :Matrix,
  );
}
