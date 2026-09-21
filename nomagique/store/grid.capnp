using Go = import "/go.capnp";
@0xa66318359218d6a8;
$Go.package("store");
$Go.import("nomagique/store");

using import "../runtime/status.capnp".Status;

# Grid is the virtual grid: raw market data is written to it, and the metrics
# registered with it receive only the fields they declared an interest in.
#
# A metric registers by declaring its interest keys, which are dotted paths
# into the data written to the grid. Writing data distributes to every metric
# whose interests that data satisfies, so a metric observes what it asked for
# and nothing else.
interface Grid {
  write @0 (
    data      :Data,
    metric    :Text,
    interests :Text
  ) -> stream;
  done @1 () -> (
    out       :Data,
    metric    :Text,
    delivered :Int64,
    metrics   :Int64,
    status    :Status
  );
}
