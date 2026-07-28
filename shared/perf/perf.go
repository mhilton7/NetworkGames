// Package perf provides bounded, allocation-free-on-observe telemetry for the
// Host, NBD server, synthetic backends, save overlay, and Pi controller.
package perf

import (
	"crypto/rand"
	"encoding/hex"
	"math"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"wiibridge/shared/contract"
)

var latencyBounds = [...]int64{
	100_000, 250_000, 500_000, 1_000_000, 2_000_000, 5_000_000,
	10_000_000, 25_000_000, 50_000_000, 100_000_000, 250_000_000,
	500_000_000,
}

type Histogram struct {
	buckets [len(latencyBounds) + 1]atomic.Uint64
	count   atomic.Uint64
	total   atomic.Uint64
	maximum atomic.Uint64
}

func (h *Histogram) Observe(duration time.Duration) {
	value := duration.Nanoseconds()
	if value < 0 {
		value = 0
	}
	index := sort.Search(len(latencyBounds), func(index int) bool {
		return value < latencyBounds[index]
	})
	h.buckets[index].Add(1)
	h.count.Add(1)
	h.total.Add(uint64(value))
	for {
		current := h.maximum.Load()
		if uint64(value) <= current || h.maximum.CompareAndSwap(current, uint64(value)) {
			break
		}
	}
}

type HistogramSnapshot struct {
	Count        uint64   `json:"count"`
	AverageUS    float64  `json:"average_us"`
	P95US        float64  `json:"p95_us"`
	P99US        float64  `json:"p99_us"`
	MaximumUS    float64  `json:"maximum_us"`
	BucketCounts []uint64 `json:"bucket_counts"`
}

func (h *Histogram) Snapshot() HistogramSnapshot {
	result := HistogramSnapshot{BucketCounts: make([]uint64, len(h.buckets))}
	for index := range h.buckets {
		result.BucketCounts[index] = h.buckets[index].Load()
	}
	result.Count = h.count.Load()
	total := h.total.Load()
	if result.Count != 0 {
		result.AverageUS = float64(total) / float64(result.Count) / 1000
	}
	result.P95US = percentile(result.BucketCounts, result.Count, 0.95) / 1000
	result.P99US = percentile(result.BucketCounts, result.Count, 0.99) / 1000
	result.MaximumUS = float64(h.maximum.Load()) / 1000
	return result
}

func percentile(buckets []uint64, count uint64, fraction float64) float64 {
	if count == 0 {
		return 0
	}
	target := uint64(math.Ceil(float64(count) * fraction))
	var seen uint64
	for index, value := range buckets {
		seen += value
		if seen >= target {
			if index >= len(latencyBounds) {
				return float64(latencyBounds[len(latencyBounds)-1])
			}
			return float64(latencyBounds[index])
		}
	}
	return 0
}

type rateBucket struct {
	second atomic.Int64
	bytes  atomic.Uint64
	ops    atomic.Uint64
}

type RollingRates struct {
	buckets [300]rateBucket
}

func (r *RollingRates) Add(now time.Time, bytes uint64) {
	second := now.Unix()
	bucket := &r.buckets[second%int64(len(r.buckets))]
	for {
		stamp := bucket.second.Load()
		if stamp == second {
			bucket.bytes.Add(bytes)
			bucket.ops.Add(1)
			return
		}
		if stamp < 0 {
			runtime.Gosched()
			continue
		}
		// A negative second marks the fixed bucket as being reset. Other
		// observers wait until both counters have been cleared before adding
		// to the new second, so rollover cannot discard a concurrent sample.
		if bucket.second.CompareAndSwap(stamp, -second) {
			bucket.bytes.Store(0)
			bucket.ops.Store(0)
			bucket.second.Store(second)
		}
	}
}

type RateSnapshot struct {
	OperationsPerSecond1m float64 `json:"operations_per_second_1m"`
	BytesPerSecond1m      float64 `json:"bytes_per_second_1m"`
	OperationsPerSecond5m float64 `json:"operations_per_second_5m"`
	BytesPerSecond5m      float64 `json:"bytes_per_second_5m"`
}

