@0x98bc45995059e059;

using Go = import "/go.capnp";
$Go.package("arithmetic");
$Go.import("github.com/theapemachine/symm/nomagique/arithmetic");

interface Divide {
  write @0 (a :Float64, b :Float64) -> stream;
  done @1 () -> (out :Float64);
}

