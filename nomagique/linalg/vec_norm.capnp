@0xcec9e28ada700252;

using Go = import "/go.capnp";
$Go.package("linalg");
$Go.import("github.com/theapemachine/symm/nomagique/linalg");

using import "matrix.capnp".Vector;

interface VecNorm {
  write @0 (
    u :Vector,
    ord :Float64,
  ) -> stream;

  done @1 () -> (
    norm :Float64,
  );
}