func (r *RollingRates) Snapshot(now time.Time) RateSnapshot {
	second := now.Unix()
	var ops1, bytes1, ops5, bytes5 uint64
	for index := range r.buckets {
		stamp := r.buckets[index].second.Load()
		age := second - stamp
		if age < 0 || age >= 300 {
			continue
		}
		ops := r.buckets[index].ops.Load()
		bytes := r.buckets[index].bytes.Load()
		ops5, bytes5 = ops5+ops, bytes5+bytes
		if age < 60 {
			ops1, bytes1 = ops1+ops, bytes1+bytes
		}
	}
	return RateSnapshot{
		OperationsPerSecond1m: float64(ops1) / 60,
		BytesPerSecond1m:      float64(bytes1) / 60,
		OperationsPerSecond5m: float64(ops5) / 300,
		BytesPerSecond5m:      float64(bytes5) / 300,
	}
}

type SourceMetrics struct {
	ReadOperations atomic.Uint64
	BytesRead      atomic.Uint64
	ReadErrors     atomic.Uint64
	OpenHandles    atomic.Int64
	CacheHits      atomic.Uint64
	CacheMisses    atomic.Uint64
	Evictions      atomic.Uint64
	IdentityErrors atomic.Uint64
	Latency        Histogram
	Rates          RollingRates
}

type DiskMetrics struct {
	MetadataReads  atomic.Uint64
	PayloadReads   atomic.Uint64
	SaveReads      atomic.Uint64
	SaveWrites     atomic.Uint64
	RejectedWrites atomic.Uint64
	ZeroReads      atomic.Uint64
	ExtentLookups  atomic.Uint64
	CrossExtent    atomic.Uint64
	CoalescedReads atomic.Uint64
	LookupLatency  Histogram
}

type NBDMetrics struct {
	ActiveConnections  atomic.Int64
	NegotiationFailure atomic.Uint64
	ReadRequests       atomic.Uint64
	WriteRequests      atomic.Uint64
	RejectedWrites     atomic.Uint64
	BytesSent          atomic.Uint64
	QueueDepth         atomic.Int64
	MaxQueueDepth      atomic.Int64
	Timeouts           atomic.Uint64
	Disconnects        atomic.Uint64
	Reconnects         atomic.Uint64
	TLSFailures        atomic.Uint64
	ProtocolErrors     atomic.Uint64
	RequestLatency     Histogram
	TLSLatency         Histogram
	Rates              RollingRates
}

type SaveMetrics struct {
	DirtyBlocks   atomic.Int64
	DirtyBytes    atomic.Int64
	JournalBytes  atomic.Int64
	FlushCount    atomic.Uint64
	FlushFailures atomic.Uint64
	RecoveryCount atomic.Uint64
	BackupCount   atomic.Uint64
	BackupFailure atomic.Uint64
	RestoreCount  atomic.Uint64
	RejectedWrite atomic.Uint64
	FlushLatency  Histogram
}

type LayerSnapshot struct {
	Counters   map[string]int64   `json:"counters"`
	Latency    HistogramSnapshot  `json:"latency"`
	TLSLatency *HistogramSnapshot `json:"tls_handshake_latency,omitempty"`
	Rates      RateSnapshot       `json:"rates"`
}

type RuntimeSnapshot struct {
	UptimeSeconds        int64   `json:"uptime_seconds"`
	HeapBytes            uint64  `json:"go_heap_bytes"`
	HeapTargetBytes      uint64  `json:"go_heap_target_bytes"`
	GCCount              uint32  `json:"gc_count"`
	GCPauseTotalNS       uint64  `json:"gc_pause_total_ns"`
	Goroutines           int     `json:"goroutines"`
	OpenFiles            int64   `json:"open_files"`
	ContainerMemoryLimit int64   `json:"container_memory_limit_bytes"`
	CPUUtilizationPct    float64 `json:"cpu_utilization_percent"`
	StartupPhase         string  `json:"startup_phase"`
	Readiness            string  `json:"readiness_state"`
}

