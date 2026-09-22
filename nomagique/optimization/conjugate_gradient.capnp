@0xc815990bcd2a00fd;

using Go = import "/go.capnp";
$Go.package("optimization");
$Go.import("github.com/theapemachine/symm/nomagique/optimization");

interface ConjugateGradient {
  write @0 (
    initX :List(Float64),
    matrixA :List(Float64),
    vectorB :List(Float64),
    dim :Int32,
  ) -> stream;

  done @1 () -> (
    x :List(Float64),
    fVal :Float64,
    iterations :Int32,
  );
}
