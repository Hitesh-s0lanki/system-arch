package tokenbucket_test

import (
	tb "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/tokenbucket"
	"strconv"
	"testing"
	"time"
)

func BenchmarkTokenBucketDecision(b *testing.B) {
	for _, scenario := range []string{"allow", "reject", "new_identity_at_capacity"} {
		b.Run(scenario, func(b *testing.B) {
			now := time.Unix(100, 0)
			cfg := tb.Config{Limit: 1, Window: time.Hour, Capacity: 1, MaxKeys: 10000}
			l, err := tb.New(cfg, tb.WithClock(func() time.Time { return now }))
			if err != nil {
				b.Fatal(err)
			}
			key := "u"
			if scenario == "reject" {
				l.Allow(key)
			}
			if scenario == "new_identity_at_capacity" {
				for i := 0; i < cfg.MaxKeys; i++ {
					l.Allow(strconv.Itoa(i))
				}
				key = "new"
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if scenario == "allow" {
					now = now.Add(time.Hour)
				}
				l.Allow(key)
			}
		})
	}
}
