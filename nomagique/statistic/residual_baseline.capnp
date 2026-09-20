@0xb7c017141db8696e;

using Go = import "/go.capnp";
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

interface ResidualBaseline {
  write @0 (in :Float64) -> stream;
  done @1 () -> (out :Float64);
}
