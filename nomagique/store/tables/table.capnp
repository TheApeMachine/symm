using Go = import "/go.capnp";
@0xc7e14a3b8d9a2c1f;
$Go.package("tables");
$Go.import("github.com/theapemachine/symm/nomagique/store/tables");

# IcebergTable appends what it is written.
#
# Every append is a snapshot plus a metadata write, so committing once per
# observation makes one snapshot per frame and the catalog becomes the clock.
# Catalog access and append run on the node's persistence worker. Write and
# done never wait on that I/O; flush joins it. Pending bytes are bounded by the
# declared maxPendingBytes (two append buffers when omitted); full admission
# fails before accepting any row in the batch. Rows are held until there is a reason to send them: enough bytes to be worth
# a snapshot, or a caller saying now. rows gathers: every row in it is appended
# as payload is, in order, so a producer with several rows at once hands them
# over together. Native records carry their schema identity and bypass JSON
# encoding and decoding; the worker writes their typed slots directly to Arrow.
using import "../../runtime/status.capnp".Durable;

interface IcebergTable extends(Durable) {
  write @0 (config :Text, payload :Data, commit :Bool, rows :List(Data), record :import "../../types/record.capnp".Record) -> stream;
  done @1 () -> (table :Text, pending :Int64, committed :Int64, bytes :Int64);
}
