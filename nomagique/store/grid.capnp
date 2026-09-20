using Go = import "/go.capnp";
@0xa66318359218d6a8;
$Go.package("store");
$Go.import("nomagique/store");

struct Cell {
  payload @0 :AnyPointer;
}

interface Grid {
  poke @0 (payload :AnyPointer) -> (cells :List(Cell));
  peek @1 () -> (cells :List(Cell));
  register @2 (payload :AnyPointer);
}
