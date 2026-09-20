@0xa30d63db64b1f5c7;

using Go = import "/go.capnp";
$Go.package("algo");
$Go.import("github.com/theapemachine/symm/nomagique/algo");

interface GaussJordan {
  write @0 (
    a11 :Float64,
    a12 :Float64,
    a21 :Float64,
    a22 :Float64,
    b1 :Float64,
    b2 :Float64
  ) -> stream;
  done @1 () -> (
    x1 :Float64,
    x2 :Float64
  );
}
