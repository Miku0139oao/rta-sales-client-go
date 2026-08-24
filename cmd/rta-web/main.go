package main

import (
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/Miku0139oao/rta-sales-client-go/desktop"
)

func main() {
	listen := os.Getenv("RTA_WEB_LISTEN")
	if listen == "" {
		listen = "127.0.0.1:8787"
	}
	static := os.Getenv("RTA_WEB_STATIC")
	if static == "" {
		static = "/srv/pre-rtasales"
	}
	server := desktop.NewWebServer()
	server.Static = static
	httpServer := &http.Server{
		Addr:              listen,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       2 * time.Minute,
		IdleTimeout:       120 * time.Second,
	}
	if host, _, err := net.SplitHostPort(listen); err == nil && !isLoopbackHost(host) {
		log.Printf("warning: RTA_WEB_LISTEN=%s is not loopback; web sessions keep RTA passwords in memory", listen)
	}
	log.Printf("rta-web listening on %s static=%s", listen, static)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func isLoopbackHost(host string) bool {
	if host == "" || host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
