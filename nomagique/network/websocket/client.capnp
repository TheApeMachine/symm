using Go = import "/go.capnp";
@0x91d758a99e924524;
$Go.package("websocket");
$Go.import("github.com/theapemachine/symm/nomagique/network/websocket");

using import "../../runtime/status.capnp".Status;

using import "../../runtime/status.capnp".Source;

using import "../../store/radix.capnp".Retained;

# Each onConnect frame is sent in order once on every physical connection before ordinary writes.
# write gathers outbound frames; the JSON graph owns their protocol content.
# connectedWrite accompanies a replacement onConnect handshake: send its delta only
# on an established connection; a new connection sends the complete handshake.
# awaitHandshake keeps discovery from opening an unsubscribed connection.
interface WebSocketClient extends(Source, Retained) {
  write @0 (endpoint :Text, write :List(Data), onConnect :List(Data), awaitHandshake :Bool, connectedWrite :List(Data)) -> stream;
  done @1 () -> Received;
}

# connection is the generation of the physical connection currently open,
# reported on every evaluation whether or not a frame arrived, so anything
# kept per connection can be looked up between frames. frame.generation is the
# connection the frame was read on, which may be an earlier one.
struct Received {
  status @0 :Status;
  connection @6 :UInt64;
  union {
    idle @1 :Void;
    frame :group {
      read @2 :Data;
      receivedAt @3 :Text;
      endpoint @4 :Text;
      generation @5 :UInt64;
      provenance @7 :Data; # Source session and sequence, stable across reconnects.
    }
  }
}
