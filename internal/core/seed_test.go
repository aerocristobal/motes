package core

import (
	"testing"
)

func TestSelectSeeds_TagMatch(t *testing.T) {
	motes := []*Mote{
		{ID: "m1", Tags: []string{"oauth", "api"}},
		{ID: "m2", Tags: []string{"docker", "ci"}},
		{ID: "m3", Tags: []string{"oauth", "auth"}},
	}
	ss := NewSeedSelector(motes, nil, nil, nil)
	seeds := ss.SelectSeeds("oauth", nil)

	ids := make(map[string]bool)
	for _, s := range seeds {
		ids[s.ID] = true
	}
	if !ids["m1"] || !ids["m3"] {
		t.Errorf("expected m1 and m3, got %v", ids)
	}
	if ids["m2"] {
		t.Errorf("m2 should not match oauth")
	}
}

func TestSelectSeeds_MultipleKeywords(t *testing.T) {
	motes := []*Mote{
		{ID: "m1", Tags: []string{"oauth"}},
		{ID: "m2", Tags: []string{"api"}},
		{ID: "m3", Tags: []string{"oauth", "api"}},
	}
	ss := NewSeedSelector(motes, nil, nil, nil)
	seeds := ss.SelectSeeds("oauth api", nil)

	// m3 has both tags → ranked highest
	if len(seeds) == 0 {
		t.Fatal("expected seeds")
	}
	if seeds[0].ID != "m3" {
		t.Errorf("m3 (both tags) should rank first, got %s", seeds[0].ID)
	}
}

func TestSelectSeeds_TitleFallback(t *testing.T) {
	motes := []*Mote{
		{ID: "m1", Tags: []string{"ci"}, Title: "OAuth implementation plan"},
		{ID: "m2", Tags: []string{"docker"}, Title: "Docker setup"},
	}
	ss := NewSeedSelector(motes, nil, nil, nil)
	seeds := ss.SelectSeeds("oauth", nil)

	ids := make(map[string]bool)
	for _, s := range seeds {
		ids[s.ID] = true
	}
	if !ids["m1"] {
		t.Errorf("m1 should match via title fallback")
	}
	if ids["m2"] {
		t.Errorf("m2 should not match")
	}
}

func TestSelectSeeds_AmbientGitBranch(t *testing.T) {
	motes := []*Mote{
		{ID: "m1", Tags: []string{"oauth", "auth"}},
		{ID: "m2", Tags: []string{"docker"}},
	}
	signals := []SignalConfig{
		{Name: "git_branch", Type: "built_in"},
	}
	ss := NewSeedSelector(motes, nil, signals, nil)

	ambient := &AmbientContext{GitBranch: "feature/oauth-flow"}
	seeds := ss.SelectSeeds("", ambient)

	ids := make(map[string]bool)
	for _, s := range seeds {
		ids[s.ID] = true
	}
	if !ids["m1"] {
		t.Errorf("m1 should match via git branch 'feature/oauth-flow'")
	}
}

func TestSelectSeeds_NoMatches(t *testing.T) {
	motes := []*Mote{
		{ID: "m1", Tags: []string{"docker"}},
	}
	ss := NewSeedSelector(motes, nil, nil, nil)
	seeds := ss.SelectSeeds("nonexistent", nil)

	if len(seeds) != 0 {
		t.Errorf("expected 0 seeds, got %d", len(seeds))
	}
}

