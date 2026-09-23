@0xc8ff75b27bdab2e7;

using Go = import "/go.capnp";
$Go.package("paper");
$Go.import("github.com/theapemachine/symm/nomagique/financial/paper");

# Sweep walks price levels, best first, the way a market order takes them.
#
# With spend, amount is a quote budget: whole levels are bought while it pays
# for them, and the level it cannot pay for in full gives the largest multiple
# of increment it can. Without spend, amount is a base quantity delivered into
# the levels. What the levels could not absorb is unfilled, never assumed
# filled beyond what they show.
interface Sweep {
  write @0 (levels :Data, amount :Data, increment :Data, spend :Bool) -> stream;
  done @1 () -> (quantity :Data, cost :Data, unfilled :Data);
}
