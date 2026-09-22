@0xf38e3a0d2eb487f6;

using Go = import "/go.capnp";
$Go.package("linalg");
$Go.import("github.com/theapemachine/symm/nomagique/linalg");

using import "matrix.capnp".Matrix;

interface Det {
  write @0 (
    a :Matrix,
  ) -> stream;

  done @1 () -> (
    det :Float64,
  );
}
