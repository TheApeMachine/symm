@0xc9cc2071af54dfc7;

using Go = import "/go.capnp";
$Go.package("linalg");
$Go.import("github.com/theapemachine/symm/nomagique/linalg");

struct Matrix {
  rows @0 :Int32;
  cols @1 :Int32;
  data @2 :List(Float64);
}

struct Vector {
  data @0 :List(Float64);
}
