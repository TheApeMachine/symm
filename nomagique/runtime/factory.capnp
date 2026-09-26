using Go = import "/go.capnp";
@0x99b7f94d23dab041;
$Go.package("runtime");
$Go.import("github.com/theapemachine/symm/nomagique/runtime");
using import "consumer.capnp".Stage;
# Each creation owns an independent instance of the authored node graph.
interface StageFactory {
 create @0 () -> (stage :Stage);
}
