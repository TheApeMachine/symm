@0xcecaa9bb2955a7cf;

using Go = import "/go.capnp";
$Go.package("learning");
$Go.import("github.com/theapemachine/symm/nomagique/learning");

struct IssueAction {
  step @0 :Int64;
  reference @1 :Float64;
  features @2 :List(Float64);
  predictions @3 :List(Float64);
  horizon @4 :Int64;
}
struct ResolveAction {
  step @0 :Int64;
  reference @1 :Float64;
}

interface TemporalLedger {
  write @0 (issue :IssueAction, resolve :ResolveAction) -> stream;
  done @1 ();
}
