using Go = import "/go.capnp";
@0xe4399eef11082d2c;
$Go.package("store");
$Go.import("nomagique/store");

interface Key {
  write @0 (in :Data, path :Text) -> stream;
  done @1 () -> (value :Float64, found :Bool);
}
