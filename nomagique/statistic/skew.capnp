@0xe724104de05c59d6;

using Go = import "/go.capnp";
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

interface Skew {
  write @0 (
    value :Float64,
  ) -> stream;

  done @1 () -> (
    skew :Float64,
  );
}
