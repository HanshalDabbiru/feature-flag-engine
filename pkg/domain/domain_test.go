package domain

import "testing"

func TestOperatorConstants(t *testing.T) {
	if EQUALS != "EQUALS" {
		t.Errorf("expected EQUALS to be \"EQUALS\", got %s", EQUALS)
	}
	if NOT_EQUALS != "NOT_EQUALS" {
		t.Errorf("expected NOT_EQUALS to be \"NOT_EQUALS\", got %s", NOT_EQUALS)
	}
	if CONTAINS != "CONTAINS" {
		t.Errorf("expected CONTAINS to be \"CONTAINS\", got %s", CONTAINS)
	}
	if STARTS_WITH != "STARTS_WITH" {
		t.Errorf("expected STARTS_WITH to be \"STARTS_WITH\", got %s", STARTS_WITH)
	}
}

func TestFeatureFlagConstruction(t *testing.T) {
	flag := FeatureFlag{
		Key:         "checkout-v2",
		Description: "New checkout flow",
		Enabled:     true,
		DefaultValue: false,
		Rules: []Rule{
			{
				Name:  "us-pro-users",
				Value: true,
				Predicates: []Predicate{
					{Attribute: "country", Operator: EQUALS, Values: []string{"US", "Canada"}},
					{Attribute: "plan", Operator: EQUALS, Values: []string{"pro"}},
				},
			},
		},
	}

	if flag.Key != "checkout-v2" {
		t.Errorf("expected key \"checkout-v2\", got %s", flag.Key)
	}
	if !flag.Enabled {
		t.Error("expected flag to be enabled")
	}
	if len(flag.Rules) != 1 {
		t.Errorf("expected 1 rule, got %d", len(flag.Rules))
	}
	if len(flag.Rules[0].Predicates) != 2 {
		t.Errorf("expected 2 predicates, got %d", len(flag.Rules[0].Predicates))
	}
}

func TestUserContext(t *testing.T) {
	ctx := UserContext{
		"country": "US",
		"plan":    "pro",
	}

	if ctx["country"] != "US" {
		t.Errorf("expected country \"US\", got %s", ctx["country"])
	}
	if ctx["plan"] != "pro" {
		t.Errorf("expected plan \"pro\", got %s", ctx["plan"])
	}
	if _, ok := ctx["missing"]; ok {
		t.Error("expected missing key to not exist")
	}
}