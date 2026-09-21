@0xb0f4a0dc04de6640;

using Go = import "/go.capnp";
$Go.package("calculus");
$Go.import("github.com/theapemachine/symm/nomagique/calculus");

interface Square {
  write @0 (value :Float64) -> stream;
  done @1 () -> (out :Float64);
}
