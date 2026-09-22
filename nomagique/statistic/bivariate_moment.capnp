@0xf361e44f1a52bc9e;

using Go = import "/go.capnp";
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

interface BivariateMoment {
  write @0 (
    x :List(Float64),
    y :List(Float64),
    orderR :Float64,
    orderS :Float64,
  ) -> stream;

  done @1 () -> (
    moment :Float64,
  );
}
