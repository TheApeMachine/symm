using Go = import "/go.capnp";
@0xc8d7a12b3e4f568a;
$Go.package("associative");
$Go.import("github.com/theapemachine/symm/nomagique/learning/associative");
interface Grid {
 write @0 (data :Data, width :UInt32, height :UInt32, frozen :Data) -> stream;
 done @1 () -> Remapped;
}
struct Remapped {
 model @0 :Data;
 union {
  unsettled @1 :Void;
  settled :group { out @2 :Data; }
 }
}
