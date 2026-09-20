using Go = import "/go.capnp";
@0xe4399eef11082d2c;
$Go.package("store");
$Go.import("nomagique/store");

interface Key {
  extract @0 (payload :AnyPointer) -> (value :Float64, found :Bool);
}
