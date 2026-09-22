@0xc6bb0f8ed96e461c;

using Go = import "/go.capnp";
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

interface KullbackLeibler {
  write @0 (
    p :List(Float64),
    q :List(Float64),
  ) -> stream;

  done @1 () -> (
    kullbackLeibler :Float64,
  );
}
