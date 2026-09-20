using Go = import "/go.capnp";
@0xc826f6eb94916a2b;
$Go.package("store");
$Go.import("nomagique/store");

interface Constant {
  evaluate @0 (payload :AnyPointer) -> (value :AnyPointer);
}
