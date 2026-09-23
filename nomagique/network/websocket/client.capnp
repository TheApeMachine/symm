using Go = import "/go.capnp";
@0x91d758a99e924524;
$Go.package("websocket");
$Go.import("github.com/theapemachine/symm/nomagique/network/websocket");

using import "../../runtime/status.capnp".Status;

using import "../../runtime/status.capnp".Source;

using import "../../store/radix.capnp".Retained;

# onConnect is sent once on every physical connection before ordinary writes.
# write gathers outbound frames; the JSON graph owns their protocol content.
interface WebSocketClient extends(Source, Retained) {
  write @0 (endpoint :Text, write :List(Data), onConnect :Data) -> stream;
  done @1 () -> Received;
}

struct Received {
  status @0 :Status;
  union {
    idle @1 :Void;
    frame :group {
      read @2 :Data;
      receivedAt @3 :Text;
      endpoint @4 :Text;
      generation @5 :UInt64;
    }
  }
}
