using Go = import "/go.capnp";
@0xe5bf838d3d21501d;
$Go.package("tables");
$Go.import("github.com/theapemachine/symm/nomagique/store/tables");

using import "../../runtime/status.capnp".Queued;

# Reads a pinned, already-loaded Iceberg metadata document.
# Catalog requests and row projection are separate graph operations.
# One row is handed out per evaluation; pending stays positive until the
# snapshot is exhausted, so the rows still unread keep the graph running.
interface IcebergScan @0xefa0adbd7a396d92 extends(Queued) {
  write @0 (metadata :List(Data), properties :List(Data)) -> stream;
  done @1 () -> Scanned;
}

struct Scanned {
 union { out @0 :Data; idle @2 :Void; }
 exhausted @1 :Bool;
 pending @3 :UInt64;
}
