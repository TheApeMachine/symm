using Go = import "/go.capnp";
@0x91d758a99e924524;
$Go.package("websocket");
$Go.import("github.com/theapemachine/symm/nomagique/network/websocket");

using import "../../runtime/status.capnp".Status;

using import "../../runtime/status.capnp".Source;

interface WebSocketClient extends(Source) {
  write @0 (endpoint :Text, write :Data) -> stream;
  done @1 () -> (status: Status, read :Data, receivedAt :Text, endpoint :Text);
}
