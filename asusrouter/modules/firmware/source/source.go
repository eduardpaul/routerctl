// Package source mirrors asusrouter/modules/firmware/source.py (split out
// of package firmware to break the identity import cycle).
package source

import (
	"context"

	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/firmware"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
)

// ARFirmwareSignature is the DPI (bwdpi) signature update state (`sig_*`).
type ARFirmwareSignature struct {
	Version *string `json:"version"`
	Update  *int    `json:"update"`
	Upgrade *int    `json:"upgrade"`
	Error   *int    `json:"error"`
	Flag    *int    `json:"flag"`
}

// ARFirmwareSync is the AiMesh config-sync firmware state (`cfg_*`).
type ARFirmwareSync struct {
	Check   *int `json:"check"`
	Upgrade *int `json:"upgrade"`
}

// ARFirmwareWeb is the firmware web-update state (`webs_state_*`).
type ARFirmwareWeb struct {
	Fetch         firmware.ARFirmwareWebFetch   `json:"fetch"`
	Upgrade       firmware.ARFirmwareWebUpgrade `json:"upgrade"`
	Error         firmware.ARFirmwareWebError   `json:"error"`
	Notify        firmware.ARFirmwareWebNotify  `json:"notify"`
	Level         *int                          `json:"level"`
	Available     *firmware.ARFirmware          `json:"available"`
	AvailableBeta *firmware.ARFirmware          `json:"available_beta"`
	Required      *firmware.ARFirmware          `json:"required"`
	State         bool                          `json:"state"`
	StateBeta     bool                          `json:"state_beta"`
	ReleaseNote   *string                       `json:"release_note"`
}

// NewARFirmwareWeb returns the Python defaults of `ARFirmwareWeb()`.
func NewARFirmwareWeb() ARFirmwareWeb {
	return ARFirmwareWeb{
		Fetch:   firmware.ARFirmwareWebFetchUnknown,
		Upgrade: firmware.ARFirmwareWebUpgradeUnknown,
		Error:   firmware.ARFirmwareWebErrorUnknown,
		Notify:  firmware.ARFirmwareWebNotifyUnknown,
	}
}

// ARFirmwareState is the firmware state of the router.
type ARFirmwareState struct {
	Current   *firmware.ARFirmware `json:"current"`
	Web       ARFirmwareWeb        `json:"web"`
	Signature ARFirmwareSignature  `json:"signature"`
	Sync      ARFirmwareSync       `json:"sync"`
}

// NewARFirmwareState returns the Python defaults of `ARFirmwareState()`.
func NewARFirmwareState() *ARFirmwareState {
	return &ARFirmwareState{Web: NewARFirmwareWeb()}
}

// ARFirmwareSource is the firmware data source for the connected router.
type ARFirmwareSource struct {
	source.Base
}

// SourceName returns the Python class name.
func (*ARFirmwareSource) SourceName() string { return "ARFirmwareSource" }

// ARFirmwareSourceUniversal is the universal instance - preferred.
var ARFirmwareSourceUniversal = &ARFirmwareSource{}

// Release-note endpoints, tried in order until one returns a note
var noteEndpoints = []endpoint.AREndpoint{
	endpoint.AREndpointFetchFirmwareUpdateNote,
	endpoint.AREndpointFetchFirmwareUpdateNoteAimesh,
}

// available parses an available-firmware string, or nil when unparsable.
func available(raw any) *firmware.ARFirmware {
	fw := firmware.ARFirmwareFromString(raw)
	if fw.Major() == nil {
		return nil
	}
	return fw
}

// isStableUpdate checks whether a newer stable firmware is available.
func isStableUpdate(current, avail *firmware.ARFirmware) bool {
	if avail == nil {
		return false
	}
	if current != nil && current.Major() != nil {
		return current.Less(avail)
	}
	return true
}

