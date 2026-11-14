package gaossync

import "context"

// Derived is what the QA tools would send to GAOS for one case (the planning half of caseItem).
type Derived struct {
	Epic      string   `json:"epic"`
	Module    string   `json:"module"`
	Features  []string `json:"features"`
	UserStory string   `json:"user_story"`
	Polarity  string   `json:"polarity"`
	Level     string   `json:"level"`
	Pri       string   `json:"pri"`
}

// Derive returns the planning fields the push derives for every sendable case of the product,
// keyed by case code. Read-only; used to compare against the decisions GAOS pushed back.
func (s *Syncer) Derive(ctx context.Context, productID int) (map[string]Derived, error) {
	cat, err := s.readCatalog(ctx, productID)
	if err != nil {
		return nil, err
	}
	out := map[string]Derived{}
	for _, c := range buildPlan(cat).cases {
		out[c.CaseID] = Derived{c.Epic, c.Module, c.Features, c.UserStory, c.Polarity, c.Level, c.Pri}
	}
	return out, nil
}
