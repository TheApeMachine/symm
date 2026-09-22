@0xf8ce79e563228a41;

using Go = import "/go.capnp";
$Go.package("integration");
$Go.import("github.com/theapemachine/symm/nomagique/integration");

interface GaussLegendre {
  write @0 (
    coeffs :List(Float64),
    min :Float64,
    max :Float64,
    points :Int32,
  ) -> stream;

  done @1 () -> (
    integral :Float64,
  );
}
