@0x8fb038c740a15331;

using Go = import "/go.capnp";
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

interface CovarianceMatrix {
  write @0 (
    rows :Int32,
    cols :Int32,
    data :List(Float64),
  ) -> stream;

  done @1 () -> (
    cov :List(Float64),
    dim :Int32,
  );
}
