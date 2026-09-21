@0xd0d4d1eefe7c9bcc;

using Go = import "/go.capnp";
$Go.package("calculus");
$Go.import("github.com/theapemachine/symm/nomagique/calculus");

interface Sqrt {
  write @0 (value :Float64) -> stream;
  done @1 () -> (out :Float64);
}
