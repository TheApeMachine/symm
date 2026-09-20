using Go = import "/go.capnp";
@0xc7b508f5d023b3a3;
$Go.package("geometry");
$Go.import("nomagique/geometry");

struct WirePhaseDial {
  # Representing []complex128 as a list of float64 pairs (real, imag)
  components @0 :List(Float64);
}

struct WireOverlapPair {
  probe @0 :WirePhaseDial;
  entry @1 :WirePhaseDial;
}

struct WirePhasePathReading {
  angles @0 :List(Float64);
}

struct WireCorpusEntry {
  dial @0 :WirePhaseDial;
  outcome @1 :AnyPointer;
  at @2 :Int64;
}

struct WireCorpusMatch {
  outcome @0 :AnyPointer;
  at @1 :Int64;
  similarity @2 :Float64;
}

struct WireCorpusQuery {
  dial @0 :WirePhaseDial;
  angles @1 :List(Float64);
  topK @2 :Int32;
  excludeTimes @3 :List(Int64);
}

struct WireCorpusCommand {
  union {
    insert @0 :WireCorpusEntry;
    query @1 :WireCorpusQuery;
    count @2 :Void;
  }
}

struct WireCorpusResult {
  inserted @0 :Bool;
  size @1 :Int32;
  # List of lists of matches, modeled as a struct containing the inner list
  struct MatchList {
    matches @0 :List(WireCorpusMatch);
  }
  scan @2 :List(MatchList);
}

interface Normalize {
  execute @0 (dial :WirePhaseDial) -> (dial :WirePhaseDial);
}

interface Overlap {
  execute @0 (pair :WireOverlapPair) -> (real :Float64, imag :Float64);
}

interface PhasePath {
  execute @0 (samples :Int32) -> (reading :WirePhasePathReading);
}

interface Corpus {
  execute @0 (command :WireCorpusCommand) -> (result :WireCorpusResult);
}
