@0xacebdbabcf2b1278;

using Go = import "/go.capnp";
$Go.package("linalg");
$Go.import("github.com/theapemachine/symm/nomagique/linalg");

using import "matrix.capnp".Matrix;

interface Norm {
  write @0 (
    a :Matrix,
    ord :Float64,
  ) -> stream;

  done @1 () -> (
    norm :Float64,
  );
}
