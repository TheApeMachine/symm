using Go = import "/go.capnp";
@0x97ff8aa1bfdc5083;
$Go.package("geometry");
$Go.import("github.com/theapemachine/symm/nomagique/geometry");

# Inversion turns signed relationship strengths into target distances in
# lattice cells. A positive strength s wants the pair closer than one cell,
# at 1/(1+s); zero wants one cell; a negative strength wants them apart, at
# 1-s. One cell is the spacing original coordinates start at.
interface Inversion {
  write @0 (strength :List(Float64)) -> stream;
  done @1 () -> (distance :List(Float64));
}