type NetworkSnapshot struct {
	EstimatedBytesPerSecond1m float64 `json:"estimated_bytes_per_second_1m"`
	TCPRetransmittedSegments  uint64  `json:"tcp_retransmitted_segments_host_namespace"`
	Measurement               string  `json:"measurement"`
}

type Snapshot struct {
	Enabled   bool            `json:"enabled"`
	UpdatedAt time.Time       `json:"updated_at"`
	Source    LayerSnapshot   `json:"source"`
	Disk      LayerSnapshot   `json:"synthetic_disk"`
	NBD       LayerSnapshot   `json:"nbd"`
	Save      LayerSnapshot   `json:"save_overlay"`
	Runtime   RuntimeSnapshot `json:"runtime"`
	Network   NetworkSnapshot `json:"network"`
}

type PiSnapshot struct {
	UpdatedAt              time.Time `json:"updated_at"`
	Available              bool      `json:"available"`
	FirmwareUptimeSeconds  int64     `json:"firmware_uptime_seconds"`
	BoardModel             string    `json:"board_model"`
	CPUUtilizationPercent  float64   `json:"cpu_utilization_percent"`
	MemoryUsedBytes        int64     `json:"memory_used_bytes"`
	MemoryTotalBytes       int64     `json:"memory_total_bytes"`
	Load1                  float64   `json:"load_1"`
	Load5                  float64   `json:"load_5"`
	Load15                 float64   `json:"load_15"`
	TemperatureCelsius     float64   `json:"temperature_celsius"`
	NetworkInterface       string    `json:"network_interface"`
	LinkState              string    `json:"link_state"`
	WiFiSignalDBM          int       `json:"wifi_signal_dbm,omitempty"`
	NBDState               string    `json:"nbd_state"`
	NBDReconnectCount      uint64    `json:"nbd_reconnect_count"`
	NBDRequestsCompleted   uint64    `json:"nbd_requests_completed"`
	NBDReadFailures        uint64    `json:"nbd_read_failures"`
	NBDReadBytesPerSecond  float64   `json:"nbd_read_bytes_per_second"`
	USBState               string    `json:"usb_gadget_state"`
	USBAttachCount         uint64    `json:"usb_attach_count"`
	USBResetCount          uint64    `json:"usb_reset_count"`
	CurrentBlockDeviceSize int64     `json:"current_block_device_size"`
	ActiveProfile          string    `json:"current_active_profile"`
	RecentErrors           uint64    `json:"recent_error_count"`
	CollectionDurationUS   int64     `json:"collection_duration_us"`
}

type SessionSummary struct {
	ID                 string    `json:"session_id"`
	Start              time.Time `json:"start_time"`
	End                time.Time `json:"end_time,omitempty"`
	Platform           string    `json:"platform"`
	GameID             string    `json:"game_id,omitempty"`
	HostVersion        string    `json:"host_version"`
	FirmwareVersion    string    `json:"firmware_version,omitempty"`
	NegotiatedProtocol int       `json:"negotiated_protocol,omitempty"`
	TotalBytes         uint64    `json:"total_bytes"`
	ReadCount          uint64    `json:"read_count"`
	AverageLatencyUS   float64   `json:"average_latency_us"`
	P95LatencyUS       float64   `json:"p95_latency_us"`
	P99LatencyUS       float64   `json:"p99_latency_us"`
	MaximumLatencyUS   float64   `json:"maximum_latency_us"`
	SourceErrors       uint64    `json:"source_errors"`
	NBDDisconnects     uint64    `json:"nbd_disconnects"`
	USBResets          uint64    `json:"usb_resets"`
	SaveFlushes        uint64    `json:"save_flushes"`
	Outcome            string    `json:"final_outcome"`
}

type liveSession struct {
	summary      SessionSummary
	bytes        atomic.Uint64
	reads        atomic.Uint64
	sourceErrors atomic.Uint64
	disconnects  atomic.Uint64
	usbResets    atomic.Uint64
	saveFlushes  atomic.Uint64
	readLatency  Histogram
}

