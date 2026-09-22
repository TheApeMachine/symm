@0xd3cde1acd39b2d80;

using Go = import "/go.capnp";
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

interface StdErr {
  write @0 (
    value :Float64,
  ) -> stream;

  done @1 () -> (
    stdErr :Float64,
  );
}
