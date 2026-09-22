@0xe582d20f5d4e7f16;

using Go = import "/go.capnp";
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

interface WassersteinDistance {
  write @0 (
    p :List(Float64),
    q :List(Float64),
  ) -> stream;

  done @1 () -> (
    wassersteinDistance :Float64,
  );
}
