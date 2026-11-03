package card

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func describe(node *SearchNode) string {
	switch node.Kind {
	case SearchAnd, SearchOr:
		separator := " and "
		if node.Kind == SearchOr {
			separator = " or "
		}
		parts := make([]string, len(node.Children))
		for i, child := range node.Children {
			parts[i] = describe(child)
		}
		return "(" + strings.Join(parts, separator) + ")"
	case SearchNot:
		return "not " + describe(node.Children[0])
	default:
		c := node.Term
		if c.Text != "" || c.Number == 0 {
			return fmt.Sprintf("%s%s%q", c.Field, c.Op, c.Text)
		}
		return fmt.Sprintf("%s%s%v", c.Field, c.Op, c.Number)
	}
}

func TestParseSearchQuery(t *testing.T) {
	// The other valid queries live in doc/search_cases.json.
	node, err := parseSearchQuery("  ")
	require.NoError(t, err)
	assert.Nil(t, node)
}

func TestParseSearchQuery_RejectsInvalidQueries(t *testing.T) {
	for _, tc := range []struct{ query, message string }{
		{"foo:bar", `q: unknown keyword "foo"`},
		{"t:", `q: "t:" needs a value`},
		{`o:"draw`, "q: missing closing quote"},
		{"(t:elf", "q: missing closing parenthesis"},
		{"t:elf)", `q: unexpected ")"`},
		{")", `q: unexpected ")"`},
		{"()", "q: empty parentheses"},
		{"or t:elf", `q: "or" must be between two terms`},
		{"t:elf or", `q: "or" must be between two terms`},
		{"(t:elf or)", `q: "or" must be between two terms`},
		{"t:elf or or c:g", `q: "or" must be between two terms`},
		{"-)", `q: unexpected ")"`},
		{"c:x", `q: "x" is not a color`},
		{"c:6", `q: "6" is not a color`},
		{"c:cg", `q: "cg" is not a color`},
		{"c<m", `q: c does not support "<"`},
		{"mv:abc", `q: "abc" is not a number`},
		{"mv<even", `q: mv does not support "<"`},
		{"pow:*", `q: "*" is not a number`},
		{"usd:NaN", `q: "NaN" is not a number`},
		{"m:2Q", `q: "2Q" is not a mana cost`},
		{"m:{", `q: "{" is not a mana cost`},
		{"m<=2W", `q: m does not support "<="`},
		{"r:epic", `q: "epic" is not a rarity (common, uncommon, rare, mythic)`},
		{"f:casual", `q: "casual" is not a format`},
		{"f>modern", `q: f does not support ">"`},
		{"t>elf", `q: t does not support ">"`},
		{"s<neo", `q: s does not support "<"`},
		{"cn>12a", `q: "12a" is not a number`},
		{"is:shiny", `q: "shiny" is not supported by is: (foil, nonfoil, proxy)`},
		{"is=foil", `q: is does not support "="`},
		{"not!=foil", `q: not does not support "!="`},
		{`!""`, `q: "!" needs a card name`},
		{strings.Repeat("a", 501), "q: must have at most 500 characters"},
		{strings.Repeat("a ", 51), "q: must have at most 50 terms"},
	} {
		t.Run(tc.query, func(t *testing.T) {
			_, err := parseSearchQuery(tc.query)
			require.Error(t, err)
			assert.True(t, errors.Is(err, errInvalidQuery))
			assert.Equal(t, tc.message, err.Error())
		})
	}
}

func TestSearchCondition_BindsEveryValue(t *testing.T) {
	node, err := parseSearchQuery(`o:"~ deals" -c:g m:2WW usd>1 f:modern`)
	require.NoError(t, err)

	var args []any
	sql := searchCondition(node, func(value any) string {
		args = append(args, value)
		return fmt.Sprintf("$%d", len(args))
	})

	assert.NotContains(t, sql, "deals")
	assert.NotContains(t, sql, "modern")
	assert.Contains(t, sql, "replace($1, '~', cards.name)")
	assert.Contains(t, sql, "NOT COALESCE(")
	assert.Equal(t, []any{"%~ deals%", "G", "{2}", 3, "{W}", 6, "usd", 1.0, "modern"}, args)
}

// neutralTree converts parser output to the neutral form of doc/search_cases.json.
func neutralTree(node *SearchNode) map[string]any {
	switch node.Kind {
	case SearchAnd, SearchOr:
		children := make([]any, len(node.Children))
		for i, child := range node.Children {
			children[i] = neutralTree(child)
		}
		if node.Kind == SearchOr {
			return map[string]any{"or": children}
		}
		return map[string]any{"and": children}
	case SearchNot:
		return map[string]any{"not": neutralTree(node.Children[0])}
	}
	c := node.Term
	var value any = c.Number
	// Colors are always text: an empty code means colorless.
	if c.Text != "" || c.Field == fieldColor || c.Field == fieldIdentity {
		value = c.Text
	}
	return map[string]any{"field": c.Field, "op": c.Op, "value": value}
}

func TestParseSearchQuery_SharedCases(t *testing.T) {
	raw, err := os.ReadFile("../../doc/search_cases.json")
	require.NoError(t, err)
	var cases []struct {
		Query   string          `json:"query"`
		Tree    json.RawMessage `json:"tree"`
		Invalid bool            `json:"invalid"`
		Sides   []string        `json:"sides"`
	}
	require.NoError(t, json.Unmarshal(raw, &cases))
	require.NotEmpty(t, cases)

	for _, tc := range cases {
		for _, side := range tc.Sides {
			require.Contains(t, []string{"api", "front"}, side, "case %q: unknown side", tc.Query)
		}
		require.NotEqual(t, tc.Invalid, len(tc.Tree) > 0, "case %q: needs exactly one of tree and invalid", tc.Query)
		if len(tc.Sides) > 0 && !slices.Contains(tc.Sides, "api") {
			continue
		}
		t.Run(tc.Query, func(t *testing.T) {
			node, err := parseSearchQuery(tc.Query)
			if tc.Invalid {
				assert.ErrorIs(t, err, errInvalidQuery)
				return
			}
			require.NoError(t, err)
			var want any
			require.NoError(t, json.Unmarshal(tc.Tree, &want))
			got, err := json.Marshal(neutralTree(node))
			require.NoError(t, err)
			var gotAny any
			require.NoError(t, json.Unmarshal(got, &gotAny))
			assert.Equal(t, want, gotAny)
		})
	}
}

func TestParseSearchQuery_HasNoKeywordForInternalCriteria(t *testing.T) {
	for _, query := range []string{"subtype:elf", "storagetype:deckbox", "storage:deckbox"} {
		_, err := parseSearchQuery(query)
		assert.ErrorIs(t, err, errInvalidQuery, query)
	}

	// A keyword is letters only, so this is a name search, not a criterion.
	node, err := parseSearchQuery("storage_type:deckbox")
	require.NoError(t, err)
	assert.Equal(t, `name="storage_type:deckbox"`, describe(node))
}
