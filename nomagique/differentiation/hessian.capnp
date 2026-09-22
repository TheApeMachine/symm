@0xe3b3757042003e85;

using Go = import "/go.capnp";
$Go.package("differentiation");
$Go.import("github.com/theapemachine/symm/nomagique/differentiation");

interface Hessian {
  write @0 (
    coeffs :List(Float64),
    x :List(Float64),
    dim :Int32,
  ) -> stream;

  done @1 () -> (
    hessian :List(Float64),
  );
}
