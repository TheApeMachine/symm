@0xb0cdc543f2b2ee63;

using Go = import "/go.capnp";
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

interface Threshold {
  write @0 (a :Float64) -> stream;
  done @1 ();
}
