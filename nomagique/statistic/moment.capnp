@0xf26b0dc53125f85a;

using Go = import "/go.capnp";
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

interface Moment {
  write @0 (
    value :Float64,
    order :Float64,
  ) -> stream;

  done @1 () -> (
    moment :Float64,
  );
}
