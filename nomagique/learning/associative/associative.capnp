using Go = import "/go.capnp";
@0xc8d7a12b3e4f568a;
$Go.package("associative");
$Go.import("nomagique/learning/associative");

struct WireGrid {
  impulse @0 :List(Float64);
}

interface Grid {
  write @0 (grid :WireGrid) -> stream;
  done @1 ();
}
