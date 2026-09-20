import os
import glob
import subprocess

files = glob.glob("/Users/theapemachine/go/src/github.com/theapemachine/symm/nomagique/transport/*.capnp*") + glob.glob("/Users/theapemachine/go/src/github.com/theapemachine/symm/nomagique/transport/*.go")
for f in files:
    if not f.endswith("_test.go.bak") and "refactor.py" not in f and os.path.exists(f):
        os.remove(f)

schemas = {
    "tee": ["""struct WireTee { payload @0 :AnyPointer; offramps @1 :AnyPointer; }""", "Tee", "WireTee"],
    "discard": ["""struct WireDiscard { payload @0 :AnyPointer; }""", "Discard", "WireDiscard"],
    "fan": ["""struct WireFan { payload @0 :AnyPointer; branches @1 :AnyPointer; }""", "Fan", "WireFan"],
    "parallel": ["""struct WireParallel { payloads @0 :AnyPointer; task @1 :AnyPointer; }""", "Parallel", "WireParallel"],
    
    "nonce": ["""struct WireNonce { }""", "Nonce", "WireNonce"],
    "timestamp": ["""struct WireTimestamp { }""", "Timestamp", "WireTimestamp"],
    "sha256": ["""struct WireSHA256 { data @0 :Data; }""", "SHA256", "WireSHA256"],
    "hmacsha512": ["""struct WireHMACSHA512 { message @0 :Data; secret @1 :Data; }""", "HMACSHA512", "WireHMACSHA512"],
    "hmacsha256": ["""struct WireHMACSHA256 { message @0 :Data; secret @1 :Data; }""", "HMACSHA256", "WireHMACSHA256"],
    "base64encode": ["""struct WireBase64Encode { data @0 :Data; }""", "Base64Encode", "WireBase64Encode"],
    "base64decode": ["""struct WireBase64Decode { data @0 :Text; }""", "Base64Decode", "WireBase64Decode"],
    "headerauth": ["""struct WireHeaderAuth { headers @0 :AnyPointer; key @1 :Text; value @2 :Text; }""", "HeaderAuth", "WireHeaderAuth"],
    "bearerauth": ["""struct WireBearerAuth { headers @0 :AnyPointer; token @1 :Text; }""", "BearerAuth", "WireBearerAuth"],
    
    "jsonencode": ["""struct WireJSONEncode { payload @0 :AnyPointer; }""", "JSONEncode", "WireJSONEncode"],
    "jsondecode": ["""struct WireJSONDecode { data @0 :Data; }""", "JSONDecode", "WireJSONDecode"],
    
    "httprequest": ["""struct WireHTTPRequest { url @0 :Text; method @1 :Text; body @2 :Data; headers @3 :AnyPointer; }""", "HTTPRequest", "WireHTTPRequest"],
    
    "pace": ["""struct WirePace { payload @0 :AnyPointer; interval @1 :Int64; }""", "Pace", "WirePace"],
    
    "process": ["""struct WireProcess { executable @0 :Text; args @1 :List(Text); }""", "Process", "WireProcess"],
    
    "stream": ["""struct WireStream { payload @0 :AnyPointer; }""", "Stream", "WireStream"],
    "fork": ["""struct WireFork { payload @0 :AnyPointer; routes @1 :List(Text); }""", "Fork", "WireFork"],
    "join": ["""struct WireJoin { payloads @0 :AnyPointer; }""", "Join", "WireJoin"],
    "route": ["""struct WireRoute { payload @0 :AnyPointer; destination @1 :Text; }""", "Route", "WireRoute"],
    "gate": ["""struct WireGate { payload @0 :AnyPointer; condition @1 :Text; }""", "Gate", "WireGate"],
    "broadcast": ["""struct WireBroadcast { payload @0 :AnyPointer; channels @1 :List(Text); }""", "Broadcast", "WireBroadcast"],
    
    "wsconnect": ["""struct WireWSConnect { url @0 :Text; }""", "WSConnect", "WireWSConnect"],
    "wsread": ["""struct WireWSRead { connection @0 :AnyPointer; }""", "WSRead", "WireWSRead"],
    "wswrite": ["""struct WireWSWrite { connection @0 :AnyPointer; payload @1 :Data; }""", "WSWrite", "WireWSWrite"],
    "wsclose": ["""struct WireWSClose { connection @0 :AnyPointer; }""", "WSClose", "WireWSClose"],
    "wspingpong": ["""struct WireWSPingPong { connection @0 :AnyPointer; }""", "WSPingPong", "WireWSPingPong"],
    "wsencodejson": ["""struct WireWSEncodeJSON { payload @0 :AnyPointer; }""", "WSEncodeJSON", "WireWSEncodeJSON"],
    "wsdecodejson": ["""struct WireWSDecodeJSON { data @0 :Data; }""", "WSDecodeJSON", "WireWSDecodeJSON"],
    "wsjsonmessage": ["""struct WireWSJSONMessage { type @0 :Text; payload @1 :AnyPointer; }""", "WSJSONMessage", "WireWSJSONMessage"],
    "wsbatch": ["""struct WireWSBatch { messages @0 :AnyPointer; }""", "WSBatch", "WireWSBatch"],
}

for name, body in schemas.items():
    id_out = subprocess.check_output(["capnpc", "-i"]).decode("utf-8").strip()
    content = f"""using Go = import "/go.capnp";
{id_out};
$Go.package("transport");
$Go.import("nomagique/transport");

{body[0]}

interface {body[1]} {{
  write @0 (payload :{body[2]}) -> stream;
  done @1 ();
}}
"""
    with open(f"/Users/theapemachine/go/src/github.com/theapemachine/symm/nomagique/transport/{name}.capnp", "w") as f:
        f.write(content)

    go_content = f"""package transport

import (
	"context"
)

type {body[1]}Server struct {{
	Downstream func(context.Context, any) error
}}

func New{body[1]}Server() *{body[1]}Server {{
	return &{body[1]}Server{{}}
}}

func (s *{body[1]}Server) Write(ctx context.Context, call {body[1]}_write) error {{
	if s.Downstream != nil {{
		// Placeholder for {body[1]} processing
		return s.Downstream(ctx, nil)
	}}
	return nil
}}

func (s *{body[1]}Server) Done(ctx context.Context, call {body[1]}_done) error {{
	return nil
}}
"""
    with open(f"/Users/theapemachine/go/src/github.com/theapemachine/symm/nomagique/transport/{name}.go", "w") as f:
        f.write(go_content)
