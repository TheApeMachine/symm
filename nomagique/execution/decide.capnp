using Go = import "/go.capnp";
@0xe9fc9395f4800f4b;
$Go.package("execution");
$Go.import("nomagique/execution");

interface Decide {
  write @0 (
    in :Data,
    winner :Text,
    contrast :Float64,
    isBreak :Bool,
    minContrast :Float64
  ) -> stream;
  done @1 () -> (out :Text);
}
