@0x9bf9094ee777baa5;

using Go = import "/go.capnp";
$Go.package("linalg");
$Go.import("github.com/theapemachine/symm/nomagique/linalg");

using import "matrix.capnp".Matrix;

interface Subtract {
  write @0 (
    a :Matrix,
    b :Matrix,
  ) -> stream;

  done @1 () -> (
    c :Matrix,
  );
}
