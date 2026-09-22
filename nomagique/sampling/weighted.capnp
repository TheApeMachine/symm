@0xf102a648d59d5102;

using Go = import "/go.capnp";
$Go.package("sampling");
$Go.import("github.com/theapemachine/symm/nomagique/sampling");

interface Weighted {
  write @0 (
    weights :List(Float64),
    count :Int32,
  ) -> stream;

  done @1 () -> (
    indices :List(Int64),
  );
}
