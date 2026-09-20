@0xe04b76c011e3e484;

using Go = import "/go.capnp";
$Go.package("calculus");
$Go.import("github.com/theapemachine/symm/nomagique/calculus");

interface Erfc {
  write @0 (in :Float64) -> stream;
  done @1 () -> (out :Float64);
}
