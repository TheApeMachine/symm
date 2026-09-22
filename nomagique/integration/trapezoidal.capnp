@0x8ce31b9df83915fb;

using Go = import "/go.capnp";
$Go.package("integration");
$Go.import("github.com/theapemachine/symm/nomagique/integration");

interface Trapezoidal {
  write @0 (
    x :List(Float64),
    y :List(Float64),
  ) -> stream;

  done @1 () -> (
    integral :Float64,
  );
}
