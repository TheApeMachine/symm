@0x8153606fe5316474;

using Go = import "/go.capnp";
$Go.package("linalg");
$Go.import("github.com/theapemachine/symm/nomagique/linalg");

using import "matrix.capnp".Matrix;

interface Inverse {
  write @0 (
    a :Matrix,
  ) -> stream;

  done @1 () -> (
    c :Matrix,
  );
}
