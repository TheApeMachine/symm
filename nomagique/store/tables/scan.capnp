using Go = import "/go.capnp";
@0xe5bf838d3d21501d;
$Go.package("tables");
$Go.import("github.com/theapemachine/symm/nomagique/store/tables");

# Reads a pinned, already-loaded Iceberg metadata document.
# Catalog requests and row projection are separate graph operations.
interface IcebergScan @0xefa0adbd7a396d92 {
  write @0 (metadata :List(Data), properties :Data) -> stream;
  done @1 () -> Scanned;
}

struct Scanned {
 union { out @0 :Data; idle @2 :Void; }
 exhausted @1 :Bool;
}
