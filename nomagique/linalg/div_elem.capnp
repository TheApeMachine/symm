@0xc768581d5cafc1dd;

using Go = import "/go.capnp";
$Go.package("linalg");
$Go.import("github.com/theapemachine/symm/nomagique/linalg");

using import "matrix.capnp".Matrix;

interface DivElem {
  write @0 (
    a :Matrix,
    b :Matrix,
  ) -> stream;

  done @1 () -> (
    c :Matrix,
  );
}
