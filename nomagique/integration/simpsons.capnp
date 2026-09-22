@0xb0782766e4b98a9d;

using Go = import "/go.capnp";
$Go.package("integration");
$Go.import("github.com/theapemachine/symm/nomagique/integration");

interface Simpsons {
  write @0 (
    x :List(Float64),
    y :List(Float64),
  ) -> stream;

  done @1 () -> (
    integral :Float64,
  );
}
