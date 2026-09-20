@0xdf012110ce3ce448;

using Go = import "/go.capnp";
$Go.package("probability");
$Go.import("github.com/theapemachine/symm/nomagique/probability");

interface Distribution {
  write @0 (a :Float64) -> stream;
  done @1 ();
}
