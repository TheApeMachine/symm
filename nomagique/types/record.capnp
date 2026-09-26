@0xeeb98dd74ab456e3;
using Go = import "/go.capnp";
$Go.package("types");
$Go.import("github.com/theapemachine/symm/nomagique/types");

# A schema-identified native record at a generic storage boundary.
struct Record {
  typeId @0 :UInt64;
  value @1 :AnyPointer;
}