type Config struct {
	Enabled          bool
	SessionHistory   bool
	MaxSessions      int
	P99Warning       time.Duration
	MemoryWarningPct int
}

type Registry struct {
	Source SourceMetrics
	Disk   DiskMetrics
	NBD    NBDMetrics
	Save   SaveMetrics

	enabled             atomic.Bool
	started             time.Time
	config              Config
	current             atomic.Pointer[liveSession]
	persistenceFailures atomic.Uint64
	mu                  sync.RWMutex
	history             []SessionSummary
}

func New(config Config) *Registry {
	if config.MaxSessions < 1 {
		config.MaxSessions = 100
	}
	if config.MaxSessions > 1000 {
		config.MaxSessions = 1000
	}
	if config.P99Warning <= 0 {
		config.P99Warning = 100 * time.Millisecond
	}
	if config.MemoryWarningPct < 1 || config.MemoryWarningPct > 100 {
		config.MemoryWarningPct = 85
	}
	registry := &Registry{started: time.Now(), config: config}
	registry.enabled.Store(config.Enabled)
	return registry
}

func (r *Registry) Enabled() bool { return r != nil && r.enabled.Load() }
func (r *Registry) SetEnabled(enabled bool) {
	if r != nil {
		r.enabled.Store(enabled)
	}
}

func (r *Registry) SavePersistenceFailure() {
	if r != nil {
		r.persistenceFailures.Add(1)
	}
}

func (r *Registry) ObserveSourceRead(bytes int, duration time.Duration, err error) {
	if !r.Enabled() {
		return
	}
	r.Source.ReadOperations.Add(1)
	r.Source.BytesRead.Add(uint64(max(bytes, 0)))
	r.Source.Latency.Observe(duration)
	r.Source.Rates.Add(time.Now(), uint64(max(bytes, 0)))
	if err != nil {
		r.Source.ReadErrors.Add(1)
		if session := r.current.Load(); session != nil {
			session.sourceErrors.Add(1)
		}
	}
}

func (r *Registry) ObserveNBDRead(bytes int, duration time.Duration, err error) {
	if !r.Enabled() {
		return
	}
	r.NBD.ReadRequests.Add(1)
	if err == nil {
		r.NBD.BytesSent.Add(uint64(max(bytes, 0)))
		r.NBD.Rates.Add(time.Now(), uint64(max(bytes, 0)))
	}
	r.NBD.RequestLatency.Observe(duration)
	if session := r.current.Load(); session != nil {
		session.reads.Add(1)
		session.bytes.Add(uint64(max(bytes, 0)))
		session.readLatency.Observe(duration)
	}
}

func (r *Registry) RecordNBDDisconnect() {
	if !r.Enabled() {
		return
	}
	r.NBD.Disconnects.Add(1)
	if session := r.current.Load(); session != nil {
		session.disconnects.Add(1)
	}
}

func (r *Registry) RecordUSBReset() {
	r.RecordUSBResets(1)
}

func (r *Registry) RecordUSBResets(count uint64) {
	if !r.Enabled() {
		return
	}
	if session := r.current.Load(); session != nil {
		session.usbResets.Add(count)
	}
}

func (r *Registry) RecordSaveFlush() {
	if !r.Enabled() {
		return
	}
	if session := r.current.Load(); session != nil {
		session.saveFlushes.Add(1)
	}
}

func (r *Registry) StartSession(platform, gameID, hostVersion, firmwareVersion string,
	protocol int,
) SessionSummary {
	if !r.Enabled() {
		return SessionSummary{}
	}
	var random [8]byte
	_, _ = rand.Read(random[:])
	summary := SessionSummary{
		ID: hex.EncodeToString(random[:]), Start: time.Now().UTC(), Platform: platform,
		GameID: gameID, HostVersion: hostVersion, FirmwareVersion: firmwareVersion,
		NegotiatedProtocol: protocol, Outcome: "active",
	}
	r.current.Store(&liveSession{summary: summary})
	return summary
}

