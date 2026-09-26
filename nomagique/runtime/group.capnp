using Go = import "/go.capnp";
@0xe3ca725840f19b6d;
$Go.package("runtime");
$Go.import("github.com/theapemachine/symm/nomagique/runtime");
using import "consumer.capnp".Consumer;
using import "status.capnp".Configured;

# One group supplies the consumers that share a native LMAX barrier.
# Members are fixed when Workspace acquires them.
interface Group extends(Configured) {
  write @0 (consumers :List(Consumer)) -> stream;
  done @1 () -> (count :UInt32);
  members @2 () -> (consumers :List(Consumer));
}
