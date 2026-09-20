@0xa2ba158ff4ed337f;

using Go = import "/go.capnp";
$Go.package("probability");
$Go.import("github.com/theapemachine/symm/nomagique/probability");

interface Entropy {
  write @0 (a :Float64) -> stream;
  done @1 ();
}
