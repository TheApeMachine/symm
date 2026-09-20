@0xddcb867daaaa1e64;

using Go = import "/go.capnp";
$Go.package("probability");
$Go.import("github.com/theapemachine/symm/nomagique/probability");

interface Normalize {
  write @0 (a :Float64) -> stream;
  done @1 ();
}
