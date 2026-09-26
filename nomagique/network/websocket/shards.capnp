using Go = import "/go.capnp";
@0xabc405b9d6da6021;
$Go.package("websocket");
$Go.import("github.com/theapemachine/symm/nomagique/network/websocket");
using import "../../runtime/status.capnp".Source;
using import "../../runtime/factory.capnp".StageFactory;
using import "client.capnp".Received;
# Stable symbol membership; additional symbols create additional graph capabilities.
# Capacity is a per-connection venue limit, never a universe limit.
interface Shards extends(Source) {
 write @0 (factory :StageFactory, symbols :List(Text), capacity :UInt32) -> stream;
 done @1 () -> Received;
}
