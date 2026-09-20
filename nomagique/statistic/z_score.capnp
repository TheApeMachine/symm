@0x827e390f13d8495e;

using Go = import "/go.capnp";
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

interface ZScore {
  write @0 (a :Float64) -> stream;
  done @1 ();
}
