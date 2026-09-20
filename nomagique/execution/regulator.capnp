using Go = import "/go.capnp";
@0xedcb06057ba81f9d;
$Go.package("execution");
$Go.import("nomagique/execution");

struct WireRegulator {
  payload @0 :AnyPointer;
  symbol @1 :Text;
}

interface Regulator {
  write @0 (regulator :WireRegulator) -> stream;
  done @1 ();
}
