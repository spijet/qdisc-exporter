//go:build linux

package main

import (
	"encoding/binary"
	"fmt"
	"net"
	"syscall"
)

// Linux TC netlink constants (stable ABI, not in stdlib syscall).
const (
	rtmNewQdisc = 36
	rtmGetQdisc = 38

	nlmFRequest = 0x01
	nlmFDump    = 0x300 // NLM_F_ROOT | NLM_F_MATCH

	// Top-level TCA attributes in RTM_NEWQDISC
	tcaKind    = 1
	tcaOptions = 2
	tcaStats2  = 7

	// Nested within TCA_OPTIONS for CAKE
	tcaCakeDiffServMode = 3

	// CAKE_DIFFSERV_* mode values
	cakeDiffServ3    = 0
	cakeDiffServ4    = 1
	cakeDiffServ8    = 2
	cakeBestEffort   = 3
	cakePrecedence   = 4

	// Nested within TCA_STATS2
	tcaStatsBasic = 1
	tcaStatsQueue = 3
	tcaStatsApp   = 4

	// Nested within TCA_STATS_APP (CAKE global)
	tcaCakeStatsCapacityEstimate64 = 2
	tcaCakeStatsMemoryLimit        = 3
	tcaCakeStatsMemoryUsed         = 4
	tcaCakeStatsTinStats           = 10

	// Nested within each tin entry
	tcaCakeTinStatsSentPackets         = 2
	tcaCakeTinStatsSentBytes64         = 3
	tcaCakeTinStatsDroppedPackets      = 4
	tcaCakeTinStatsDroppedBytes64      = 5
	tcaCakeTinStatsAcksDroppedPackets  = 6
	tcaCakeTinStatsAcksDroppedBytes64  = 7
	tcaCakeTinStatsEcnMarkedPackets    = 8
	tcaCakeTinStatsEcnMarkedBytes64    = 9
	tcaCakeTinStatsBacklogPackets      = 10
	tcaCakeTinStatsBacklogBytes        = 11
	tcaCakeTinStatsThresholdRate64     = 12
	tcaCakeTinStatsTargetUs            = 13
	tcaCakeTinStatsIntervalUs          = 14
	tcaCakeTinStatsWayIndirectHits     = 15
	tcaCakeTinStatsWayMisses           = 16
	tcaCakeTinStatsWayCollisions       = 17
	tcaCakeTinStatsPeakDelayUs         = 18
	tcaCakeTinStatsAvgDelayUs          = 19
	tcaCakeTinStatsBaseDelayUs         = 20
	tcaCakeTinStatsSparseFlows         = 21
	tcaCakeTinStatsBulkFlows           = 22
	tcaCakeTinStatsUnresponsiveFlows   = 23
	tcaCakeTinStatsMaxSkblen           = 24
	tcaCakeTinStatsFlowQuantum         = 25

	// NLA_F_NESTED may be set by some kernel versions; mask it when matching types.
	nlaFNested = 0x8000

	sizeofNlMsghdr = 16
	sizeofTcMsg    = 20
)

// cakeTinNames maps CAKE_DIFFSERV_* mode → ordered tin name list.
var cakeTinNames = map[uint32][]string{
	cakeDiffServ3:  {"Bulk", "Best Effort", "Voice"},
	cakeDiffServ4:  {"Bulk", "Best Effort", "Video", "Voice"},
	cakeDiffServ8:  {"Bulk", "CS1", "Best Effort", "Video", "Voice", "CS5", "CS6", "CS7"},
	cakeBestEffort: {"Best Effort"},
	cakePrecedence: {"CS0", "CS1", "CS2", "CS3", "CS4", "CS5", "CS6", "CS7"},
}

// QdiscStats holds all collected data for one CAKE qdisc instance.
type QdiscStats struct {
	IfName  string
	Handle  uint32
	// From TCA_STATS_BASIC
	SentBytes   uint64
	SentPackets uint32
	// From TCA_STATS_QUEUE
	QueueLen   uint32
	Backlog    uint32
	Drops      uint32
	Requeues   uint32
	Overlimits uint32
	// CAKE-global (from TCA_STATS_APP)
	CapacityEstimate uint64
	MemoryLimit      uint32
	MemoryUsed       uint32
	// Per-tin stats
	Tins     []CakeTinStats
	TinNames []string // same length as Tins; empty string if name unknown
}

// CakeTinStats holds per-tin statistics for a CAKE qdisc.
type CakeTinStats struct {
	SentPackets        uint32
	SentBytes          uint64
	DroppedPackets     uint32
	DroppedBytes       uint64
	AckDropPackets     uint32
	AckDropBytes       uint64
	EcnMarkPackets     uint32
	EcnMarkBytes       uint64
	BacklogPackets     uint32
	BacklogBytes       uint32
	ThresholdRate      uint64
	TargetUs           uint32
	IntervalUs         uint32
	WayIndirectHits    uint32
	WayMisses          uint32
	WayCollisions      uint32
	PeakDelayUs        uint32
	AvgDelayUs         uint32
	BaseDelayUs        uint32
	SparseFlows        uint32
	BulkFlows          uint32
	UnresponsiveFlows  uint32
	MaxSkbLen          uint32
	FlowQuantum        uint32
}

