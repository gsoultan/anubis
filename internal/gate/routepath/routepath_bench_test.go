package routepath

import "testing"

// The gate normalises and matches on every proxied request, inside a p99 < 1 ms
// budget that also has to cover token verification and the decision. These two
// must therefore be microseconds, not tens of them.
func BenchmarkNormalizePath(b *testing.B) {
	paths := []string{
		"/invoices/42", "/a//b/../c/", "/static/css/app.css",
		"/very/deep/path/with/several/segments/and/a/file.json",
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := NormalizePath(paths[i%len(paths)]); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMatch(b *testing.B) {
	routes := benchRoutes()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if r, _ := Match(routes, "app.example.com", "GET", "/invoices/42"); r == nil {
			b.Fatal("no route matched")
		}
	}
}
