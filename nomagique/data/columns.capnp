@0x950fc8e410e47c17;

using Go = import "/go.capnp";
$Go.package("data");
$Go.import("github.com/theapemachine/symm/nomagique/data");

# Columns zips named columns into rows. names lists the field names in the
# order the columns are wired; each column is a JSON array, and every column
# must be as long as the others. Row i is an object holding element i of each
# column under its name, plus its position under index when index names a
# field. Until every column has arrived there are no rows, and it is idle.
interface Columns {
  write @0 (names :Text, columns :List(Data), index :Text) -> stream;
  done @1 () -> Rows;
}

struct Rows {
  union {
    idle @0 :Void;
    out  @1 :Data;
  }
}
