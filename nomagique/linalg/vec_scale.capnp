@0xb8053461c90d0f95;

using Go = import "/go.capnp";
$Go.package("linalg");
$Go.import("github.com/theapemachine/symm/nomagique/linalg");

using import "matrix.capnp".Vector;

interface VecScale {
  write @0 (
    alpha :Float64,
    u :Vector,
  ) -> stream;

  done @1 () -> (
    w :Vector,
  );
}
