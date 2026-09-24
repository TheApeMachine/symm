using Go = import "/go.capnp";
@0xe7e419f7a1369d1a;
$Go.package("data");
$Go.import("github.com/theapemachine/symm/nomagique/data");

# Arrow projects every row of an Arrow IPC stream, in stream order, as one
# JSON document per row typed by its fields. A stream is handed over whole, so
# an archive is read a batch per evaluation rather than a row.
interface Arrow {
  write @0 (data :Data) -> stream;
  done @1 () -> (rows :List(Data));
}
