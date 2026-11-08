// Package temperature mirrors asusrouter/modules/temperature/.
package temperature

// Temperature scaling for AsusRouter.
//
// Helpers for rescaling temperature values that some routers report
// orders of magnitude off, plus the one-time user notification.

import (
	"fmt"
	"log/slog"
	"strconv"
	"sync"

	"github.com/eduardpaul/routerctl/asusrouter/config"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
)

// Expected range of a temperature value
const (
	ExpectedDecimalPlaces  = 2
	ExpectedTemperatureMin = 10.0
	ExpectedTemperatureMax = 150.0
	MaxScaleCount          = 5
)

var temperatureWarnedLock sync.Mutex

// pyRound mirrors Python `round(value, places)` (correctly rounded on
// the exact binary value, like Go's formatting).
func pyRound(value float64, places int) float64 {
	r, _ := strconv.ParseFloat(strconv.FormatFloat(value, 'f', places, 64), 64)
	return r
}

// ScaleTemperature mirrors `scale_temperature`: scale temperature values
// to a range (nil values are dropped). Python's defaults are
// ExpectedTemperatureMin, ExpectedTemperatureMax and MaxScaleCount.
//
// This is a temporary solution for those routers with orders of
// magnitude lower (or higher) temperature values.
func ScaleTemperature[K comparable](temperature map[K]*float64, rangeMin, rangeMax float64, maxScaleCount int) (map[K]float64, bool) {
	scaledTemperature := map[K]float64{}
	scaled := false

	for key, temp := range temperature {
		if temp == nil {
			continue
		}

		scaledTemp := *temp

		count := 0
		for scaledTemp < rangeMin && count < maxScaleCount {
			scaledTemp *= 10
			count++
		}

		for scaledTemp > rangeMax && count < maxScaleCount {
			scaledTemp /= 10
			count++
		}

		if scaledTemp != *temp {
			scaled = true
		}
		scaledTemperature[key] = pyRound(scaledTemp, ExpectedDecimalPlaces)
	}

	return scaledTemperature, scaled
}

// notifier is the part of a config the warning uses.
type notifier interface {
	EnsureNotificationFlag(key config.ARConfigKeyBase)
	Get(key config.ARConfigKeyBase) any
	Set(key config.ARConfigKeyBase, value any) error
}

// WarnTemperatureScaled mirrors `warn_temperature_scaled`: issue a
// one-time warning when temperature values are rescaled.
func WarnTemperatureScaled(cfg notifier, variables map[string]any) {
	cfg.EnsureNotificationFlag(config.ARConfigKeyNotifiedOptimisticTemperature)
	temperatureWarnedLock.Lock()
	defer temperatureWarnedLock.Unlock()
	if notified, ok := cfg.Get(config.ARConfigKeyNotifiedOptimisticTemperature).(bool); ok && !notified {
		slog.Warn(fmt.Sprintf(
			"Temperature values were rescaled due to the issue with "+
				"the raw data. The original data is: "+
				"`%s` and the expected range is between %s and %s.",
			converters.PyStr(variables),
			converters.PyStr(ExpectedTemperatureMin),
			converters.PyStr(ExpectedTemperatureMax),
		))
		_ = cfg.Set(config.ARConfigKeyNotifiedOptimisticTemperature, true)
	}
}
