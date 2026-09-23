@0xb17337a0c49d691f;

using Go = import "/go.capnp";
$Go.package("store");
$Go.import("github.com/theapemachine/symm/nomagique/store");

# Index keeps appended documents in arrival order, one list per partition, and
# answers requests against a partition without walking the others. It is the
# run index a replay reads fragments from: the tape is streamed in once, and a
# fragment is a contiguous range of one partition.
#
# Partition is a comma-separated list of paths whose values together name a
# document's partition (a capture session and an instrument, say); a request
# names its partition as the array of those values, and a path a document
# lacks is null in its partition. Order is a comma-separated list of paths
# holding exact numbers; documents must be appended in that order within their
# partition, and appending one that sorts before its predecessor is an error.
#
# Each request is a JSON document, answered in request order:
#   {"partition": [..], "position": n}              the document at position n
#   {"partition": [..], "seek": "first", "from": D} the first document ordered at or after D
#   {"partition": [..], "seek": "last", "from": D, "where": {path: value}}
#                                                   the last at or before D whose fields equal where
# An answer is {"found": bool, "position": n, "count": c, "document": ...};
# a position past the end, or a seek with no such document, is not found.
# Answers line up with request slots; a slot nothing asked on is empty.
#
# Every port gathers, so the index runs on every evaluation with whatever
# arrived.
interface Index {
  write @0 (append :List(Data), request :List(Data), partition :Text, order :Text) -> stream;
  done @1 () -> (answers :List(Data));
}
