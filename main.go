//go:build linux

package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/VictoriaMetrics/metrics"
)

// Injected by -ldflags at build time.
var (
	version = "dev"
	commit  = "unknown"
	builtAt = "unknown"
)

func main() {
	listenAddr  := flag.String("listen", ":9700", "HTTP listen address")
	interval    := flag.Duration("interval", 10*time.Second, "stats collection interval")
	ifaces      := flag.String("interfaces", "", "comma-separated interface names (empty = all CAKE qdiscs)")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("qdisc-exporter %s (%s) built %s\n", version, commit, builtAt)
		os.Exit(0)
	}

	var filter map[string]struct{}
	if *ifaces != "" {
		filter = make(map[string]struct{})
		for _, s := range strings.Split(*ifaces, ",") {
			if s = strings.TrimSpace(s); s != "" {
				filter[s] = struct{}{}
			}
		}
	}

	c, err := newCollector(filter)
	if err != nil {
		log.Fatalf("init: %v", err)
	}
	defer c.close()

	if err := c.collect(); err != nil {
		log.Printf("initial collect: %v", err)
	}

	go func() {
		t := time.NewTicker(*interval)
		defer t.Stop()
		for range t.C {
			if err := c.collect(); err != nil {
				log.Printf("collect: %v", err)
			}
		}
	}()

	http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		metrics.WritePrometheus(w, false)
	})
	log.Printf("qdisc-exporter %s (%s) listening on %s", version, commit, *listenAddr)
	log.Fatal(http.ListenAndServe(*listenAddr, nil))
}
