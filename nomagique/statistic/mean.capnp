@0x8010078df8ab7894;

using Go = import "/go.capnp";
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

interface Mean {
  write @0 (value :Float64) -> stream;
  done @1 () -> (out :Float64);
}
