@0xc2bfa1f3e3df28f1;

using Go = import "/go.capnp";
$Go.package("differentiation");
$Go.import("github.com/theapemachine/symm/nomagique/differentiation");

interface Laplacian {
  write @0 (
    coeffs :List(Float64),
    x :List(Float64),
  ) -> stream;

  done @1 () -> (
    laplacian :Float64,
  );
}