// getQdiscs fetches RTM_GETQDISC dump and returns stats for every CAKE qdisc found.
func getQdiscs() ([]QdiscStats, error) {
	fd, err := syscall.Socket(syscall.AF_NETLINK, syscall.SOCK_RAW|syscall.SOCK_CLOEXEC, syscall.NETLINK_ROUTE)
	if err != nil {
		return nil, fmt.Errorf("socket: %w", err)
	}
	defer syscall.Close(fd)

	lsa := &syscall.SockaddrNetlink{Family: syscall.AF_NETLINK}
	if err := syscall.Bind(fd, lsa); err != nil {
		return nil, fmt.Errorf("bind: %w", err)
	}
	if err := syscall.Sendto(fd, buildGetQdiscReq(), 0, lsa); err != nil {
		return nil, fmt.Errorf("sendto: %w", err)
	}

	var result []QdiscStats
	buf := make([]byte, 1<<16)
	for {
		n, _, err := syscall.Recvfrom(fd, buf, 0)
		if err != nil {
			return nil, fmt.Errorf("recvfrom: %w", err)
		}
		done, batch, err := parseNlMsgs(buf[:n])
		if err != nil {
			return nil, err
		}
		result = append(result, batch...)
		if done {
			return result, nil
		}
	}
}

func buildGetQdiscReq() []byte {
	const total = sizeofNlMsghdr + sizeofTcMsg
	buf := make([]byte, total)
	le := binary.LittleEndian
	le.PutUint32(buf[0:], total)
	le.PutUint16(buf[4:], rtmGetQdisc)
	le.PutUint16(buf[6:], nlmFRequest|nlmFDump)
	le.PutUint32(buf[8:], 1) // seq
	// tcmsg is all-zero → dump all qdiscs, all interfaces
	return buf
}

func parseNlMsgs(buf []byte) (done bool, qdiscs []QdiscStats, err error) {
	le := binary.LittleEndian
	for len(buf) >= sizeofNlMsghdr {
		msgLen := int(le.Uint32(buf[0:]))
		if msgLen < sizeofNlMsghdr || msgLen > len(buf) {
			return false, nil, fmt.Errorf("malformed nlmsg len %d", msgLen)
		}
		msgType := le.Uint16(buf[4:])
		payload := buf[sizeofNlMsghdr:msgLen]
		buf = buf[nlAlign(msgLen):]

		switch msgType {
		case syscall.NLMSG_DONE:
			return true, qdiscs, nil
		case syscall.NLMSG_ERROR:
			if len(payload) >= 4 {
				if errno := int32(le.Uint32(payload[0:])); errno != 0 {
					return false, nil, fmt.Errorf("nlmsg errno %d", -errno)
				}
			}
			return true, qdiscs, nil
		case rtmNewQdisc:
			q, ok, perr := parseTcMsg(payload)
			if perr != nil {
				return false, nil, perr
			}
			if ok {
				qdiscs = append(qdiscs, q)
			}
		}
	}
	return false, qdiscs, nil
}

func parseTcMsg(payload []byte) (QdiscStats, bool, error) {
	if len(payload) < sizeofTcMsg {
		return QdiscStats{}, false, nil
	}
	le := binary.LittleEndian
	ifindex := int(int32(le.Uint32(payload[4:])))
	handle := le.Uint32(payload[8:])
	attrs := payload[sizeofTcMsg:]

	var kind string
	var options, stats2 []byte
	forEachAttr(attrs, func(atype uint16, val []byte) {
		switch atype {
		case tcaKind:
			kind = nullStr(val)
		case tcaOptions:
			options = val
		case tcaStats2:
			stats2 = val
		}
	})

	if kind != "cake" {
		return QdiscStats{}, false, nil
	}

	ifName, _ := ifaceNameByIndex(ifindex)
	q := QdiscStats{IfName: ifName, Handle: handle}
	if options != nil {
		parseCakeOptions(options, &q)
	}
	if stats2 != nil {
		parseStats2(stats2, &q)
	}
	return q, true, nil
}

func parseCakeOptions(data []byte, q *QdiscStats) {
	le := binary.LittleEndian
	forEachAttr(data, func(atype uint16, val []byte) {
		if atype == tcaCakeDiffServMode && len(val) >= 4 {
			mode := le.Uint32(val)
			if names, ok := cakeTinNames[mode]; ok {
				q.TinNames = names
			}
		}
	})
}

func parseStats2(data []byte, q *QdiscStats) {
	le := binary.LittleEndian
	forEachAttr(data, func(atype uint16, val []byte) {
		switch atype {
		case tcaStatsBasic:
			if len(val) >= 12 {
				q.SentBytes = le.Uint64(val[0:])
				q.SentPackets = le.Uint32(val[8:])
			}
		case tcaStatsQueue:
			if len(val) >= 20 {
				q.QueueLen   = le.Uint32(val[0:])
				q.Backlog    = le.Uint32(val[4:])
				q.Drops      = le.Uint32(val[8:])
				q.Requeues   = le.Uint32(val[12:])
				q.Overlimits = le.Uint32(val[16:])
			}
		case tcaStatsApp:
			parseCakeStats(val, q)
		}
	})
}

