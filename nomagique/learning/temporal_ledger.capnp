@0xcecaa9bb2955a7cf;

using Go = import "/go.capnp";
$Go.package("learning");
$Go.import("github.com/theapemachine/symm/nomagique/learning");

interface TemporalLedger {
  write @0 (issueStep :Int64, issueReference :Float64, resolveStep :Int64, resolveReference :Float64) -> stream;
  done @1 () -> (out :Float64);
}