// fetchNote fetches the release note from the first endpoint that returns
// one.
func fetchNote(ctx context.Context, fetchRaw registry.FetchRawFunc) (*string, error) {
	for _, ep := range noteEndpoints {
		raw, err := fetchRaw(ctx, ep, "")
		if err != nil {
			return nil, err
		}
		var content *string
		if raw != nil {
			content = converters.RawToStr(*raw)
		}
		if content == nil || *content == "" {
			continue
		}
		if note := firmware.ReadFirmwareNote(*content); note != nil {
			return note, nil
		}
	}
	return nil, nil
}

// fetchState fetches the firmware state and release note when available.
func fetchState(ctx context.Context, c *registry.Context, src source.DataSource) (any, error) {
	raw, err := c.Read(ctx, endpoint.AREndpointFetchFirmwareUpdate, "")
	if err != nil {
		return nil, err
	}
	update, ok := raw.(map[string]any)
	if !ok {
		update = map[string]any{}
	}

	var note *string
	var current *firmware.ARFirmware
	if c.Identity != nil {
		current = c.Identity.Firmware()
	}
	avail := available(update["webs_state_info"])
	if c.FetchRaw != nil && isStableUpdate(current, avail) {
		note, err = fetchNote(ctx, c.FetchRaw)
		if err != nil {
			return nil, err
		}
	}

	return map[string]any{"update": update, "note": note, "available": avail}, nil
}

// translateState translates the fetched firmware state into an
// `ARFirmwareState`.
func translateState(raw any, tc *registry.TranslateContext) any {
	data, ok := raw.(map[string]any)
	if !ok {
		return NewARFirmwareState()
	}

	update, _ := data["update"].(map[string]any)
	if update == nil {
		update = map[string]any{}
	}
	var current *firmware.ARFirmware
	if tc != nil && tc.Identity != nil {
		current = tc.Identity.Firmware()
	}

	// Reuse the available firmware parsed by `fetch_state` when present
	avail, _ := data["available"].(*firmware.ARFirmware)
	if avail == nil {
		avail = available(update["webs_state_info"])
	}
	availBeta := available(update["webs_state_info_beta"])
	state := isStableUpdate(current, avail)

	web := ARFirmwareWeb{
		Fetch:         firmware.ARFirmwareWebFetchFromValue(update["webs_state_update"]),
		Upgrade:       firmware.ARFirmwareWebUpgradeFromValue(update["webs_state_upgrade"]),
		Error:         firmware.ARFirmwareWebErrorFromValue(update["webs_state_error"]),
		Notify:        firmware.ARFirmwareWebNotifyFromValue(update["webs_state_flag"]),
		Level:         converters.RawToInt(update["webs_state_level"]),
		AvailableBeta: availBeta,
		Required:      available(update["webs_state_REQinfo"]),
		State:         state,
		StateBeta:     availBeta != nil,
	}
	if state {
		web.Available = avail
	}
	if note, ok := data["note"].(*string); ok {
		web.ReleaseNote = note
	} else if note, ok := data["note"].(string); ok {
		web.ReleaseNote = &note
	}

	signature := ARFirmwareSignature{
		Version: converters.RawToStr(update["sig_ver"]),
		Update:  converters.RawToInt(update["sig_state_update"]),
		Upgrade: converters.RawToInt(update["sig_state_upgrade"]),
		Error:   converters.RawToInt(update["sig_state_error"]),
		Flag:    converters.RawToInt(update["sig_state_flag"]),
	}

	sync := ARFirmwareSync{
		Check:   converters.RawToInt(update["cfg_check"]),
		Upgrade: converters.RawToInt(update["cfg_upgrade"]),
	}

	return &ARFirmwareState{Current: current, Web: web, Signature: signature, Sync: sync}
}

func init() {
	registry.RegisterSource(registry.SourceEntry{
		Name:      "ARFirmwareSource",
		Default:   func() source.DataSource { return &ARFirmwareSource{} },
		Fetch:     fetchState,
		Translate: translateState,
	})
}
