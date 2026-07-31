package enrich

import (
	"context"
	"testing"
)

func BenchmarkTemplateGenerate(b *testing.B) {
	l := sample()
	g := TemplateGenerator{}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := g.Generate(context.Background(), l); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkContentHash(b *testing.B) {
	l := sample()
	b.ReportAllocs()
	for b.Loop() {
		ContentHash(l)
	}
}