func parseCakeStats(data []byte, q *QdiscStats) {
	le := binary.LittleEndian
	forEachAttr(data, func(atype uint16, val []byte) {
		switch atype {
		case tcaCakeStatsCapacityEstimate64:
			if len(val) >= 8 {
				q.CapacityEstimate = le.Uint64(val)
			}
		case tcaCakeStatsMemoryLimit:
			if len(val) >= 4 {
				q.MemoryLimit = le.Uint32(val)
			}
		case tcaCakeStatsMemoryUsed:
			if len(val) >= 4 {
				q.MemoryUsed = le.Uint32(val)
			}
		case tcaCakeStatsTinStats:
			// Each nested attr (index 1..n) is one tin's stats.
			forEachAttr(val, func(_ uint16, tinData []byte) {
				q.Tins = append(q.Tins, parseCakeTin(tinData))
			})
		}
	})
}

func parseCakeTin(data []byte) CakeTinStats {
	le := binary.LittleEndian
	var t CakeTinStats
	forEachAttr(data, func(atype uint16, val []byte) {
		u32 := func() uint32 {
			if len(val) >= 4 {
				return le.Uint32(val)
			}
			return 0
		}
		u64 := func() uint64 {
			if len(val) >= 8 {
				return le.Uint64(val)
			}
			return 0
		}
		switch atype {
		case tcaCakeTinStatsSentPackets:        t.SentPackets = u32()
		case tcaCakeTinStatsSentBytes64:        t.SentBytes = u64()
		case tcaCakeTinStatsDroppedPackets:     t.DroppedPackets = u32()
		case tcaCakeTinStatsDroppedBytes64:     t.DroppedBytes = u64()
		case tcaCakeTinStatsAcksDroppedPackets: t.AckDropPackets = u32()
		case tcaCakeTinStatsAcksDroppedBytes64: t.AckDropBytes = u64()
		case tcaCakeTinStatsEcnMarkedPackets:   t.EcnMarkPackets = u32()
		case tcaCakeTinStatsEcnMarkedBytes64:   t.EcnMarkBytes = u64()
		case tcaCakeTinStatsBacklogPackets:     t.BacklogPackets = u32()
		case tcaCakeTinStatsBacklogBytes:       t.BacklogBytes = u32()
		case tcaCakeTinStatsThresholdRate64:    t.ThresholdRate = u64()
		case tcaCakeTinStatsTargetUs:           t.TargetUs = u32()
		case tcaCakeTinStatsIntervalUs:         t.IntervalUs = u32()
		case tcaCakeTinStatsWayIndirectHits:    t.WayIndirectHits = u32()
		case tcaCakeTinStatsWayMisses:          t.WayMisses = u32()
		case tcaCakeTinStatsWayCollisions:      t.WayCollisions = u32()
		case tcaCakeTinStatsPeakDelayUs:        t.PeakDelayUs = u32()
		case tcaCakeTinStatsAvgDelayUs:         t.AvgDelayUs = u32()
		case tcaCakeTinStatsBaseDelayUs:        t.BaseDelayUs = u32()
		case tcaCakeTinStatsSparseFlows:        t.SparseFlows = u32()
		case tcaCakeTinStatsBulkFlows:          t.BulkFlows = u32()
		case tcaCakeTinStatsUnresponsiveFlows:  t.UnresponsiveFlows = u32()
		case tcaCakeTinStatsMaxSkblen:          t.MaxSkbLen = u32()
		case tcaCakeTinStatsFlowQuantum:        t.FlowQuantum = u32()
		}
	})
	return t
}

// forEachAttr iterates over a flat netlink attribute list, masking NLA_F_NESTED.
func forEachAttr(data []byte, fn func(atype uint16, val []byte)) {
	le := binary.LittleEndian
	for len(data) >= 4 {
		nlaLen := int(le.Uint16(data[0:]))
		if nlaLen < 4 || nlaLen > len(data) {
			break
		}
		atype := le.Uint16(data[2:]) &^ nlaFNested
		fn(atype, data[4:nlaLen])
		data = data[nlAlign(nlaLen):]
	}
}

// nlAlign rounds up to 4-byte alignment (NLMSG_ALIGN / NLA_ALIGN).
func nlAlign(n int) int { return (n + 3) &^ 3 }

func nullStr(b []byte) string {
	s := string(b)
	if len(s) > 0 && s[len(s)-1] == 0 {
		return s[:len(s)-1]
	}
	return s
}

var ifaceCache = map[int]string{}

func ifaceNameByIndex(ifindex int) (string, error) {
	if name, ok := ifaceCache[ifindex]; ok {
		return name, nil
	}
	iface, err := net.InterfaceByIndex(ifindex)
	if err != nil {
		return fmt.Sprintf("if%d", ifindex), err
	}
	ifaceCache[ifindex] = iface.Name
	return iface.Name, nil
}
