@0x97bc5347b7300089;

using Go = import "/go.capnp";
$Go.package("linalg");
$Go.import("github.com/theapemachine/symm/nomagique/linalg");

using import "matrix.capnp".Matrix;

interface Scale {
  write @0 (
    alpha :Float64,
    a :Matrix,
  ) -> stream;

  done @1 () -> (
    c :Matrix,
  );
}
