@0xbcc23f7e1cf8bcc4;

using Go = import "/go.capnp";
$Go.package("probability");
$Go.import("github.com/theapemachine/symm/nomagique/probability");

interface NormalKullbackLeibler {
  write @0 (
    muL :Float64,
    sigmaL :Float64,
    muR :Float64,
    sigmaR :Float64,
  ) -> stream;

  done @1 () -> (
    kl :Float64,
  );
}
