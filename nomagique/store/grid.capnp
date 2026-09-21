using Go = import "/go.capnp";
@0xa66318359218d6a8;
$Go.package("store");
$Go.import("github.com/theapemachine/symm/nomagique/store");

using import "../runtime/status.capnp".Status;
using import "../data/metric.capnp".MetricService;

# Grid is the virtual grid. Raw market data is written to it and the metrics
# wired into it receive the fields they declared an interest in.
#
# Every feed lands on the one data port. It gathers rather than replaces, so
# adding a venue is wiring one more producer into it rather than widening the
# schema.
#
# What a metric asked for comes back out already typed, one value per declared
# interest and in the order they were declared, so a metric is handed the
# numbers it needs rather than a document it has to go looking through.
#
# The grid holds no values of its own. A metric is not a cell holding a number
# but a capability the grid calls, so reading the grid is asking every metric
# wired into it for its current state. Wiring another metric in is what makes
# the grid wider; nothing here enumerates them.
interface Grid {
  write @0 (
    data      :List(Data),
    interests :Text,
    metrics   :List(MetricService)
  ) -> stream;
  done @1 () -> (
    values    :List(Float64),
    out       :Data,
    delivered :Int64,
    metrics   :Int64,
    status    :Status
  );
}
