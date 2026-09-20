using Go = import "/go.capnp";
@0x9ffa62d694ec8b4f;
$Go.package("data");
$Go.import("nomagique/data");

struct WireSelect {
  payload @0 :AnyPointer;
  path @1 :Text;
}

interface Select {
  write @0 (select :WireSelect) -> stream;
  done @1 ();
}
