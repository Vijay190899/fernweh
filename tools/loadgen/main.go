// Command loadgen drives realistic mixed traffic at the gateway and reports
// latency percentiles, the "performs well under load" claim, measured.
//
//	go run ./tools/loadgen -target http://localhost:8080 -rps 50 -duration 30s
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

var queries = []string{
	"A beach weekend under €1,000 in March",
	"family hotel in Crete with kids club, 4 stars",
	"Städtetrip nach Wien im Oktober, ruhig und günstig",
	"romantic vineyard escape in Portugal",
	"ski chalet with sauna, max €400 per night",
	"quiet wellness retreat with thermal baths in May",
	"one week in Mallorca under 1400 euro with pool",
	"adventure hiking in the Azores in June",
	"luxury city break in Paris with rooftop bar",
	"cheap sunny island in January",
}

func main() {
	target := flag.String("target", "http://localhost:8080", "gateway base URL")
	rps := flag.Int("rps", 50, "requests per second")
	duration := flag.Duration("duration", 30*time.Second, "test duration")
	flag.Parse()

	client := &http.Client{Timeout: 10 * time.Second}
	interval := time.Second / time.Duration(*rps)
	deadline := time.Now().Add(*duration)

	var (
		mu        sync.Mutex
		latencies []time.Duration
		errs      atomic.Int64
		codes     sync.Map
		wg        sync.WaitGroup
	)

	fmt.Printf("loadgen: %d rps against %s for %s\n", *rps, *target, *duration)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for now := range ticker.C {
		if now.After(deadline) {
			break
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			q := queries[rand.Intn(len(queries))]
			body, _ := json.Marshal(map[string]string{
				"query":      q,
				"session_id": fmt.Sprintf("load_%d", rand.Intn(500)),
			})
			start := time.Now()
			resp, err := client.Post(*target+"/api/search", "application/json", bytes.NewReader(body))
			took := time.Since(start)
			if err != nil {
				errs.Add(1)
				return
			}
			resp.Body.Close()
			n, _ := codes.LoadOrStore(resp.StatusCode, new(atomic.Int64))
			n.(*atomic.Int64).Add(1)
			mu.Lock()
			latencies = append(latencies, took)
			mu.Unlock()
		}()
	}
	wg.Wait()

	if len(latencies) == 0 {
		fmt.Println("no successful requests, is the stack up?")
		os.Exit(1)
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	pct := func(p float64) time.Duration {
		return latencies[int(float64(len(latencies)-1)*p)]
	}

	fmt.Printf("\nrequests: %d   transport errors: %d\n", len(latencies), errs.Load())
	codes.Range(func(k, v any) bool {
		fmt.Printf("  HTTP %v: %d\n", k, v.(*atomic.Int64).Load())
		return true
	})
	fmt.Printf("latency  p50: %v   p95: %v   p99: %v   max: %v\n",
		pct(0.50).Round(time.Millisecond), pct(0.95).Round(time.Millisecond),
		pct(0.99).Round(time.Millisecond), latencies[len(latencies)-1].Round(time.Millisecond))
}
