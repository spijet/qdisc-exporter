//go:build linux

package main

import (
	"fmt"
	"strconv"

	"github.com/VictoriaMetrics/metrics"
)

type collector struct {
	filter map[string]struct{} // nil = accept all interfaces
}

func newCollector(filter map[string]struct{}) (*collector, error) {
	return &collector{filter: filter}, nil
}

func (c *collector) close() {}

func (c *collector) collect() error {
	qdiscs, err := getQdiscs()
	if err != nil {
		return err
	}
	for _, q := range qdiscs {
		if c.filter != nil {
			if _, ok := c.filter[q.IfName]; !ok {
				continue
			}
		}
		updateMetrics(q)
	}
	return nil
}

func updateMetrics(q QdiscStats) {
	iface := q.IfName

	g := func(name string) *metrics.Gauge {
		return metrics.GetOrCreateGauge(name, nil)
	}
	ifaceMetric := func(name string) *metrics.Gauge {
		return g(fmt.Sprintf(`%s{iface=%q}`, name, iface))
	}
	tinName := func(i int) string {
		if i < len(q.TinNames) && q.TinNames[i] != "" {
			return q.TinNames[i]
		}
		return fmt.Sprintf("tin%d", i)
	}
	tinMetric := func(name, tin, tname string) *metrics.Gauge {
		return g(fmt.Sprintf(`%s{iface=%q,tin=%q,tin_name=%q}`, name, iface, tin, tname))
	}

	ifaceMetric("cake_sent_bytes_total").Set(float64(q.SentBytes))
	ifaceMetric("cake_sent_packets_total").Set(float64(q.SentPackets))
	ifaceMetric("cake_dropped_packets_total").Set(float64(q.Drops))
	ifaceMetric("cake_overlimit_packets_total").Set(float64(q.Overlimits))
	ifaceMetric("cake_requeue_packets_total").Set(float64(q.Requeues))
	ifaceMetric("cake_backlog_bytes").Set(float64(q.Backlog))
	ifaceMetric("cake_backlog_packets").Set(float64(q.QueueLen))
	ifaceMetric("cake_capacity_estimate_bps").Set(float64(q.CapacityEstimate))
	ifaceMetric("cake_memory_limit_bytes").Set(float64(q.MemoryLimit))
	ifaceMetric("cake_memory_used_bytes").Set(float64(q.MemoryUsed))

	for i, tin := range q.Tins {
		t := strconv.Itoa(i)
		n := tinName(i)

		tinMetric("cake_tin_sent_bytes_total", t, n).Set(float64(tin.SentBytes))
		tinMetric("cake_tin_sent_packets_total", t, n).Set(float64(tin.SentPackets))
		tinMetric("cake_tin_dropped_bytes_total", t, n).Set(float64(tin.DroppedBytes))
		tinMetric("cake_tin_dropped_packets_total", t, n).Set(float64(tin.DroppedPackets))
		tinMetric("cake_tin_ack_drop_bytes_total", t, n).Set(float64(tin.AckDropBytes))
		tinMetric("cake_tin_ack_drop_packets_total", t, n).Set(float64(tin.AckDropPackets))
		tinMetric("cake_tin_ecn_mark_bytes_total", t, n).Set(float64(tin.EcnMarkBytes))
		tinMetric("cake_tin_ecn_mark_packets_total", t, n).Set(float64(tin.EcnMarkPackets))
		tinMetric("cake_tin_backlog_bytes", t, n).Set(float64(tin.BacklogBytes))
		tinMetric("cake_tin_backlog_packets", t, n).Set(float64(tin.BacklogPackets))
		tinMetric("cake_tin_threshold_rate_bps", t, n).Set(float64(tin.ThresholdRate))
		tinMetric("cake_tin_target_us", t, n).Set(float64(tin.TargetUs))
		tinMetric("cake_tin_interval_us", t, n).Set(float64(tin.IntervalUs))
		tinMetric("cake_tin_peak_delay_us", t, n).Set(float64(tin.PeakDelayUs))
		tinMetric("cake_tin_avg_delay_us", t, n).Set(float64(tin.AvgDelayUs))
		tinMetric("cake_tin_base_delay_us", t, n).Set(float64(tin.BaseDelayUs))
		tinMetric("cake_tin_way_indirect_hits_total", t, n).Set(float64(tin.WayIndirectHits))
		tinMetric("cake_tin_way_misses_total", t, n).Set(float64(tin.WayMisses))
		tinMetric("cake_tin_way_collisions_total", t, n).Set(float64(tin.WayCollisions))
		tinMetric("cake_tin_sparse_flows", t, n).Set(float64(tin.SparseFlows))
		tinMetric("cake_tin_bulk_flows", t, n).Set(float64(tin.BulkFlows))
		tinMetric("cake_tin_unresponsive_flows", t, n).Set(float64(tin.UnresponsiveFlows))
		tinMetric("cake_tin_max_skb_len_bytes", t, n).Set(float64(tin.MaxSkbLen))
		tinMetric("cake_tin_flow_quantum_bytes", t, n).Set(float64(tin.FlowQuantum))
	}
}
