import os
import glob

# We will just rewrite the entire contents of each Go file using string templates.
servers = {
    "broadcast": {
        "struct": "BroadcastServer",
        "method": "Broadcast",
        "imports": ["context", "capnproto.org/go/capnp/v3"],
        "downstream_type": "capnp.Ptr",
        "extra": "",
        "code": """	var out capnp.Ptr
	if payloadPtr.IsValid() {
		out = payloadPtr
	}
	if s.Downstream != nil {
		return s.Downstream(ctx, out)
	}
	return nil"""
    },
    "http": {
        "struct": "HTTPServerImpl",
        "method": "HTTPServer",
        "imports": ["context", "net/http", "os", "strings", "sync", "text/template", "time", "github.com/theapemachine/errnie", "capnproto.org/go/capnp/v3"],
        "downstream_type": "capnp.Ptr",
        "extra": """type HTTPServerImpl struct {
	once sync.Once
	Downstream func(context.Context, capnp.Ptr) error
}

func NewHTTPServerImpl() *HTTPServerImpl {
	return &HTTPServerImpl{}
}""",
        "code": """	addrStr, err := args.Addr()
	if err != nil {
		return err
	}
	
	pathStr, err := args.Path()
	if err != nil {
		return err
	}
	
	templatePathStr, err := args.TemplatePath()
	if err != nil {
		return err
	}
	
	fsRootStr, err := args.FsRoot()
	if err != nil {
		return err
	}

	s.once.Do(func() {
		a := ":8080"
		if addrStr != "" {
			a = addrStr
		}
		p := "/"
		if pathStr != "" {
			p = pathStr
		}
		tmplPath := ""
		if templatePathStr != "" {
			tmplPath = templatePathStr
		}
		fsRoot := ""
		if fsRootStr != "" {
			fsRoot = fsRootStr
		}

		mux := http.NewServeMux()
		
		if fsRoot != "" {
			if info, err := os.Stat(fsRoot); err == nil && info.IsDir() {
				fs := http.FileServer(http.Dir(fsRoot))
				mux.Handle(p+"static/", http.StripPrefix(p+"static/", fs))
			}
		}

		var tmpl *template.Template
		if tmplPath != "" {
			t, err := template.ParseFiles(tmplPath)
			if err == nil {
				tmpl = t
			} else {
				errnie.Error(errnie.Err(errnie.IO, "[ui] failed to parse template", err))
			}
		}

		mux.HandleFunc(p, func(w http.ResponseWriter, r *http.Request) {
			if tmpl != nil {
				err := tmpl.Execute(w, map[string]any{
					"Time": time.Now().Format(time.RFC3339),
				})
				if err != nil {
					http.Error(w, "Template execution failed", http.StatusInternalServerError)
				}
			} else {
				w.Header().Set("Content-Type", "text/html")
				_, _ = w.Write([]byte("<html><body>OK</body></html>"))
			}
		})

		server := &http.Server{Addr: a, Handler: mux}
		go func() {
			if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed && !strings.Contains(err.Error(), "address already in use") {
				errnie.Error(errnie.Err(errnie.IO, "[ui] http server failed", err))
			}
		}()
	})

	if s.Downstream != nil {
		return s.Downstream(ctx, payloadPtr)
	}
	return nil"""
    },
    "webrtc": {
        "struct": "WebRTCServerImpl",
        "method": "WebRTCServer",
        "imports": ["context", "capnproto.org/go/capnp/v3"],
        "downstream_type": "capnp.Ptr",
        "extra": "",
        "code": """	if s.Downstream != nil {
		return s.Downstream(ctx, payloadPtr)
	}
	return nil"""
    },
    "websocket": {
        "struct": "WebSocketServerImpl",
        "method": "WebSocketServer",
        "imports": ["context", "capnproto.org/go/capnp/v3"],
        "downstream_type": "capnp.Ptr",
        "extra": "",
        "code": """	if s.Downstream != nil {
		return s.Downstream(ctx, payloadPtr)
	}
	return nil"""
    }
}

for filename, spec in servers.items():
    if not spec["extra"]:
        spec["extra"] = f"""type {spec["struct"]} struct {{
	Downstream func(context.Context, {spec["downstream_type"]}) error
}}

func New{spec["struct"]}() *{spec["struct"]} {{
	return &{spec["struct"]}{{}}
}}"""
        
    imports_str = "\n".join([f'\t"{i}"' for i in spec["imports"]])
    
    content = f"""package ui

import (
{imports_str}
)

{spec["extra"]}

func (s *{spec["struct"]}) Write(ctx context.Context, call {spec["method"]}_write) error {{
	args, err := call.Args().Server()
	if err != nil {{
		// fallback to see if it's named something else
		return err
	}}
	
	payloadPtr, err := args.Payload()
	if err != nil {{
		return err
	}}
	
{spec["code"]}
}}

func (s *{spec["struct"]}) Done(ctx context.Context, call {spec["method"]}_done) error {{
	return nil
}}
"""
    # Fix the method arg call for Broadcast
    if filename == "broadcast":
        content = content.replace("call.Args().Server()", "call.Args().Broadcast()")
    
    with open(f"/Users/theapemachine/go/src/github.com/theapemachine/symm/nomagique/ui/{filename}.go", "w") as f:
        f.write(content)