func (r *Registry) EndSession(outcome string) (SessionSummary, bool) {
	if r == nil {
		return SessionSummary{}, false
	}
	session := r.current.Swap(nil)
	if session == nil {
		return SessionSummary{}, false
	}
	result := session.summary
	result.End = time.Now().UTC()
	result.TotalBytes = session.bytes.Load()
	result.ReadCount = session.reads.Load()
	result.SourceErrors = session.sourceErrors.Load()
	result.NBDDisconnects = session.disconnects.Load()
	result.USBResets = session.usbResets.Load()
	result.SaveFlushes = session.saveFlushes.Load()
	latency := session.readLatency.Snapshot()
	result.AverageLatencyUS, result.P95LatencyUS = latency.AverageUS, latency.P95US
	result.P99LatencyUS, result.MaximumLatencyUS = latency.P99US, latency.MaximumUS
	result.Outcome = outcome
	if r.config.SessionHistory {
		r.mu.Lock()
		r.history = append(r.history, result)
		if len(r.history) > r.config.MaxSessions {
			r.history = append([]SessionSummary(nil),
				r.history[len(r.history)-r.config.MaxSessions:]...)
		}
		r.mu.Unlock()
	}
	return result, true
}

func (r *Registry) CurrentSession() (SessionSummary, bool) {
	if r == nil {
		return SessionSummary{}, false
	}
	session := r.current.Load()
	if session == nil {
		return SessionSummary{}, false
	}
	result := session.summary
	result.TotalBytes, result.ReadCount = session.bytes.Load(), session.reads.Load()
	result.SourceErrors = session.sourceErrors.Load()
	result.NBDDisconnects = session.disconnects.Load()
	result.USBResets = session.usbResets.Load()
	result.SaveFlushes = session.saveFlushes.Load()
	latency := session.readLatency.Snapshot()
	result.AverageLatencyUS, result.P95LatencyUS = latency.AverageUS, latency.P95US
	result.P99LatencyUS, result.MaximumLatencyUS = latency.P99US, latency.MaximumUS
	return result, true
}

