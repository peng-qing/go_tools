package mongo_driver

import (
	"fmt"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func bsonText(t *testing.T, value any) string {
	t.Helper()
	data, err := bson.MarshalExtJSON(value, false, false)
	if err != nil {
		t.Fatalf("marshal BSON failed: %v", err)
	}
	return string(data)
}

func TestFilterBuilder(t *testing.T) {
	elemFilter := Filter{{Key: "score", Value: Filter{{Key: "$gte", Value: 60}}}}
	andConditions := []Filter{
		{{Key: "status", Value: "active"}},
		{{Key: "age", Value: Filter{{Key: "$gte", Value: 18}}}},
	}
	orConditions := []Filter{
		{{Key: "status", Value: "active"}},
		{{Key: "status", Value: "pending"}},
	}
	norConditions := []Filter{
		{{Key: "status", Value: "disabled"}},
		{{Key: "deleted", Value: true}},
	}

	tests := []struct {
		name     string
		build    func() Filter
		expected Filter
	}{
		{
			name:     "empty",
			build:    func() Filter { return NewFilterBuilder().Build() },
			expected: Filter{},
		},
		{
			name:     "EQ",
			build:    func() Filter { return NewFilterBuilder().EQ("age", 18).Build() },
			expected: Filter{{Key: "age", Value: Filter{{Key: "$eq", Value: 18}}}},
		},
		{
			name:     "NE",
			build:    func() Filter { return NewFilterBuilder().NE("status", "deleted").Build() },
			expected: Filter{{Key: "status", Value: Filter{{Key: "$ne", Value: "deleted"}}}},
		},
		{
			name:     "GT",
			build:    func() Filter { return NewFilterBuilder().GT("age", 18).Build() },
			expected: Filter{{Key: "age", Value: Filter{{Key: "$gt", Value: 18}}}},
		},
		{
			name:     "GTE",
			build:    func() Filter { return NewFilterBuilder().GTE("age", 18).Build() },
			expected: Filter{{Key: "age", Value: Filter{{Key: "$gte", Value: 18}}}},
		},
		{
			name:     "LT",
			build:    func() Filter { return NewFilterBuilder().LT("age", 65).Build() },
			expected: Filter{{Key: "age", Value: Filter{{Key: "$lt", Value: 65}}}},
		},
		{
			name:     "LTE",
			build:    func() Filter { return NewFilterBuilder().LTE("age", 65).Build() },
			expected: Filter{{Key: "age", Value: Filter{{Key: "$lte", Value: 65}}}},
		},
		{
			name:     "CloseInterval",
			build:    func() Filter { return NewFilterBuilder().CloseInterval("age", 18, 65).Build() },
			expected: Filter{{Key: "age", Value: Filter{{Key: "$gte", Value: 18}, {Key: "$lte", Value: 65}}}},
		},
		{
			name:     "LeftCloseRightOpen",
			build:    func() Filter { return NewFilterBuilder().LeftCloseRightOpen("age", 18, 65).Build() },
			expected: Filter{{Key: "age", Value: Filter{{Key: "$gte", Value: 18}, {Key: "$lt", Value: 65}}}},
		},
		{
			name:     "LeftOpenRightOpen",
			build:    func() Filter { return NewFilterBuilder().LeftOpenRightOpen("age", 18, 65).Build() },
			expected: Filter{{Key: "age", Value: Filter{{Key: "$gt", Value: 18}, {Key: "$lt", Value: 65}}}},
		},
		{
			name:     "LeftOpenRightClose",
			build:    func() Filter { return NewFilterBuilder().LeftOpenRightClose("age", 18, 65).Build() },
			expected: Filter{{Key: "age", Value: Filter{{Key: "$gt", Value: 18}, {Key: "$lte", Value: 65}}}},
		},
		{
			name:     "IN",
			build:    func() Filter { return NewFilterBuilder().IN("status", []string{"active", "pending"}).Build() },
			expected: Filter{{Key: "status", Value: Filter{{Key: "$in", Value: []string{"active", "pending"}}}}},
		},
		{
			name:     "NIN",
			build:    func() Filter { return NewFilterBuilder().NIN("status", []string{"deleted", "disabled"}).Build() },
			expected: Filter{{Key: "status", Value: Filter{{Key: "$nin", Value: []string{"deleted", "disabled"}}}}},
		},
		{
			name:     "ALL",
			build:    func() Filter { return NewFilterBuilder().ALL("tags", []string{"go", "mongo"}).Build() },
			expected: Filter{{Key: "tags", Value: Filter{{Key: "$all", Value: []string{"go", "mongo"}}}}},
		},
		{
			name:     "SIZE",
			build:    func() Filter { return NewFilterBuilder().SIZE("tags", 2).Build() },
			expected: Filter{{Key: "tags", Value: Filter{{Key: "$size", Value: 2}}}},
		},
		{
			name:     "ElemMatch",
			build:    func() Filter { return NewFilterBuilder().ElemMatch("results", elemFilter).Build() },
			expected: Filter{{Key: "results", Value: Filter{{Key: "$elemMatch", Value: elemFilter}}}},
		},
		{
			name:     "Slice",
			build:    func() Filter { return NewFilterBuilder().Slice("comments", 10, 5).Build() },
			expected: Filter{{Key: "comments", Value: Filter{{Key: "$slice", Value: []int{10, 5}}}}},
		},
		{
			name:     "EXISTS",
			build:    func() Filter { return NewFilterBuilder().EXISTS("email", true).Build() },
			expected: Filter{{Key: "email", Value: Filter{{Key: "$exists", Value: true}}}},
		},
		{
			name:     "TYPE",
			build:    func() Filter { return NewFilterBuilder().TYPE("created_at", "date").Build() },
			expected: Filter{{Key: "created_at", Value: Filter{{Key: "$type", Value: "date"}}}},
		},
		{
			name:  "AND",
			build: func() Filter { return NewFilterBuilder().AND(andConditions...).Build() },
			expected: Filter{{Key: "$and", Value: A{
				Filter{{Key: "status", Value: "active"}},
				Filter{{Key: "age", Value: Filter{{Key: "$gte", Value: 18}}}},
			}}},
		},
		{
			name:  "OR",
			build: func() Filter { return NewFilterBuilder().OR(orConditions...).Build() },
			expected: Filter{{Key: "$or", Value: A{
				Filter{{Key: "status", Value: "active"}},
				Filter{{Key: "status", Value: "pending"}},
			}}},
		},
		{
			name:  "NOT",
			build: func() Filter { return NewFilterBuilder().NOT("age", Filter{{Key: "$gte", Value: 18}}).Build() },
			expected: Filter{{Key: "age", Value: Filter{
				{Key: "$not", Value: Filter{{Key: "$gte", Value: 18}}},
			}}},
		},
		{
			name:  "NOR",
			build: func() Filter { return NewFilterBuilder().NOR(norConditions...).Build() },
			expected: Filter{{Key: "$nor", Value: A{
				Filter{{Key: "status", Value: "disabled"}},
				Filter{{Key: "deleted", Value: true}},
			}}},
		},
		{
			name:     "AND_empty",
			build:    func() Filter { return NewFilterBuilder().AND().Build() },
			expected: Filter{},
		},
		{
			name:     "OR_empty",
			build:    func() Filter { return NewFilterBuilder().OR().Build() },
			expected: Filter{},
		},
		{
			name:     "NOT_empty",
			build:    func() Filter { return NewFilterBuilder().NOT("", Filter{}).Build() },
			expected: Filter{},
		},
		{
			name:     "NOR_empty",
			build:    func() Filter { return NewFilterBuilder().NOR().Build() },
			expected: Filter{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := tt.build()
			actualText := bsonText(t, actual)
			expectedText := bsonText(t, tt.expected)
			fmt.Printf("[%s]\n  actual:   %s\n  expected: %s\n", tt.name, actualText, expectedText)
			if actualText != expectedText {
				t.Errorf("BSON mismatch\nactual:   %s\nexpected: %s", actualText, expectedText)
			}
		})
	}
}
