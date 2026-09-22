@0xeeaf8e41f664268f;

using Go = import "/go.capnp";
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

interface Mode {
  write @0 (
    value :Float64,
  ) -> stream;

  done @1 () -> (
    mode :Float64,
    frequency :Int64,
  );
}
