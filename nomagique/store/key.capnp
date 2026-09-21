using Go = import "/go.capnp";
@0xe4399eef11082d2c;
$Go.package("store");
$Go.import("github.com/theapemachine/symm/nomagique/store");

interface Key {
  write @0 (data :Data, path :Text) -> stream;
  done @1 () -> (value :Float64, found :Bool);
}
