package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ioant.local/localservice/internal/service"
	"ioant.local/localservice/web"
)

func main() {
	addr := flag.String("listen", "0.0.0.0:8787", "LAN listen address")
	data := flag.String("data", ".localservice", "persistent data directory")
	publicURL := flag.String("public-url", "", "canonical HTTPS URL for iOS OTA; no trailing slash")
	cert := flag.String("tls-cert", "", "HTTPS certificate PEM")
	key := flag.String("tls-key", "", "HTTPS private key PEM")
	buildOrigin := flag.String("build-origin", "http://127.0.0.1:8787", "origin used by ILS build jobs to publish")
	buildRoot := flag.String("build-root", ".", "ILS checkout containing scripts/push.sh")
	flag.Parse()
	app, err := service.New(*data, *publicURL, web.Files)
	if err != nil {
		log.Fatal(err)
	}
	if err := app.ConfigureBuildRunner(*buildOrigin, *buildRoot); err != nil {
		log.Fatal(err)
	}
	defer app.StopBuild()
	server := &http.Server{Addr: *addr, Handler: app.HandlerV2(), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		app.StopBuild()
		shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	log.Printf("ILS listening on %s; admin token: %s/admin-token", *addr, *data)
	if (*cert == "") != (*key == "") {
		log.Fatal("tls-cert and tls-key must be provided together")
	}
	if *cert != "" {
		err = server.ListenAndServeTLS(*cert, *key)
	} else {
		err = server.ListenAndServe()
	}
	if err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
