@0xbc8519465e8d245c;

using Go = import "/go.capnp";
$Go.package("linalg");
$Go.import("github.com/theapemachine/symm/nomagique/linalg");

using import "matrix.capnp".Vector;

interface VecAdd {
  write @0 (
    u :Vector,
    v :Vector,
  ) -> stream;

  done @1 () -> (
    w :Vector,
  );
}
