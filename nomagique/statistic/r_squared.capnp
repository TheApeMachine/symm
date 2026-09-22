@0x891f5c5c3b6e9548;

using Go = import "/go.capnp";
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

interface RSquared {
  write @0 (
    x :Float64,
    y :Float64,
  ) -> stream;

  done @1 () -> (
    rSquared :Float64,
  );
}
