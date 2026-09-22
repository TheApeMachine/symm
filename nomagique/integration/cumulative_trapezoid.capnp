@0xfb2e0da4a1d502c1;

using Go = import "/go.capnp";
$Go.package("integration");
$Go.import("github.com/theapemachine/symm/nomagique/integration");

interface CumulativeTrapezoid {
  write @0 (
    x :List(Float64),
    y :List(Float64),
  ) -> stream;

  done @1 () -> (
    cumulative :List(Float64),
    total :Float64,
  );
}
