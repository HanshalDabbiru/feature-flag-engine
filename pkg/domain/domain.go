package domain

// FeatureFlag is the top-level entity. When a client calls Evaluate, it walks
// the Rules in order and returns the first matching Rule's Value. If no rules
// match (or Enabled is false) it returns DefaultValue.
type FeatureFlag struct {
	Key          string
	Description  string
	Enabled      bool
	Rules        []Rule
	DefaultValue bool
}

// Rule groups a set of Predicates that must ALL match (AND logic). If every
// Predicate passes for a given UserContext, the rule matches and Value is returned.
type Rule struct {
	Name       string
	Predicates []Predicate
	Value      bool
}

// Predicate is a single condition: "does the user's Attribute satisfy Operator
// against any of the Values?" e.g. Attribute="country" Operator=EQUALS Values=["US","Canada"].
type Predicate struct {
	Attribute string
	Operator  Operator
	Values    []string
}

// Operator defines the comparison type used in a Predicate.
type Operator string

const (
	EQUALS      Operator = "EQUALS"
	NOT_EQUALS  Operator = "NOT_EQUALS"
	CONTAINS    Operator = "CONTAINS"
	STARTS_WITH Operator = "STARTS_WITH"
)

// UserContext holds the attributes of the user being evaluated, e.g.
// {"country": "US", "plan": "pro"}. Predicate.Attribute is looked up here.
type UserContext map[string]string
