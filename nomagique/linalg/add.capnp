@0xb4bb18f267534c26;

using Go = import "/go.capnp";
$Go.package("linalg");
$Go.import("github.com/theapemachine/symm/nomagique/linalg");

using import "matrix.capnp".Matrix;

interface Add {
  write @0 (
    a :Matrix,
    b :Matrix,
  ) -> stream;

  done @1 () -> (
    c :Matrix,
  );
}