func (r *Registry) Sessions(offset, limit int) []SessionSummary {
	if r == nil || limit <= 0 {
		return nil
	}
	if limit > 100 {
		limit = 100
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if offset < 0 {
		offset = 0
	}
	if offset >= len(r.history) {
		return nil
	}
	count := min(limit, len(r.history)-offset)
	result := make([]SessionSummary, count)
	for index := 0; index < count; index++ {
		result[index] = r.history[len(r.history)-1-offset-index]
	}
	return result
}

func (r *Registry) Session(id string) (SessionSummary, bool) {
	if r == nil || id == "" {
		return SessionSummary{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for index := len(r.history) - 1; index >= 0; index-- {
		if r.history[index].ID == id {
			return r.history[index], true
		}
	}
	return SessionSummary{}, false
}

func (r *Registry) ImportSessions(sessions []SessionSummary) {
	if r == nil || !r.config.SessionHistory {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for index := len(sessions) - 1; index >= 0; index-- {
		if sessions[index].ID != "" && !sessions[index].End.IsZero() {
			r.history = append(r.history, sessions[index])
		}
	}
	if len(r.history) > r.config.MaxSessions {
		r.history = append([]SessionSummary(nil),
			r.history[len(r.history)-r.config.MaxSessions:]...)
	}
}

func (r *Registry) Snapshot(phase, readiness string, openFiles, memoryLimit int64) Snapshot {
	now := time.Now().UTC()
	if r == nil {
		return Snapshot{UpdatedAt: now}
	}
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	tlsLatency := r.NBD.TLSLatency.Snapshot()
	return Snapshot{
		Enabled: r.Enabled(), UpdatedAt: now,
		Source: LayerSnapshot{
			Counters: map[string]int64{
				"read_operations": int64(r.Source.ReadOperations.Load()),
				"bytes_read":      int64(r.Source.BytesRead.Load()), "read_errors": int64(r.Source.ReadErrors.Load()),
				"open_handles": r.Source.OpenHandles.Load(), "cache_hits": int64(r.Source.CacheHits.Load()),
				"cache_misses": int64(r.Source.CacheMisses.Load()), "cache_evictions": int64(r.Source.Evictions.Load()),
				"identity_check_failures": int64(r.Source.IdentityErrors.Load()),
			},
			Latency: r.Source.Latency.Snapshot(), Rates: r.Source.Rates.Snapshot(now),
		},
		Disk: LayerSnapshot{
			Counters: map[string]int64{
				"metadata_reads": int64(r.Disk.MetadataReads.Load()), "payload_reads": int64(r.Disk.PayloadReads.Load()),
				"save_reads": int64(r.Disk.SaveReads.Load()), "save_writes": int64(r.Disk.SaveWrites.Load()),
				"rejected_writes": int64(r.Disk.RejectedWrites.Load()), "zero_padding_reads": int64(r.Disk.ZeroReads.Load()),
				"extent_lookups": int64(r.Disk.ExtentLookups.Load()), "cross_extent_reads": int64(r.Disk.CrossExtent.Load()),
				"coalesced_reads": int64(r.Disk.CoalescedReads.Load()),
			},
			Latency: r.Disk.LookupLatency.Snapshot(),
		},
		NBD: LayerSnapshot{
			Counters: map[string]int64{
				"active_connections":   r.NBD.ActiveConnections.Load(),
				"negotiation_failures": int64(r.NBD.NegotiationFailure.Load()),
				"read_requests":        int64(r.NBD.ReadRequests.Load()), "write_requests": int64(r.NBD.WriteRequests.Load()),
				"rejected_writes": int64(r.NBD.RejectedWrites.Load()), "bytes_sent": int64(r.NBD.BytesSent.Load()),
				"queue_depth": r.NBD.QueueDepth.Load(), "maximum_queue_depth": r.NBD.MaxQueueDepth.Load(),
				"timeouts": int64(r.NBD.Timeouts.Load()), "disconnects": int64(r.NBD.Disconnects.Load()),
				"reconnects": int64(r.NBD.Reconnects.Load()), "tls_failures": int64(r.NBD.TLSFailures.Load()),
				"protocol_errors": int64(r.NBD.ProtocolErrors.Load()),
			},
			Latency: r.NBD.RequestLatency.Snapshot(), TLSLatency: &tlsLatency,
			Rates: r.NBD.Rates.Snapshot(now),
		},
		Save: LayerSnapshot{
			Counters: map[string]int64{
				"dirty_blocks": r.Save.DirtyBlocks.Load(), "dirty_bytes": r.Save.DirtyBytes.Load(),
				"journal_bytes": r.Save.JournalBytes.Load(), "flush_count": int64(r.Save.FlushCount.Load()),
				"flush_failures": int64(r.Save.FlushFailures.Load()), "recovery_count": int64(r.Save.RecoveryCount.Load()),
				"backup_count": int64(r.Save.BackupCount.Load()), "backup_failures": int64(r.Save.BackupFailure.Load()),
				"restore_count": int64(r.Save.RestoreCount.Load()), "rejected_writes": int64(r.Save.RejectedWrite.Load()),
			},
			Latency: r.Save.FlushLatency.Snapshot(),
		},
		Runtime: RuntimeSnapshot{
			UptimeSeconds: int64(time.Since(r.started).Seconds()), HeapBytes: memory.HeapAlloc,
			HeapTargetBytes: memory.NextGC, GCCount: memory.NumGC,
			GCPauseTotalNS: memory.PauseTotalNs, Goroutines: runtime.NumGoroutine(),
			OpenFiles: openFiles, ContainerMemoryLimit: memoryLimit,
			CPUUtilizationPct: processCPUUtilization(r.started),
			StartupPhase:      phase, Readiness: readiness,
		},
		Network: NetworkSnapshot{
			EstimatedBytesPerSecond1m: r.NBD.Rates.Snapshot(now).BytesPerSecond1m,
			TCPRetransmittedSegments:  readTCPRetransmittedSegments(),
			Measurement:               "host namespace aggregate; not attributed to a specific Wii session",
		},
	}
}

func processCPUUtilization(started time.Time) float64 {
	data, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		return 0
	}
	closeName := strings.LastIndexByte(string(data), ')')
	if closeName < 0 {
		return 0
	}
	fields := strings.Fields(string(data[closeName+1:]))
	if len(fields) <= 12 {
		return 0
	}
	user, userErr := strconv.ParseUint(fields[11], 10, 64)
	system, systemErr := strconv.ParseUint(fields[12], 10, 64)
	elapsed := time.Since(started).Seconds()
	if userErr != nil || systemErr != nil || elapsed <= 0 {
		return 0
	}
	// Linux USER_HZ is 100 on WiiBridge's supported Host architectures. This
	// is an average process utilization, not an instantaneous CPU sample.
	return (float64(user+system) / 100) / elapsed * 100
}

func readTCPRetransmittedSegments() uint64 {
	data, err := os.ReadFile("/proc/net/snmp")
	if err != nil {
		return 0
	}
	lines := strings.Split(string(data), "\n")
	for index := 0; index+1 < len(lines); index++ {
		if !strings.HasPrefix(lines[index], "Tcp:") ||
			!strings.HasPrefix(lines[index+1], "Tcp:") {
			continue
		}
		names := strings.Fields(lines[index])
		values := strings.Fields(lines[index+1])
		for field := 1; field < len(names) && field < len(values); field++ {
			if names[field] == "RetransSegs" {
				value, _ := strconv.ParseUint(values[field], 10, 64)
				return value
			}
		}
	}
	return 0
}

func (r *Registry) Warnings(snapshot Snapshot) []contract.Error {
	var warnings []contract.Error
	if snapshot.NBD.Latency.P99US >= float64(r.config.P99Warning.Microseconds()) {
		warnings = append(warnings, contract.New(
			"PERF-LATENCY-HIGH", "nbd", contract.SeverityWarning,
			"Recent NBD P99 latency exceeds the configured warning threshold.", true))
	}
	if snapshot.NBD.Counters["reconnects"] >= 3 {
		warnings = append(warnings, contract.New(
			"PERF-NBD-RECONNECTS-HIGH", "nbd", contract.SeverityWarning,
			"Repeated NBD reconnects were observed.", true))
	}
	if snapshot.Source.Counters["cache_misses"] >= 10 &&
		snapshot.Source.Counters["cache_evictions"]*2 >=
			snapshot.Source.Counters["cache_misses"] {
		warnings = append(warnings, contract.New(
			"PERF-SOURCE-CACHE-THRASHING", "source", contract.SeverityWarning,
			"Source-handle evictions are high relative to cache misses.", true))
	}
	if snapshot.NBD.Counters["tls_failures"] > 0 {
		warnings = append(warnings, contract.New(
			"PERF-TLS-NEGOTIATION-FAILURES", "nbd", contract.SeverityWarning,
			"One or more NBD TLS handshakes failed.", true))
	}
	if snapshot.Save.Counters["journal_bytes"] > 48<<20 {
		warnings = append(warnings, contract.New(
			"PERF-SAVE-JOURNAL-NEAR-LIMIT", "save-overlay", contract.SeverityWarning,
			"The save journal is approaching its configured bound.", true))
	}
	if snapshot.Runtime.ContainerMemoryLimit > 0 &&
		snapshot.Runtime.HeapBytes*100 >= uint64(snapshot.Runtime.ContainerMemoryLimit)*uint64(r.config.MemoryWarningPct) {
		warnings = append(warnings, contract.New(
			"PERF-CONTAINER-MEMORY-HIGH", "host", contract.SeverityWarning,
			"Host heap use is approaching the container memory limit.", true))
	}
	if r.persistenceFailures.Load() > 0 {
		warnings = append(warnings, contract.New(
			"PERF-PERSISTENCE-FAILED", "performance", contract.SeverityWarning,
			"One or more bounded metrics summaries could not be persisted.", true))
	}
	return warnings
}
