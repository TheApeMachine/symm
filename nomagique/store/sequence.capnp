@0x979bf36443adf0bd;
using Go = import "/go.capnp";
$Go.package("store");
$Go.import("github.com/theapemachine/symm/nomagique/store");

# An append-only sequence of immutable graph values. Positions are zero-based
# storage addresses, not market timestamps. The graph supplies every read index.
# Empty append slots mean no arrival. Reading past the end returns missing.
interface Sequence {
 write @0 (append :List(Data), index :UInt64) -> stream;
 done @1 () -> Item;
}
struct Item {
 count @0 :UInt64;
 found @1 :Bool;
 union {
  missing @2 :Void;
  item :group {
   out @3 :Data;
   next @4 :UInt64;
  }
 }
}
