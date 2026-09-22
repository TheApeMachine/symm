@0xf5c92bd344b6889f;

using Go = import "/go.capnp";
$Go.package("differentiation");
$Go.import("github.com/theapemachine/symm/nomagique/differentiation");

interface FiniteDifference {
  write @0 (
    values :List(Float64),
    step :Float64,
  ) -> stream;

  done @1 () -> (
    forward :List(Float64),
    central :List(Float64),
    backward :List(Float64),
  );
}