func TestSelectSeeds_CoAccessSignal(t *testing.T) {
	motes := []*Mote{
		{ID: "m1", Tags: []string{"oauth", "auth"}},
		{ID: "m2", Tags: []string{"database", "auth"}},
		{ID: "m3", Tags: []string{"docker"}},
	}
	signals := []SignalConfig{
		{Name: "git_branch", Type: "built_in"},
		{
			Name:        "auth_coaccess",
			Type:        "co_access",
			TriggerTags: []string{"oauth"},
			BoostTags:   []string{"auth"},
			BoostAmount: 0.5,
		},
	}
	ss := NewSeedSelector(motes, nil, signals, nil)
	// Search for "oauth" — m1 matches via tag, triggering co_access signal
	// which boosts all motes with "auth" tag (m1 and m2)
	seeds := ss.SelectSeeds("oauth", &AmbientContext{})

	ids := make(map[string]bool)
	for _, s := range seeds {
		ids[s.ID] = true
	}
	if !ids["m1"] {
		t.Error("m1 should match via tag + co_access boost")
	}
	if !ids["m2"] {
		t.Error("m2 should match via co_access boost (has auth tag)")
	}
	if ids["m3"] {
		t.Error("m3 should not match (no auth or oauth tags)")
	}
}

func TestSelectSeeds_BodyTextMatching(t *testing.T) {
	motes := []*Mote{
		{ID: "m1", Title: "Unrelated", Tags: []string{"other"}, Body: "This discusses scoring algorithms and retrieval."},
		{ID: "m2", Title: "Also unrelated", Tags: []string{"misc"}, Body: "Nothing relevant here."},
	}

	ss := NewSeedSelector(motes, nil, nil, nil)
	seeds := ss.SelectSeeds("scoring retrieval", nil)

	if len(seeds) == 0 {
		t.Fatal("expected body-text matching to find mote m1, got no seeds")
	}
	if seeds[0].ID != "m1" {
		t.Errorf("expected first seed to be m1, got %s", seeds[0].ID)
	}
}

func TestSelectSeeds_TagMatchHigherThanBodyMatch(t *testing.T) {
	motes := []*Mote{
		{ID: "body-only", Title: "Foo", Tags: []string{"unrelated"}, Body: "This talks about scoring in detail."},
		{ID: "tag-match", Title: "Bar", Tags: []string{"scoring"}, Body: "No relevant body content."},
	}

	ss := NewSeedSelector(motes, nil, nil, nil)
	seeds := ss.SelectSeeds("scoring", nil)

	if len(seeds) < 2 {
		t.Fatalf("expected 2 seeds, got %d", len(seeds))
	}
	// Tag match (1.0 + 0.3 body if body also matches) should outrank body-only (0.3)
	if seeds[0].ID != "tag-match" {
		t.Errorf("expected tag-match to rank first, got %s", seeds[0].ID)
	}
}

func TestSelectSeeds_BodyMatchAdditive(t *testing.T) {
	motes := []*Mote{
		{ID: "both", Title: "Scoring engine", Tags: []string{"scoring"}, Body: "The scoring engine computes relevance."},
		{ID: "tag-only", Title: "Scoring", Tags: []string{"scoring"}, Body: "No keywords here."},
	}

	ss := NewSeedSelector(motes, nil, nil, nil)
	seeds := ss.SelectSeeds("scoring", nil)

	if len(seeds) < 2 {
		t.Fatalf("expected 2 seeds, got %d", len(seeds))
	}
	// "both" has tag match (1.0) + body match (0.3 for "scoring") = 1.3
	// "tag-only" has tag match (1.0) only = 1.0
	if seeds[0].ID != "both" {
		t.Errorf("expected 'both' (tag+body) to rank first, got %s", seeds[0].ID)
	}
	if seeds[1].ID != "tag-only" {
		t.Errorf("expected 'tag-only' to rank second, got %s", seeds[1].ID)
	}
}

func TestKeywordOverlap(t *testing.T) {
	tests := []struct {
		query  []string
		target []string
		want   int
	}{
		{[]string{"foo", "bar"}, []string{"bar", "baz"}, 1},
		{[]string{"foo", "bar"}, []string{"foo", "bar"}, 2},
		{[]string{"foo"}, []string{"baz"}, 0},
		{nil, []string{"foo"}, 0},
		{[]string{"foo"}, nil, 0},
	}

	for _, tt := range tests {
		got := keywordOverlap(tt.query, tt.target)
		if got != tt.want {
			t.Errorf("keywordOverlap(%v, %v) = %d, want %d", tt.query, tt.target, got, tt.want)
		}
	}
}

