using Go = import "/go.capnp";
@0xcc19c1ec7d4a1b39;
$Go.package("runtime");
$Go.import("github.com/theapemachine/symm/nomagique/runtime");
using import "status.capnp".Status;
using import "status.capnp".Durable;
using import "status.capnp".Queued;
using import "status.capnp".Source;
using import "consumer.capnp".Stage;
using import "group.capnp".Group;

# Groups run in their wired order. Consumers within a group run concurrently.
# Native LMAX gates each later group on completion of every earlier consumer.
# With advance enabled, the first group polls sources concurrently. One ring
# cycle contains one observation per source, ordered by its wired position.
# Later groups finish every observation before the native barrier advances.
interface Workspace extends(Stage, Durable, Queued, Source) {
  write @0 (data :List(Data), capacity :UInt32, writers :UInt8,
            admit :Bool, epoch :Int64, groups :List(Group), advance :Bool) -> stream;
  done @1 () -> (epoch :Int64, published :Int64, completed :Int64,
                pending :UInt64, status :Status, data :Data);
}
