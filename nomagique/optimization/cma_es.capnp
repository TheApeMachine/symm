@0xd7e4c329c4e8caae;

using Go = import "/go.capnp";
$Go.package("optimization");
$Go.import("github.com/theapemachine/symm/nomagique/optimization");

interface CmaEs {
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
