package utils

import (
	"time"
)

var IsCN bool

func init() {
	IsCN = isInChina()
}

func isInChina() bool {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return false
	}

	_, offset := time.Now().Zone()
	_, chinaOffset := time.Now().In(loc).Zone()

	return offset == chinaOffset
}
