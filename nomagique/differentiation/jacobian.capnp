@0xd4efb29fa0ce1833;

using Go = import "/go.capnp";
$Go.package("differentiation");
$Go.import("github.com/theapemachine/symm/nomagique/differentiation");

interface Jacobian {
  write @0 (
    coeffs :List(Float64),
    x :List(Float64),
    inDim :Int32,
    outDim :Int32,
  ) -> stream;

  done @1 () -> (
    jacobian :List(Float64),
  );
}
