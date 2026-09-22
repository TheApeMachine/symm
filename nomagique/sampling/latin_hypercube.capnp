@0xd8e4eaba9248d28c;

using Go = import "/go.capnp";
$Go.package("sampling");
$Go.import("github.com/theapemachine/symm/nomagique/sampling");

interface LatinHypercube {
  write @0 (
    count :Int32,
  ) -> stream;

  done @1 () -> (
    samples :List(Float64),
  );
}
