@0xad3aaf3f9cbf7256;

using Go = import "/go.capnp";
$Go.package("sampling");
$Go.import("github.com/theapemachine/symm/nomagique/sampling");

interface IID {
  write @0 (
    count :Int32,
    min :Float64,
    max :Float64,
  ) -> stream;

  done @1 () -> (
    samples :List(Float64),
  );
}
