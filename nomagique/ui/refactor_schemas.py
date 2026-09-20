import os
import glob
import subprocess

schemas = {
    "broadcast": ["""struct WireBroadcast {
  payload @0 :AnyPointer;
  server @1 :AnyPointer;
}""", """interface Broadcast {
  write @0 (broadcast :WireBroadcast) -> stream;
  done @1 ();
}"""],
    "http": ["""struct WireHTTPServer {
  payload @0 :AnyPointer;
  addr @1 :Text;
  path @2 :Text;
  templatePath @3 :Text;
  fsRoot @4 :Text;
}""", """interface HTTPServer {
  write @0 (server :WireHTTPServer) -> stream;
  done @1 ();
}"""],
    "websocket": ["""struct WireWebSocketServer {
  payload @0 :AnyPointer;
  addr @1 :Text;
  path @2 :Text;
}""", """interface WebSocketServer {
  write @0 (server :WireWebSocketServer) -> stream;
  done @1 ();
}"""],
    "webrtc": ["""struct WireWebRTCServer {
  payload @0 :AnyPointer;
  addr @1 :Text;
  path @2 :Text;
}""", """interface WebRTCServer {
  write @0 (server :WireWebRTCServer) -> stream;
  done @1 ();
}"""]
}

for name, body in schemas.items():
    id_out = subprocess.check_output(["capnpc", "-i"]).decode("utf-8").strip()
    content = f"""using Go = import "/go.capnp";
{id_out};
$Go.package("ui");
$Go.import("nomagique/ui");

{body[0]}

{body[1]}
"""
    with open(f"/Users/theapemachine/go/src/github.com/theapemachine/symm/nomagique/ui/{name}.capnp", "w") as f:
        f.write(content)
