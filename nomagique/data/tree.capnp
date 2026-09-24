@0x826d38d256c4ec4e;

using Go = import "/go.capnp";
$Go.package("data");
$Go.import("github.com/theapemachine/symm/nomagique/data");

# Tree grows a prefix tree from paths. items is a JSON array of
# {"path": "...", "label": "..."}; a path is steps joined by separator, and a
# step may itself list several tokens joined by commas. Every node counts the
# paths through it, and its probability is that count over all paths. A node
# where paths end is an end, and each label seen ending there is a candidate,
# with the share of endings there that carried it. root is the tree as a JSON
# object {id, prefix, probability, tokens, label, share, visits, isEnd, state,
# children}; candidates
# is a JSON array {id, rank, action, prefix, probability, state}, strongest
# first. Every end also carries, on the node itself, label (the label most of
# the paths ending there carried), share (that label's part of those endings)
# and visits (the paths through it), so a node reads as what was recorded
# there and the step leading into it as its tokens. branches lists every end as {id, signature, depth, visits,
# confidence, policy}: its path, how many steps deep, how many paths ended
# there, and the label most of them carried with its share. With no paths
# there is no tree, and it is idle.
interface Tree {
  write @0 (items :Data, separator :Text) -> stream;
  done @1 () -> Grown;
}

struct Grown {
  union {
    idle  @0 :Void;
    grown :group {
      root       @1 :Data;
      candidates @2 :Data;
      branches   @3 :Data;
    }
  }
}
