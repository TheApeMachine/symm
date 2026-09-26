using Go = import "/go.capnp";
@0x99b7f94d23dab041;
$Go.package("runtime");
$Go.import("github.com/theapemachine/symm/nomagique/runtime");
using import "consumer.capnp".Stage;
using import "status.capnp".Configured;
# Each creation owns an independent instance of the authored node graph.
interface StageFactory extends(Stage, Configured) {
 create @0 () -> (stage :Stage);
 # When used as a stage, select a child by a native Text output from an
 # earlier LMAX group. Empty selectors support explicit create calls only.
 write @1 (producer :Text, node :Text, field :Text) -> stream;
 done @2 () -> (partitions :UInt64);
}
