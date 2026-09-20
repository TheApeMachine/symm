@0xe728c04e1184dc60;

using Go = import "/go.capnp";
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

interface Variance {
  write @0 (a :Float64) -> stream;
  done @1 ();
}
