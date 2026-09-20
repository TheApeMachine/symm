using Go = import "/go.capnp";
@0xd3e5f7a9b2c4d6f8;
$Go.package("learning");
$Go.import("github.com/theapemachine/symm/nomagique/learning");

interface Backdoor {
  write @0 (treatment :Float64, outcome :Float64, adjustment :Float64) -> stream;
  done @1 () -> (effect :Float64, out :Float64);
}
