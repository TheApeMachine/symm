@0xa8ae229ef9912c3d;

using Go = import "/go.capnp";
$Go.package("differentiation");
$Go.import("github.com/theapemachine/symm/nomagique/differentiation");

interface Gradient {
  write @0 (
    coeffs :List(Float64),
    x :List(Float64),
  ) -> stream;

  done @1 () -> (
    gradient :List(Float64),
  );
}
