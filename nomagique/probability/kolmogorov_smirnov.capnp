@0x96e97680c8c68e68;

using Go = import "/go.capnp";
$Go.package("probability");
$Go.import("github.com/theapemachine/symm/nomagique/probability");

interface KolmogorovSmirnov {
  write @0 (value :Float64) -> stream;
  done @1 () -> (out :Float64);
}
