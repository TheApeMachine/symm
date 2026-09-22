using Go = import "/go.capnp";
@0xe7e419f7a1369d1a;
$Go.package("data");
$Go.import("github.com/theapemachine/symm/nomagique/data");

# Projects one indexed row from an Arrow IPC stream using its field types.
interface Arrow {
  write @0 (data :Data, row :UInt64) -> stream;
  done @1 () -> (out :Data);
}
