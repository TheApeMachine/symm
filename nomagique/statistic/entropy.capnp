@0xdc092ba24368ee04;

using Go = import "/go.capnp";
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

interface Entropy {
  write @0 (
    p :List(Float64),
  ) -> stream;

  done @1 () -> (
    entropy :Float64,
  );
}
