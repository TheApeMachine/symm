using Go = import "/go.capnp";
@0xd108491e9f38e217;
$Go.package("websocket");
$Go.import("github.com/theapemachine/symm/nomagique/network/websocket");

using import "../../runtime/status.capnp".Status;

using import "../../ui/binding.capnp".Receiver;

interface WebSocketServer extends(Receiver) {
  write @0 (data :Data, addr :Text, path :Text) -> stream;
  done @1 () -> (status :Status, out :Data);
}
