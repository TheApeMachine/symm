using Go = import "/go.capnp";
@0xe2a7c40b9d135f86;
$Go.package("cognition");
$Go.import("github.com/theapemachine/symm/nomagique/cognition");

# Memory is the learned memory: the sequences that have been observed and how
# strongly each has been reinforced.
#
# It is one owner wired into the graph, not a structure each node happens to
# hold. A reader and a writer that construct their own learn nothing from each
# other: the writer records into one and the reader walks another, and the
# system runs perfectly while knowing nothing.
interface Memory {
  # Records that a sequence was observed, and reports what it now carries.
  reinforce @0 (key :Data) -> (weight :UInt64, lastSeen :UInt64);

  # Everything observed in one basin: the classes that followed it and what
  # each carries. A basin never observed returns nothing, which is not the
  # same as a basin whose classes all carry zero.
  basin @1 (prefix :Data) -> (classes :List(Data), weights :List(UInt64));

  steps @2 () -> (out :UInt64);
}
