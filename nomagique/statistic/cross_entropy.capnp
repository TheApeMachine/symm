@0xfae815869f833fd7;

using Go = import "/go.capnp";
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

interface CrossEntropy {
  write @0 (
    p :List(Float64),
    q :List(Float64),
  ) -> stream;

  done @1 () -> (
    crossEntropy :Float64,
  );
}
