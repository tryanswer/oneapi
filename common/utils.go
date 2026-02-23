package common

import (
	"fmt"
	"github.com/songquanpeng/one-api/common/config"
)

func LogQuota(quota int64) string {
	if config.DisplayInCurrencyEnabled {
		return fmt.Sprintf("%s%.6f 额度", config.CurrencySymbol, float64(quota)/config.QuotaPerUnit)
	}
	return fmt.Sprintf("%d 点额度", quota)
}
