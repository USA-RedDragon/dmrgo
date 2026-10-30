// Command serve builds the WebAssembly decoder and serves the page of the
// decoder example on localhost, with the wasm_exec.js of the compiler and
// the I/Q fixtures of the repository under /testdata/.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type options struct {
	addr   *string
	build  *bool
	tinygo *bool
	files  *string
}

func defineFlags(fs *flag.FlagSet) options {
	return options{
		addr:   fs.String("addr", "localhost:8080", "listen address; WebUSB and the microphone need localhost or HTTPS"),
		build:  fs.Bool("build", true, "build the decoder before serving"),
		tinygo: fs.Bool("tinygo", false, "build and serve the decoder with TinyGo instead of Go"),
		files:  fs.String("files", "", "a directory of recordings to serve under /files/, for the src parameter of the page"),
	}
}

func main() {
	o := defineFlags(flag.CommandLine)
	flag.Parse()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		log.Fatal("serve: cannot find the example directory")
	}
	dir := filepath.Dir(filepath.Dir(file))
	wasm := filepath.Join(dir, "decoder.wasm")
	compiler := "go"
	args := []string{"build", "-trimpath", "-ldflags=-s -w", "-o", wasm, "."}
	if *o.tinygo {
		wasm = filepath.Join(dir, "decoder-tinygo.wasm")
		compiler = "tinygo"
		args = []string{"build", "-target", "wasm", "-opt=2", "-no-debug", "-o", wasm, "."}
	}
	if *o.build {
		cmd := exec.CommandContext(context.Background(), compiler, args...)
		cmd.Dir, cmd.Stdout, cmd.Stderr = dir, os.Stdout, os.Stderr
		cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
		if err := cmd.Run(); err != nil {
			log.Fatalf("serve: %s build: %v", compiler, err)
		}
	}
	execJS, err := wasmExec(*o.tinygo)
	if err != nil {
		log.Fatal("serve: ", err)
	}
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir(dir)))
	mux.HandleFunc("/wasm_exec.js", func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, execJS) })
	mux.HandleFunc("/decoder.wasm", func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, wasm) })
	testdata := filepath.Join(dir, "..", "..", "..", "cmd", "dmrrf", "testdata")
	mux.Handle("/testdata/", http.StripPrefix("/testdata/", http.FileServer(http.Dir(testdata))))
	if *o.files != "" {
		mux.Handle("/files/", http.StripPrefix("/files/", http.FileServer(http.Dir(*o.files))))
	}
	fmt.Printf("serving %s (%s) on http://%s/\n", dir, filepath.Base(wasm), *o.addr)
	srv := &http.Server{Addr: *o.addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(srv.ListenAndServe())
}

func wasmExec(tinygo bool) (string, error) {
	var paths []string
	if tinygo {
		out, err := exec.CommandContext(context.Background(), "tinygo", "env", "TINYGOROOT").Output()
		if err != nil {
			return "", fmt.Errorf("tinygo env TINYGOROOT: %w", err)
		}
		paths = []string{filepath.Join(strings.TrimSpace(string(out)), "targets", "wasm_exec.js")}
	} else {
		out, err := exec.CommandContext(context.Background(), "go", "env", "GOROOT").Output()
		if err != nil {
			return "", fmt.Errorf("go env GOROOT: %w", err)
		}
		root := strings.TrimSpace(string(out))
		paths = []string{filepath.Join(root, "lib", "wasm", "wasm_exec.js"), filepath.Join(root, "misc", "wasm", "wasm_exec.js")}
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("no wasm_exec.js at %s", strings.Join(paths, " or "))
}
