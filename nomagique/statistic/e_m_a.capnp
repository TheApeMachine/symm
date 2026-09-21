@0x9195a8887f863c33;

using Go = import "/go.capnp";
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

interface EMA {
  write @0 (value :Float64) -> stream;
  done @1 () -> (out :Float64);
}
