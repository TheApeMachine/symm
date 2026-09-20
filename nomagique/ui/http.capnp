using Go = import "/go.capnp";
@0xdf0315c46f0719ed;
$Go.package("ui");
$Go.import("nomagique/ui");

struct WireHTTPServer {
  payload @0 :AnyPointer;
  addr @1 :Text;
  path @2 :Text;
  templatePath @3 :Text;
  fsRoot @4 :Text;
}

interface HTTPServer {
  write @0 (server :WireHTTPServer) -> stream;
  done @1 ();
}
