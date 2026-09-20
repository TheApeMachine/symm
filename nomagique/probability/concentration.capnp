@0xec3983211c7e08c7;

using Go = import "/go.capnp";
$Go.package("probability");
$Go.import("github.com/theapemachine/symm/nomagique/probability");

interface Concentration {
  write @0 (in :Float64) -> stream;
  done @1 () -> (out :Float64);
}
