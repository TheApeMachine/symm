@0xc082888e07e3d871;

using Go = import "/go.capnp";
$Go.package("linalg");
$Go.import("github.com/theapemachine/symm/nomagique/linalg");

using import "matrix.capnp".Matrix;

interface Transpose {
  write @0 (
    a :Matrix,
  ) -> stream;

  done @1 () -> (
    c :Matrix,
  );
}
