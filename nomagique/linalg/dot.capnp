@0xd09779b52e2da783;

using Go = import "/go.capnp";
$Go.package("linalg");
$Go.import("github.com/theapemachine/symm/nomagique/linalg");

using import "matrix.capnp".Vector;

interface Dot {
  write @0 (
    u :Vector,
    v :Vector,
  ) -> stream;

  done @1 () -> (
    dot :Float64,
  );
}
