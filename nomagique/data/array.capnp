@0xa1b654a4b5836148;

using Go = import "/go.capnp";
$Go.package("data");
$Go.import("github.com/theapemachine/symm/nomagique/data");

# Array writes a list as a JSON array. Exactly one of numbers, integers, texts
# or flags carries the list. stride and offset pick every stride-th element starting at
# offset, so interleaved records (x0,y0,x1,y1,...) come apart into columns; an
# unset stride takes every element. A list that did not arrive is not an empty
# list: with none delivered there is no array.
interface Array {
  write @0 (
    numbers  :List(Float64),
    integers :List(Int64),
    texts    :List(Text),
    flags   :List(Bool),
    stride  :UInt32,
    offset  :UInt32
  ) -> stream;
  done @1 () -> (out :Data);
}
