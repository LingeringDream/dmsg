package view

import (
	"testing"

	"dmsg/internal/ranking"
)

func TestDefaultViews(t *testing.T) {
	views := DefaultViews()
	if len(views) < 4 {
		t.Fatalf("expected >= 4 default views, got %d", len(views))
	}

	// Check named views exist
	names := make(map[string]bool)
	for _, v := range views {
		names[v.Name] = true
	}
	for _, expected := range []string{"All", "Following", "High Trust", "Trending", "Latest"} {
		if !names[expected] {
			t.Fatalf("missing default view: %s", expected)
		}
	}
}

func TestViewStrategies(t *testing.T) {
	views := DefaultViews()

	// All should use mixed
	allView := findView(views, "All")
	if allView.Strategy != ranking.StrategyMixed {
		t.Fatal("All view should use mixed strategy")
	}

	// Following should use time
	followView := findView(views, "Following")
	if followView.Strategy != ranking.StrategyTime {
		t.Fatal("Following view should use time strategy")
	}

	// Trending should use hot
	trendView := findView(views, "Trending")
	if trendView.Strategy != ranking.StrategyHot {
		t.Fatal("Trending view should use hot strategy")
	}
}

func TestViewTrustFiltering(t *testing.T) {
	views := DefaultViews()
	highTrust := findView(views, "High Trust")
	if highTrust.MinTrust < 0.5 {
		t.Fatal("High Trust view should have min_trust > 0.5")
	}
}

func findView(views []View, name string) *View {
	for _, v := range views {
		if v.Name == name {
			return &v
		}
	}
	return nil
}
