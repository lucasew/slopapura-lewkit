// Command slopapura is the SlopApura eletrocromo app.
// The page is rendered on the server. htmx swaps that HTML.
//
//go:generate go tool templ generate
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/lewtec/lewkit/x/app"
	"github.com/lewtec/lewkit/x/entry"
	"github.com/lewtec/lewkit/x/release"

	_ "github.com/lewtec/lewkit/x/driver/httpclient/native"
	_ "time/tzdata"
)

func init() { entry.Bind(run) }

var listenFlag string

func main() {
	listenFlag = os.Getenv("SLOPAPURA_LISTEN")
	fs := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&listenFlag, "listen", listenFlag, "serve without a window, for example 127.0.0.1:8892")
	if err := fs.Parse(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		os.Exit(2)
	}
	entry.Main(context.Background(), run)
}

func run(ctx context.Context) error {
	handler, err := newHandler(ctx)
	if err != nil {
		return err
	}
	addr := listenFlag
	if addr == "" && !releaseStamped() {
		addr = "127.0.0.1:8892"
	}
	if addr != "" {
		return serve(ctx, addr, handler)
	}
	return app.App{
		Title:   "SlopApura",
		Width:   1280,
		Height:  800,
		Handler: app.Web(handler),
	}.Run(ctx)
}

func releaseStamped() bool {
	version := release.Version()
	if version == "dev" || strings.HasPrefix(version, "dev-") {
		return false
	}
	_, err := release.AppID()
	return err == nil
}

func serve(ctx context.Context, addr string, handler http.Handler) error {
	ln, err := net.Listen("tcp4", addr)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Handler: handler,
		BaseContext: func(net.Listener) context.Context {
			return ctx
		},
	}
	fmt.Printf("http://%s\n", ln.Addr().String())
	go func() {
		<-ctx.Done()
		shut, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shut)
	}()
	err = srv.Serve(ln)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
