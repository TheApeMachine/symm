using Go = import "/go.capnp";
@0xc7e14a3b8d9a2c1f;
$Go.package("tables");
$Go.import("github.com/theapemachine/symm/nomagique/store/tables");

# IcebergTable appends what it is written.
#
# Every append is a snapshot plus a metadata write, so committing once per
# observation makes one snapshot per frame and the catalog becomes the clock.
# Rows are held until there is a reason to send them: enough bytes to be worth
# a snapshot, or a caller saying now. rows gathers: every row in it is appended
# as payload is, in order, so a producer with several rows at once hands them
# over together.
using import "../../runtime/status.capnp".Durable;

interface IcebergTable extends(Durable) {
  write @0 (config :Text, payload :Data, commit :Bool, rows :List(Data)) -> stream;
  done @1 () -> (out :Data, pending :Int64, committed :Int64, bytes :Int64);
}
