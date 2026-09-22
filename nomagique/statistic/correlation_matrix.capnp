@0x9eef6c8135f3f4f3;

using Go = import "/go.capnp";
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

interface CorrelationMatrix {
  write @0 (
    rows :Int32,
    cols :Int32,
    data :List(Float64),
  ) -> stream;

  done @1 () -> (
    corr :List(Float64),
    dim :Int32,
  );
}