// mockSearcher implements TextSearcher for testing.
type mockSearcher struct {
	results []TextSearchResult
}

func (ms *mockSearcher) Search(query string, topK int) []TextSearchResult {
	return ms.results
}

func TestBM25SeedBoost(t *testing.T) {
	// m1 has no tags, no title match — only discoverable via BM25
	motes := []*Mote{
		{ID: "m1", Title: "Unrelated title", Tags: []string{"other"}, Body: "Details about scoring algorithms."},
		{ID: "m2", Title: "Docker setup", Tags: []string{"docker"}, Body: "Container configuration."},
	}

	searcher := &mockSearcher{
		results: []TextSearchResult{
			{ID: "m1", Score: 5.0},
		},
	}

	ss := NewSeedSelector(motes, nil, nil, searcher)
	seeds := ss.SelectSeeds("scoring", nil)

	found := false
	for _, s := range seeds {
		if s.ID == "m1" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected BM25 to boost m1 into seeds")
	}
}

func TestBM25SeedBoost_NilSearcher(t *testing.T) {
	motes := []*Mote{
		{ID: "m1", Tags: []string{"scoring"}},
	}
	ss := NewSeedSelector(motes, nil, nil, nil)
	seeds := ss.SelectSeeds("scoring", nil)
	if len(seeds) == 0 {
		t.Error("nil searcher should not prevent tag-based seeds")
	}
}

func TestSelectSeeds_ConceptIndex(t *testing.T) {
	motes := []*Mote{
		{ID: "m1", Tags: []string{"other"}, Body: "Uses [[authentication]] for access."},
		{ID: "m2", Tags: []string{"docker"}},
	}
	conceptIndex := map[string][]string{
		"authentication": {"m1"},
	}

	ss := NewSeedSelector(motes, nil, nil, nil)
	ss.SetConceptIndex(conceptIndex)
	seeds := ss.SelectSeeds("authentication", nil)

	ids := make(map[string]bool)
	for _, s := range seeds {
		ids[s.ID] = true
	}
	if !ids["m1"] {
		t.Error("m1 should match via concept index for 'authentication'")
	}
	if ids["m2"] {
		t.Error("m2 should not match")
	}
}

func TestSelectSeeds_ConceptAdditiveWithTags(t *testing.T) {
	motes := []*Mote{
		{ID: "concept-only", Tags: []string{"other"}, Body: "Uses [[scoring]] internally."},
		{ID: "tag-only", Tags: []string{"scoring"}},
		{ID: "both", Tags: []string{"scoring"}, Body: "See [[scoring]] details."},
	}
	conceptIndex := map[string][]string{
		"scoring": {"concept-only", "both"},
	}

	ss := NewSeedSelector(motes, nil, nil, nil)
	ss.SetConceptIndex(conceptIndex)
	seeds := ss.SelectSeeds("scoring", nil)

	if len(seeds) < 3 {
		t.Fatalf("expected 3 seeds, got %d", len(seeds))
	}
	// "both" has tag (1.0) + concept (1.0) + body (0.3) = 2.3
	// "tag-only" has tag (1.0) = 1.0
	// "concept-only" has concept (1.0) + body (0.3) = 1.3
	if seeds[0].ID != "both" {
		t.Errorf("expected 'both' to rank first, got %s", seeds[0].ID)
	}
}

func TestExtractKeywords(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"feature/oauth-flow", []string{"feature", "oauth", "flow"}},
		{"the api is great", []string{"api", "great"}},
		{"OAuth OAuth oauth", []string{"oauth"}}, // dedup
		{"", nil},
	}
	for _, tc := range tests {
		result := ExtractKeywords(tc.input)
		if len(result) != len(tc.expected) {
			t.Errorf("ExtractKeywords(%q): got %v, want %v", tc.input, result, tc.expected)
			continue
		}
		for i := range result {
			if result[i] != tc.expected[i] {
				t.Errorf("ExtractKeywords(%q)[%d]: got %q, want %q",
					tc.input, i, result[i], tc.expected[i])
			}
		}
	}
}
