using Go = import "/go.capnp";
@0xf935a1768bf2027d;
$Go.package("data");
$Go.import("nomagique/data");

struct WireExtract {
  payload @0 :AnyPointer;
}

interface Extract {
  write @0 (extract :WireExtract) -> stream;
  done @1 ();
}
