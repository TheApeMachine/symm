@0xde301da0ebce33dd;

using Go = import "/go.capnp";
$Go.package("distribution");
$Go.import("github.com/theapemachine/symm/nomagique/distribution");

interface DirichletKullbackLeibler {
  write @0 (
    alphaL :List(Float64),
    alphaR :List(Float64),
  ) -> stream;

  done @1 () -> (
    kl :Float64,
  );
}
