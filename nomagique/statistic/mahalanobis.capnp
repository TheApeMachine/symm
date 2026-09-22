@0x8f3e484cbd075623;

using Go = import "/go.capnp";
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

interface Mahalanobis {
  write @0 (
    x :List(Float64),
    y :List(Float64),
    cholData :List(Float64),
    dim :Int32,
  ) -> stream;

  done @1 () -> (
    distance :Float64,
  );
}
