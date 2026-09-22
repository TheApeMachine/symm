@0x89ab0b81f5a984cd;

using Go = import "/go.capnp";
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

interface LinearRegression {
  write @0 (
    x :Float64,
    y :Float64,
  ) -> stream;

  done @1 () -> (
    alpha :Float64,
    beta :Float64,
  );
}
