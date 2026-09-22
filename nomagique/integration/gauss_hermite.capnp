@0xfc85624b99f9dd48;

using Go = import "/go.capnp";
$Go.package("integration");
$Go.import("github.com/theapemachine/symm/nomagique/integration");

interface GaussHermite {
  write @0 (
    coeffs :List(Float64),
    points :Int32,
  ) -> stream;

  done @1 () -> (
    integral :Float64,
  );
}
