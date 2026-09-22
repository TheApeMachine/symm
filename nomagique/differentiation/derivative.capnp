@0x809ff7cf4b87e941;

using Go = import "/go.capnp";
$Go.package("differentiation");
$Go.import("github.com/theapemachine/symm/nomagique/differentiation");

interface Derivative {
  write @0 (
    coeffs :List(Float64),
    x :Float64,
  ) -> stream;

  done @1 () -> (
    deriv :Float64,
    deriv2 :Float64,
  );
}
