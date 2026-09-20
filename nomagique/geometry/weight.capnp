@0xaacd3bc39e5c0d5f;

using Go = import "/go.capnp";
$Go.package("geometry");
$Go.import("github.com/theapemachine/symm/nomagique/geometry");

interface Weight {
  write @0 (a :Float64) -> stream;
  done @1 ();
}
