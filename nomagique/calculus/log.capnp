@0xc461bdf128bdf63b;

using Go = import "/go.capnp";
$Go.package("calculus");
$Go.import("github.com/theapemachine/symm/nomagique/calculus");

interface Log {
  write @0 (in :Float64) -> stream;
  done @1 () -> (out :Float64);
}
