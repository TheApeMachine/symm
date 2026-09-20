@0xa7a2f184cdaf6d7e;

using Go = import "/go.capnp";
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

interface CausalVariance {
  write @0 (a :Float64) -> stream;
  done @1 ();
}
