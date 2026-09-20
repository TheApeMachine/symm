@0xdceee6be88eb5850;

using Go = import "/go.capnp";
$Go.package("probability");
$Go.import("github.com/theapemachine/symm/nomagique/probability");

interface Ambiguity {
  write @0 (a :Float64) -> stream;
  done @1 ();
}
