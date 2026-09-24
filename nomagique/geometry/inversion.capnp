using Go = import "/go.capnp";
@0x97ff8aa1bfdc5083;
$Go.package("geometry");
$Go.import("github.com/theapemachine/symm/nomagique/geometry");

# Inversion turns signed relationship strengths into target distances in
# lattice cells. Strength has no unit of its own, so each is read relative to
# the root mean square r of the strengths handed over together: s' = s / r.
# A positive s' wants the pair closer than one cell, at 1/(1+s'); zero wants
# one cell; a negative s' wants them apart, at 1-s'. With no strength anywhere
# every pair is asked one cell. One cell is the spacing original coordinates
# start at.
interface Inversion {
  write @0 (strength :List(Float64)) -> stream;
  done @1 () -> (distance :List(Float64));
}
