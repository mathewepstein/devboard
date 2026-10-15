package github

import (
	"math"
	"time"
)

const BudgetShare = 0.6

// Stretch interval until cost*3600/interval <= BudgetShare*limit.
func StretchInterval(base time.Duration, cost, limit int) time.Duration {
	if cost <= 0 || limit <= 0 {
		return base
	}
	allowed := BudgetShare * float64(limit)
	projected := float64(cost) * 3600 / base.Seconds()
	if projected <= allowed {
		return base
	}
	secs := math.Ceil(float64(cost) * 3600 / allowed)
	return time.Duration(secs) * time.Second
}
