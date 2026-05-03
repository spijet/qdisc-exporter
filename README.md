# qdisc-exporter

A Prometheus exporter for Linux **CAKE** (`tc-cake`) qdisc statistics. Reads
per-tin metrics directly from the kernel via netlink (`RTM_GETQDISC`) and
exposes them at `/metrics` in Prometheus text format — no `tc` binary or
`iproute2` required at runtime.

---

## Requirements

- Linux kernel 4.19+ with `sch_cake` loaded
- Run as root **or** with `CAP_NET_ADMIN` (required to open `AF_NETLINK` sockets
  for rtnetlink)

---

## Building

```
make build          # produces ./qdisc-exporter (cross-compiles to linux/amd64)
make test           # run unit tests
```

Requires Go 1.21+. The binary is statically linked (`CGO_ENABLED=0`) and has
no runtime dependencies.

---

## Usage

```
qdisc-exporter [flags]

  -listen      string   HTTP listen address (default ":9700")
  -interval    duration stats collection interval (default 10s)
  -interfaces  string   comma-separated interface names to monitor
                        (default: all interfaces with a CAKE qdisc)
  -version              print version and exit
```

### Examples

```bash
# Monitor all CAKE qdiscs, expose on the default port
sudo qdisc-exporter

# Monitor only ppp0 and its ingress mirror ifb-wan
sudo qdisc-exporter -interfaces ppp0,ifb-wan

# Custom listen address and faster collection
sudo qdisc-exporter -listen :9701 -interval 5s
```

---

## Docker

The container must share the host network namespace to see the host's qdiscs:

```bash
docker run -d \
  --name qdisc-exporter \
  --network host \
  --restart unless-stopped \
  ghcr.io/spijet/qdisc-exporter
```

Pass extra flags after the image name:

```bash
docker run --network host ghcr.io/spijet/qdisc-exporter \
  --interfaces ppp0,ifb-wan --interval 5s
```

---

## Prometheus scrape config

```yaml
scrape_configs:
  - job_name: qdisc
    static_configs:
      - targets: ["localhost:9700"]
    scrape_interval: 15s
```

---

## Metrics

All metrics carry an `iface` label for the interface name. Per-tin metrics also
carry `tin` (0-indexed integer) and `tin_name` (human-readable name derived from
the CAKE diffserv mode, e.g. `Bulk`, `Best Effort`, `Video`, `Voice`).

### Per-interface

| Metric | Type | Description |
|--------|------|-------------|
| `cake_sent_bytes_total` | gauge | Total bytes transmitted |
| `cake_sent_packets_total` | gauge | Total packets transmitted |
| `cake_dropped_packets_total` | gauge | Total packets dropped |
| `cake_overlimit_packets_total` | gauge | Total packets that exceeded the rate limit |
| `cake_requeue_packets_total` | gauge | Total packets requeued |
| `cake_backlog_bytes` | gauge | Current queue backlog in bytes |
| `cake_backlog_packets` | gauge | Current queue backlog in packets |
| `cake_capacity_estimate_bps` | gauge | CAKE's estimated link capacity (bits/s) |
| `cake_memory_limit_bytes` | gauge | Configured memory limit |
| `cake_memory_used_bytes` | gauge | Current memory used by queued packets |

### Per-tin

| Metric | Type | Description |
|--------|------|-------------|
| `cake_tin_sent_bytes_total` | gauge | Bytes sent through this tin |
| `cake_tin_sent_packets_total` | gauge | Packets sent through this tin |
| `cake_tin_dropped_bytes_total` | gauge | Bytes of dropped packets |
| `cake_tin_dropped_packets_total` | gauge | Dropped packets |
| `cake_tin_ack_drop_bytes_total` | gauge | Bytes of ACK-filtered drops (egress) |
| `cake_tin_ack_drop_packets_total` | gauge | ACK-filtered drops (egress) |
| `cake_tin_ecn_mark_bytes_total` | gauge | Bytes of ECN-marked packets |
| `cake_tin_ecn_mark_packets_total` | gauge | ECN-marked packets |
| `cake_tin_backlog_bytes` | gauge | Current backlog bytes in this tin |
| `cake_tin_backlog_packets` | gauge | Current backlog packets in this tin |
| `cake_tin_threshold_rate_bps` | gauge | Configured bandwidth threshold (bits/s) |
| `cake_tin_target_us` | gauge | AQM target latency (µs) |
| `cake_tin_interval_us` | gauge | AQM interval (µs) |
| `cake_tin_peak_delay_us` | gauge | Peak sojourn delay observed (µs) |
| `cake_tin_avg_delay_us` | gauge | Average sojourn delay (µs) |
| `cake_tin_base_delay_us` | gauge | Sparse-flow base delay (µs) |
| `cake_tin_way_indirect_hits_total` | gauge | Set-associative hash indirect hits |
| `cake_tin_way_misses_total` | gauge | Set-associative hash misses |
| `cake_tin_way_collisions_total` | gauge | Set-associative hash collisions |
| `cake_tin_sparse_flows` | gauge | Active sparse flows |
| `cake_tin_bulk_flows` | gauge | Active bulk flows |
| `cake_tin_unresponsive_flows` | gauge | Unresponsive flows |
| `cake_tin_max_skb_len_bytes` | gauge | Largest packet seen in this tin (bytes) |
| `cake_tin_flow_quantum_bytes` | gauge | DRR++ quantum (bytes dequeued per round) |

### Tin names by diffserv mode

| Mode | Tins |
|------|------|
| `diffserv3` | Bulk · Best Effort · Voice |
| `diffserv4` | Bulk · Best Effort · Video · Voice |
| `diffserv8` | Bulk · CS1 · Best Effort · Video · Voice · CS5 · CS6 · CS7 |
| `besteffort` | Best Effort |
| `precedence` | CS0 · CS1 · CS2 · CS3 · CS4 · CS5 · CS6 · CS7 |

---

## Grafana dashboard

A ready-to-import dashboard is included as `grafana-dashboard.json`. Import it
via **Dashboards → Import → Upload JSON file**. It covers all metrics above,
organised into rows: Interface Overview, Per-Tin Traffic, AQM Delay, Flow
Isolation, Hash Table, Queue State, and Configuration.

---

## Disclaimer

This exporter is published under the MIT license. The author is not to be held
responsible if it doesn't work for your kernel version or qdisc setup.

## Disclaimer No. 2

Parts of this exporter are written with the help of neural networks and/or "AI"
coding agents. If that makes you uncomfortable — please avoid using.
