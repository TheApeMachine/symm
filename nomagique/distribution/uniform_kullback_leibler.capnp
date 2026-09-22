@0xaf311e6d56a620a2;

using Go = import "/go.capnp";
$Go.package("distribution");
$Go.import("github.com/theapemachine/symm/nomagique/distribution");

interface UniformKullbackLeibler {
  write @0 (
    minL :List(Float64),
    maxL :List(Float64),
    minR :List(Float64),
    maxR :List(Float64),
    dim :Int32,
  ) -> stream;

  done @1 () -> (
    kl :Float64,
  );
}
