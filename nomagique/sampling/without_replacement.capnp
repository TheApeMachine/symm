@0xdd40949f1e1f0fbf;

using Go = import "/go.capnp";
$Go.package("sampling");
$Go.import("github.com/theapemachine/symm/nomagique/sampling");

interface WithoutReplacement {
  write @0 (
    count :Int32,
    n :Int32,
  ) -> stream;

  done @1 () -> (
    indices :List(Int64),
  );
}
