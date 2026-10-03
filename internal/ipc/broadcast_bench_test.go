package ipc

import "testing"

// The keystroke path calls Broadcast for every sound event, and the GUI is
// usually not running. Both orders were measured on the stand machine with
// `go test -run xxx -bench=Broadcast -benchtime=2000000x ./internal/ipc`
// (i7-4790K, Linux):
//
//	marshal before looking at the subscription set   240.7 ns/op   80 B/op   2 allocs/op
//	subscription count first                          12.56 ns/op    0 B/op   0 allocs/op
//
// The benchmark is also the regression guard: if Broadcast ever goes back to
// marshalling before checking whether anyone subscribed, this case costs about
// nineteen times more and the numbers above go stale in a visible way.
func BenchmarkBroadcastNoSubscribers(b *testing.B) {
	srv := NewServer("/tmp/bench-nonexistent.sock", nil, nil)
	ev := Event{Event: "sound", Name: "corrected"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		srv.Broadcast(ev)
	}
}
