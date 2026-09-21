using Go = import "/go.capnp";
@0xa66318359218d6a8;
$Go.package("store");
$Go.import("github.com/theapemachine/symm/nomagique/store");

using import "../runtime/status.capnp".Status;
using import "../data/metric.capnp".MetricService;

# Grid is the virtual grid. Raw market data is written to it and the metrics
# wired into it receive the fields they declared an interest in.
#
# The grid holds no values of its own. A metric is not a cell holding a number
# but a capability the grid calls, so reading the grid is asking every metric
# wired into it for its current state. Wiring another metric in is what makes
# the grid wider; nothing here enumerates them.
interface Grid {
  write @0 (
    data      :Data,
    interests :Text,
    metrics   :List(MetricService)
  ) -> stream;
  done @1 () -> (
    out       :Data,
    delivered :Int64,
    metrics   :Int64,
    status    :Status
  );
}
