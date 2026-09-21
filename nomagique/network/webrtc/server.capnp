using Go = import "/go.capnp";
@0xc107316fcb5b03f0;
$Go.package("webrtc");
$Go.import("github.com/theapemachine/symm/nomagique/network/webrtc");

using import "../../runtime/status.capnp".Status;

interface WebRTCServer {
  write @0 (data :Data, track :Text) -> stream;
  done @1 () -> (status :Status, out :Data);
}
