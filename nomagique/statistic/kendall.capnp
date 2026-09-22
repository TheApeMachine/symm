@0xc487a85abda44227;

using Go = import "/go.capnp";
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

interface Kendall {
  write @0 (
    x :List(Float64),
    y :List(Float64),
  ) -> stream;

  done @1 () -> (
    kendall :Float64,
  );
}
