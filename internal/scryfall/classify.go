package scryfall

import "strings"

const (
	TypeLand         = "Land"
	TypeCreature     = "Creature"
	TypePlaneswalker = "Planeswalker"
	TypeBattle       = "Battle"
	TypeInstant      = "Instant"
	TypeSorcery      = "Sorcery"
	TypeArtifact     = "Artifact"
	TypeEnchantment  = "Enchantment"
	TypeOther        = "Other"
)

var typesByPrecedence = []string{
	TypeLand, TypeCreature, TypePlaneswalker, TypeBattle, TypeInstant, TypeSorcery, TypeArtifact, TypeEnchantment,
}

var colorOrder = "WUBRG"

func PrimaryType(typeLine string) string {
	if idx := strings.Index(typeLine, "//"); idx >= 0 {
		typeLine = typeLine[:idx]
	}
	types := typeLine
	if idx := strings.Index(typeLine, "—"); idx >= 0 {
		types = typeLine[:idx]
	}
	for _, t := range typesByPrecedence {
		if strings.Contains(types, t) {
			return t
		}
	}
	return TypeOther
}

func IsPrimaryType(t string) bool {
	if t == TypeOther {
		return true
	}
	for _, known := range typesByPrecedence {
		if t == known {
			return true
		}
	}
	return false
}

func ColorCode(colors []string) string {
	var b strings.Builder
	for _, c := range colorOrder {
		for _, color := range colors {
			if strings.EqualFold(color, string(c)) {
				b.WriteRune(c)
				break
			}
		}
	}
	return b.String()
}
