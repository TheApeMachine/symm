@0x8078f00762cfa83a;

using Go = import "/go.capnp";
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

interface ResidualDivergence {
  write @0 (a :Float64) -> stream;
  done @1 ();
}
