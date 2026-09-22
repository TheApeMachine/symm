@0x9075bc94088dcde7;

using Go = import "/go.capnp";
$Go.package("optimization");
$Go.import("github.com/theapemachine/symm/nomagique/optimization");

interface Rosenbrock {
  write @0 (
    initX :List(Float64),
    a :Float64,
    b :Float64,
  ) -> stream;

  done @1 () -> (
    x :List(Float64),
    fVal :Float64,
    iterations :Int32,
  );
}
