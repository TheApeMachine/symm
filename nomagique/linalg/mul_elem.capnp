@0xa436e913ad667d17;

using Go = import "/go.capnp";
$Go.package("linalg");
$Go.import("github.com/theapemachine/symm/nomagique/linalg");

using import "matrix.capnp".Matrix;

interface MulElem {
  write @0 (
    a :Matrix,
    b :Matrix,
  ) -> stream;

  done @1 () -> (
    c :Matrix,
  );
}
