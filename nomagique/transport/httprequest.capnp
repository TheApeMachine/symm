using Go = import "/go.capnp";
@0xa8ffe8fde8c3decd;
$Go.package("transport");
$Go.import("nomagique/transport");

struct WireHTTPRequest { url @0 :Text; method @1 :Text; body @2 :Data; headers @3 :AnyPointer; }

interface HTTPRequest {
  write @0 (payload :WireHTTPRequest) -> stream;
  done @1 ();
}
