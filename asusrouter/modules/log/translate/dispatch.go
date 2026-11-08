package translate

// Program-based dispatch of log entries to their event translators.

import (
	"sync"

	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
)

const cacheSize = 4096

// A program with no patterns of its own; every message stays unknown
var fallback = NewARLogPatternSet(AREventUnknownUnknown)

var patterns = map[log.ARProgram]*ARLogPatternSet{
	log.ARProgramAhs:                  AhsPatterns,
	log.ARProgramAutoChannelDaemon:    AutoChannelDaemonPatterns,
	log.ARProgramAvahiDaemon:          AvahiDaemonPatterns,
	log.ARProgramBandSteeringDaemon:   BandSteeringDaemonPatterns,
	log.ARProgramCron:                 CronPatterns,
	log.ARProgramDhcpClient:           DhcpClientPatterns,
	log.ARProgramDiskMonitor:          DiskMonitorPatterns,
	log.ARProgramDnsmasqDhcp:          DnsmasqDhcpPatterns,
	log.ARProgramFtpServer:            DaemonPatterns,
	log.ARProgramHostapd:              HostapdPatterns,
	log.ARProgramHttpDaemon:           HttpDaemonPatterns,
	log.ARProgramInit:                 InitPatterns,
	log.ARProgramItunes:               DaemonPatterns,
	log.ARProgramKernel:               KernelPatterns,
	log.ARProgramMiniupnp:             MiniupnpPatterns,
	log.ARProgramModprobe:             ModprobePatterns,
	log.ARProgramNtp:                  NtpPatterns,
	log.ARProgramRcService:            RcServicePatterns,
	log.ARProgramRoamingAssistant:     RoamingAssistantPatterns,
	log.ARProgramSambaServer:          DaemonPatterns,
	log.ARProgramSyslog:               SyslogPatterns,
	log.ARProgramTimemachine:          DaemonPatterns,
	log.ARProgramWifiScheduler:        WifiSchedulerPatterns,
	log.ARProgramWirelessClientDaemon: WirelessClientDaemonPatterns,
}

// ResolveProgram mirrors `resolve_program`: the entry's typed program
// (UNKNOWN when tag-less).
func ResolveProgram(entry *log.ARLogEntry) log.ARProgram {
	if entry.Program == nil {
		return log.ARProgramUnknown
	}

	kind, _ := log.ParseProgram(entry.Program.Name)
	return kind
}

type contentKey struct {
	program log.ARProgram
	content string
}

var (
	contentMu    sync.Mutex
	contentCache = map[contentKey]map[log.AREventKey]any{}
)

// translateContent translates a message, cached: nothing else shapes the
// result (the cached map is never handed out, only copies of it).
func translateContent(program log.ARProgram, content string) map[log.AREventKey]any {
	key := contentKey{program, content}
	contentMu.Lock()
	cached, ok := contentCache[key]
	contentMu.Unlock()
	if ok {
		return cached
	}

	set, found := patterns[program]
	if !found {
		set = fallback
	}
	result := set.Translate(content)

	contentMu.Lock()
	if len(contentCache) >= cacheSize {
		contentCache = map[contentKey]map[log.AREventKey]any{}
	}
	contentCache[key] = result
	contentMu.Unlock()
	return result
}

// TranslateEntry mirrors `translate_entry`: translate one entry;
// unmapped programs fall back to unknown. program nil resolves it from
// the entry.
func TranslateEntry(entry *log.ARLogEntry, program *log.ARProgram) map[log.AREventKey]any {
	var kind log.ARProgram
	if program == nil {
		kind = ResolveProgram(entry)
	} else {
		kind = *program
	}

	cached := translateContent(kind, entry.ContentValue())
	out := make(map[log.AREventKey]any, len(cached))
	for k, v := range cached {
		out[k] = v
	}
	return out
}

// TranslateEntries mirrors `translate_entries`: translate every entry
// (unmapped ones marked explicitly unknown).
func TranslateEntries(entries []*log.ARLogEntry) []map[log.AREventKey]any {
	out := make([]map[log.AREventKey]any, 0, len(entries))
	for _, entry := range entries {
		out = append(out, TranslateEntry(entry, nil))
	}
	return out
}
