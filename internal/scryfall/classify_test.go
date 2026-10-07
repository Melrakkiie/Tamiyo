package scryfall

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPrimaryType(t *testing.T) {
	cases := map[string]string{
		"Legendary Creature — Elf Druid":         TypeCreature,
		"Artifact Creature — Golem":              TypeCreature,
		"Basic Land — Forest":                    TypeLand,
		"Artifact Land":                          TypeLand,
		"Legendary Planeswalker — Jace":          TypePlaneswalker,
		"Instant":                                TypeInstant,
		"Sorcery":                                TypeSorcery,
		"Artifact — Equipment":                   TypeArtifact,
		"Enchantment — Aura":                     TypeEnchantment,
		"Battle — Siege":                         TypeBattle,
		"Kindred Instant — Elf":                  TypeInstant,
		"Creature — Human Wizard // Land — Town": TypeCreature,
		"Instant // Land":                        TypeInstant,
		"Sorcery // Land":                        TypeSorcery,
		"Legendary Enchantment — Saga // Legendary Creature — Goblin": TypeEnchantment,
		"Land // Creature — Elemental":                                TypeLand,
		"Scheme":                                                      TypeOther,
	}
	for typeLine, want := range cases {
		assert.Equal(t, want, PrimaryType(typeLine), typeLine)
	}
}

func TestIsPrimaryType(t *testing.T) {
	assert.True(t, IsPrimaryType(TypeCreature))
	assert.True(t, IsPrimaryType(TypeOther))
	assert.False(t, IsPrimaryType("creature"))
	assert.False(t, IsPrimaryType("Tribal"))
}

func TestColorCode(t *testing.T) {
	assert.Equal(t, "", ColorCode(nil))
	assert.Equal(t, "G", ColorCode([]string{"G"}))
	assert.Equal(t, "WUBRG", ColorCode([]string{"G", "R", "B", "U", "W"}))
	assert.Equal(t, "UR", ColorCode([]string{"r", "U", "R"}))
	assert.Equal(t, "", ColorCode([]string{"C"}))
}
