@0xeb99ee321cec603d;

using Go = import "/go.capnp";
$Go.package("arithmetic");
$Go.import("github.com/theapemachine/symm/nomagique/arithmetic");

interface Add {
  write @0 (a :Float64, b :Float64) -> stream;
  done @1 ();
}
