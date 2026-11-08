package common

import (
	"log/slog"
	"strings"
	"sync"

	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
)

// Territory codes already reported as unknown, to warn only once each
var (
	reportedUnknownRegions = map[string]struct{}{}
	unknownRegionsLock     sync.Mutex
)

// warnUnknownRegion warns once per distinct unknown territory code.
func warnUnknownRegion(tcode string) {
	unknownRegionsLock.Lock()
	defer unknownRegionsLock.Unlock()
	if _, ok := reportedUnknownRegions[tcode]; ok {
		return
	}
	reportedUnknownRegions[tcode] = struct{}{}
	slog.Warn("We found an unknown region. Please, report this value: `" + tcode + "`")
}

// TranslateRegion translates a territory code like `US/01` into region and
// area.
func TranslateRegion(tcode any) (ARRegion, *int) {
	s, ok := tcode.(string)
	if !ok || s == "" {
		return ARRegionUnknown, nil
	}

	code, area, _ := strings.Cut(s, "/")
	region := ARRegionFromValue(code)
	if region == ARRegionUnknown {
		warnUnknownRegion(s)
	}

	return region, converters.RawToInt(area)
}
