package card

import (
	"errors"
	"fmt"
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
	for _, tc := range []struct{ query, want string }{
		{"elves", `name="elves"`},
		{"llanowar elves", `(name="llanowar" and name="elves")`},
		{`"llanowar elves"`, `name="llanowar elves"`},
		{`!"Sol Ring"`, `exact="Sol Ring"`},
		{"!Opt", `exact="Opt"`},
		{"name:ring", `name="ring"`},
		{"t:creature", `type="creature"`},
		{"TYPE=Elf", `type="Elf"`},
		{`o:"draw a card"`, `oracle="draw a card"`},
		{"oracle:~", `oracle="~"`},
		{"c:g", `color>="G"`},
		{"color=gw", `color="WG"`},
		{"c<=rakdos", `color<="BR"`},
		{"c:c", `color=""`},
		{"c:colorless", `color=""`},
		{"c:m", `color_count>=2`},
		{"c:2", `color_count=2`},
		{"c>1", `color_count>1`},
		{"id:esper", `identity<="WUB"`},
		{"ci>=g", `identity>="G"`},
		{"identity:multicolor", `identity_count>=2`},
		{"mv<=3", `mana_value<=3`},
		{"cmc:2.5", `mana_value=2.5`},
		{"manavalue:even", `mana_parity="even"`},
		{"mv=ODD", `mana_parity="odd"`},
		{"pow>=4", `power>=4`},
		{"tou<-1", `toughness<-1`},
		{"m:2WW", `mana_cost>="{2}{W}{W}"`},
		{"mana={w/u}{G}", `mana_cost="{W/U}{G}"`},
		{"m>{10}x", `mana_cost>"{10}{X}"`},
		{"r:rare", `rarity=3`},
		{"rarity>=u", `rarity>=2`},
		{"s:NEO", `set="neo"`},
		{"e:mh2", `set="mh2"`},
		{"cn:12a", `number="12a"`},
		{"number>100", `number>100`},
		{"f:Commander", `format="commander"`},
		{"legal:modern", `format="modern"`},
		{`kw:"first strike"`, `keyword="first strike"`},
		{"is:foil", `is="foil"`},
		{"not:proxy", `not is="proxy"`},
		{"usd>=1.5", `usd>=1.5`},
		{"eur<2", `eur<2`},
		{"-t:land", `not type="land"`},
		{"t!=land", `not type="land"`},
		{"c!=g", `not color="G"`},
		{"t:elf or t:goblin", `(type="elf" or type="goblin")`},
		{"t:elf OR t:goblin c:g", `(type="elf" or (type="goblin" and color>="G"))`},
		{"(t:elf or t:goblin) c:g", `((type="elf" or type="goblin") and color>="G")`},
		{"-(r:common or r:uncommon)", `not (rarity=1 or rarity=2)`},
		{"Ach! Hans, Run!", `(name="Ach!" and name="Hans," and name="Run!")`},
		{"  ", ""},
	} {
		t.Run(tc.query, func(t *testing.T) {
			node, err := parseSearchQuery(tc.query)
			require.NoError(t, err)
			if tc.want == "" {
				assert.Nil(t, node)
				return
			}
			assert.Equal(t, tc.want, describe(node))
		})
	}
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
