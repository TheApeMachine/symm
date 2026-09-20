@0xd7262fb17bd35f6a;

using Go = import "/go.capnp";
$Go.package("arithmetic");
$Go.import("github.com/theapemachine/symm/nomagique/arithmetic");

interface Multiply {
  write @0 (a :Float64, b :Float64) -> stream;
  done @1 () -> (out :Float64);
}

