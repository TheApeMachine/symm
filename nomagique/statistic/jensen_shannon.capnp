@0xb616199bff1eaa5a;

using Go = import "/go.capnp";
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

interface JensenShannon {
  write @0 (
    p :List(Float64),
    q :List(Float64),
  ) -> stream;

  done @1 () -> (
    jensenShannon :Float64,
  );
}
