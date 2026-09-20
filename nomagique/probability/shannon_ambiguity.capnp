@0xad9084005e3d26d4;

using Go = import "/go.capnp";
$Go.package("probability");
$Go.import("github.com/theapemachine/symm/nomagique/probability");

interface ShannonAmbiguity {
  write @0 (a :Float64) -> stream;
  done @1 ();
}
