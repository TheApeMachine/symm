using Go = import "/go.capnp";
@0xe9fc9395f4800f4b;
$Go.package("execution");
$Go.import("nomagique/execution");

struct WireDecide {
  eval @0 :AnyPointer;
  minContrast @1 :Float64;
}

interface Decide {
  write @0 (decide :WireDecide) -> stream;
  done @1 ();
}
