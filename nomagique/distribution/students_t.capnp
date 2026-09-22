@0xfc192c4d5d3c9ef5;

using Go = import "/go.capnp";
$Go.package("distribution");
$Go.import("github.com/theapemachine/symm/nomagique/distribution");

interface StudentsT {
  write @0 (
    mu :List(Float64),
    sigma :List(Float64),
    dim :Int32,
    nu :Float64,
    y :List(Float64),
  ) -> stream;

  done @1 () -> (
    prob :Float64,
    logProb :Float64,
    mean :List(Float64),
    cov :List(Float64),
    rand :List(Float64),
  );
}
