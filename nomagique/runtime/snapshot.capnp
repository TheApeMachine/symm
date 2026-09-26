@0xfcae017eb873db21;
using Go = import "/go.capnp";
$Go.package("runtime");
$Go.import("github.com/theapemachine/symm/nomagique/runtime");
using import "consumer.capnp".Stage;
using import "status.capnp".Durable;

# Snapshot transfers retained state through the owning capability.
interface Snapshot {
 snapshot @0 () -> (data :Data);
 restore @1 (data :Data) -> ();
}
interface State extends(Stage, Snapshot) {}
# A save durably replaces one complete named state, never a partially written set.
interface Checkpoint extends(Durable) {
 load @0 (key :Text) -> (data :Data);
 save @1 (key :Text, data :Data) -> ();
}
struct SnapshotSet {
 version @0 :Text;
 entries @1 :List(Entry);
 struct Entry { name @0 :Text; interfaceId @1 :UInt64; configuration @2 :Data; data @3 :Data; }
}
