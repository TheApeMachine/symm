@0x89cfaafeb12a6b3a;

using Go = import "/go.capnp";
$Go.package("geometry");
$Go.import("github.com/theapemachine/symm/nomagique/geometry");

interface Intersection {
  write @0 (leftStart :Int64, leftEnd :Int64, rightStart :Int64, rightEnd :Int64) -> stream;
  done @1 () -> (out :Bool);
}
