using Go = import "/go.capnp";
@0x9cc2ec51223481bc;
$Go.package("data");
$Go.import("github.com/theapemachine/symm/nomagique/data");

using import "../runtime/status.capnp".Status;

# CanonicalizeFutures maps Kraken Futures raw websocket frames (ticker and trade)
# to the canonical channel/data representation expected by the signal graph.
interface CanonicalizeFutures {
  write @0 (data :Data) -> stream;
  done @1 () -> (out :Data, status :Status);
}
