@0xe1b88c3a2b6ff2b7;

using Go = import "/go.capnp";
$Go.package("integration");
$Go.import("github.com/theapemachine/symm/nomagique/integration");

interface Romberg {
  write @0 (
    y :List(Float64),
    dx :Float64,
  ) -> stream;

  done @1 () -> (
    integral :Float64,
  );
}
