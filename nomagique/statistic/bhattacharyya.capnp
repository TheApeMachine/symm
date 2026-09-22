@0x9cb6c8078e2623ee;

using Go = import "/go.capnp";
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

interface Bhattacharyya {
  write @0 (
    p :List(Float64),
    q :List(Float64),
  ) -> stream;

  done @1 () -> (
    bhattacharyya :Float64,
  );
}
