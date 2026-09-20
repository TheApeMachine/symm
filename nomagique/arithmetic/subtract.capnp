@0xb8083d9659870b37;

using Go = import "/go.capnp";
$Go.package("arithmetic");
$Go.import("github.com/theapemachine/symm/nomagique/arithmetic");

interface Subtract {
  write @0 (a :Float64, b :Float64) -> stream;
  done @1 ();
}
