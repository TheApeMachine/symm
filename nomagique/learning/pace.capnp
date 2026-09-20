@0xa7d7faff428fc957;

using Go = import "/go.capnp";
$Go.package("learning");
$Go.import("github.com/theapemachine/symm/nomagique/learning");


interface Pace {
  write @0 (errorMagnitude :Float64) -> stream;
  done @1 ();
}
