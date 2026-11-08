package credentials

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"

	"github.com/eduardpaul/routerctl/asusrouter/consts"
	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/tools/writers"
)

// md5Hex mirrors `_md5_hex`: the lowercase MD5 hex digest the WebUI
// sends for creds.
func md5Hex(value string) string {
	// Not a security hash; the wire protocol expects this digest
	sum := md5.Sum([]byte(value))
	return hex.EncodeToString(sum[:])
}

// b64 mirrors `_b64`: the base64 form the WebUI sends for new
// credential values.
func b64(value string) string {
	return base64.StdEncoding.EncodeToString([]byte(value))
}

// BuildChpassRequest mirrors `build_chpass_request`: the CHPASS request
// body for a credential change. Python raises ValueError when neither
// new value is given; Go returns an error.
func BuildChpassRequest(
	curUsername string,
	curPassword string,
	newUsername *string,
	newPassword *string,
) (string, error) {
	if newUsername == nil && newPassword == nil {
		return "", errors.New(
			"at least one of new_username or new_password is required",
		)
	}

	data := []writers.KeyValue{
		{Key: "cur_username", Value: md5Hex(curUsername)},
		{Key: "cur_passwd", Value: md5Hex(curPassword)},
		{Key: string(common.ARServiceWebuiRestart), Value: "1"},
	}
	if newUsername != nil {
		data = append(data, writers.KeyValue{Key: "new_username", Value: b64(*newUsername)})
	}
	if newPassword != nil {
		data = append(data, writers.KeyValue{Key: "new_passwd", Value: b64(*newPassword)})
	}

	return writers.DictToRequest(data, consts.RequestTypeGet), nil
}

// ReadChpassResult mirrors `read_chpass_result`: read the CHPASS
// response into a status.
func ReadChpassResult(raw any) ARCredentialsStatus {
	data := raw
	switch v := raw.(type) {
	case *string:
		if v == nil {
			return ARCredentialsStatusUnknown
		}
		return ReadChpassResult(*v)
	case string:
		parsed, ok := jsonLoads(v)
		if !ok {
			return ARCredentialsStatusUnknown
		}
		data = parsed
	}
	m, ok := data.(map[string]any)
	if !ok {
		return ARCredentialsStatusUnknown
	}
	return ARCredentialsStatusFromValue(m[common.StatusCodeKey])
}

// jsonLoads mirrors Python `json.loads` (integral literals become int64).
func jsonLoads(text string) (any, bool) {
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, false
	}
	// Trailing data is an error in Python
	if _, err := dec.Token(); err != io.EOF {
		return nil, false
	}
	return pyNumbers(v), true
}

func pyNumbers(v any) any {
	switch t := v.(type) {
	case json.Number:
		if !strings.ContainsAny(string(t), ".eE") {
			if i, err := strconv.ParseInt(string(t), 10, 64); err == nil {
				return i
			}
		}
		f, _ := t.Float64()
		return f
	case map[string]any:
		for k, item := range t {
			t[k] = pyNumbers(item)
		}
	case []any:
		for i, item := range t {
			t[i] = pyNumbers(item)
		}
	}
	return v
}
