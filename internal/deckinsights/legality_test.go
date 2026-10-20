package deckinsights

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var (
	thrasios = Printing{
		Name:     "Thrasios, Triton Hero",
		TypeLine: "Legendary Creature — Merfolk Wizard",
		OracleText: "{4}: Scry 1, then reveal the top card of your library. If it's a land card, put it onto the battlefield tapped. Otherwise, draw a card.\n" +
			"Partner (You can have two commanders if both have partner.)",
		ColorIdentity: []string{"G", "U"},
	}
	tymna = Printing{
		Name:     "Tymna the Weaver",
		TypeLine: "Legendary Creature — Human Cleric",
		OracleText: "Lifelink\n" +
			"At the beginning of your second main phase, you may pay X life, where X is the number of opponents that were dealt combat damage this turn. If you do, draw X cards.\n" +
			"Partner (You can have two commanders if both have partner.)",
		ColorIdentity: []string{"W", "B"},
	}
	pir = Printing{
		Name:     "Pir, Imaginative Rascal",
		TypeLine: "Legendary Creature — Human",
		OracleText: "Partner with Toothy, Imaginary Friend (When this creature enters, target player may put Toothy into their hand from their library, then shuffle.)\n" +
			"If one or more counters would be put on a permanent your team controls, that many plus one of each of those kinds of counters are put on that permanent instead.",
		ColorIdentity: []string{"G"},
	}
	toothy = Printing{
		Name:     "Toothy, Imaginary Friend",
		TypeLine: "Legendary Creature — Illusion",
		OracleText: "Partner with Pir, Imaginative Rascal (When this creature enters, target player may put Pir into their hand from their library, then shuffle.)\n" +
			"Whenever you draw a card, put a +1/+1 counter on Toothy.\n" +
			"When Toothy leaves the battlefield, draw a card for each +1/+1 counter on it.",
		ColorIdentity: []string{"U"},
	}
	will = Printing{
		Name:     "Will Kenrith",
		TypeLine: "Legendary Planeswalker — Will",
		OracleText: "+2: Until your next turn, up to two target creatures each have base power and toughness 0/3 and lose all abilities.\n" +
			"−2: Target player draws two cards. Until end of turn, whenever that player casts an instant or sorcery spell, copy it and they may choose new targets for the copy.\n" +
			"Partner with Rowan Kenrith (When this enters, target player may put Rowan into their hand from their library, then shuffle.)\n" +
			"Will Kenrith can be your commander.",
		ColorIdentity: []string{"U"},
	}
	rowan = Printing{
		Name:     "Rowan Kenrith",
		TypeLine: "Legendary Planeswalker — Rowan",
		OracleText: "+2: During target player's next turn, each creature that player controls attacks if able.\n" +
			"−2: Rowan Kenrith deals 3 damage to each tapped creature target player controls.\n" +
			"Partner with Will Kenrith (When this enters, target player may put Will into their hand from their library, then shuffle.)\n" +
			"Rowan Kenrith can be your commander.",
		ColorIdentity: []string{"R"},
	}
	wilson = Printing{
		Name:     "Wilson, Refined Grizzly",
		TypeLine: "Legendary Creature — Bear Warrior",
		OracleText: "Choose a Background (You can have a Background as a second commander.)\n" +
			"Trample, ward {2}",
		ColorIdentity: []string{"G"},
	}
	raisedByGiants = Printing{
		Name:          "Raised by Giants",
		TypeLine:      "Legendary Enchantment — Background",
		OracleText:    "Commander creatures you own have base power and toughness 10/10 and are Giants in addition to their other types.",
		ColorIdentity: []string{"G"},
	}
	clara = Printing{
		Name:     "Clara Oswald",
		TypeLine: "Legendary Creature — Human Advisor",
		OracleText: "If a triggered ability of a Doctor you control triggers, that ability triggers an additional time.\n" +
			"Doctor’s companion (You can have two commanders if the other is the Doctor.)",
		ColorIdentity: []string{"U"},
	}
	eleventhDoctor = Printing{
		Name:          "The Eleventh Doctor",
		TypeLine:      "Legendary Creature — Time Lord Doctor",
		OracleText:    "Whenever The Eleventh Doctor deals combat damage to a player, you may exile a card from your hand with a mana value of 3 or less.",
		ColorIdentity: []string{"U", "R"},
	}
	eleven = Printing{
		Name:          "Eleven, the Mage",
		TypeLine:      "Legendary Creature — Human Wizard",
		OracleText:    "Friends forever (You can have two commanders if both have friends forever.)",
		ColorIdentity: []string{"U"},
	}
	mike = Printing{
		Name:          "Mike, the Dungeon Master",
		TypeLine:      "Legendary Creature — Human Wizard",
		OracleText:    "Friends forever (You can have two commanders if both have friends forever.)",
		ColorIdentity: []string{"W"},
	}
	firstSurvivor = Printing{
		Name:          "First Survivor",
		TypeLine:      "Legendary Creature — Human Survivor",
		OracleText:    "Partner—Survivors (You can have two commanders if both have this ability.)",
		ColorIdentity: []string{"G"},
	}
	secondSurvivor = Printing{
		Name:          "Second Survivor",
		TypeLine:      "Legendary Creature — Human Survivor",
		OracleText:    "Partner—Survivors (You can have two commanders if both have this ability.)",
		ColorIdentity: []string{"B"},
	}
	father = Printing{
		Name:          "A Father",
		TypeLine:      "Legendary Creature — God Warrior",
		OracleText:    "Partner—Father & son (You can have two commanders if both have this ability.)",
		ColorIdentity: []string{"R"},
	}
	golos = Printing{
		Name:          "Golos, Tireless Pilgrim",
		TypeLine:      "Legendary Artifact Creature — Scout",
		OracleText:    "When Golos enters, you may search your library for a land card, put that card onto the battlefield tapped, then shuffle.",
		ColorIdentity: []string{},
	}
)

func TestCanPair(t *testing.T) {
	tests := []struct {
		name          string
		first, second Printing
		want          bool
	}{
		{"two partners", thrasios, tymna, true},
		{"partner with each other", pir, toothy, true},
		{"partner with each other, planeswalkers", rowan, will, true},
		{"choose a background and a background", wilson, raisedByGiants, true},
		{"a background and choose a background", raisedByGiants, wilson, true},
		{"doctor's companion and a doctor", clara, eleventhDoctor, true},
		{"a doctor and doctor's companion", eleventhDoctor, clara, true},
		{"friends forever", eleven, mike, true},
		{"the same partner quality", firstSurvivor, secondSurvivor, true},
		{"partner and partner with", thrasios, pir, false},
		{"partner with someone else", pir, rowan, false},
		{"partner and a plain legend", thrasios, golos, false},
		{"choose a background and a creature", wilson, thrasios, false},
		{"two backgrounds", raisedByGiants, raisedByGiants, false},
		{"doctor's companion and a non-doctor", clara, golos, false},
		{"two doctors", eleventhDoctor, eleventhDoctor, false},
		{"friends forever and partner", eleven, tymna, false},
		{"different partner qualities", firstSurvivor, father, false},
		{"partner quality and plain partner", firstSurvivor, thrasios, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, canPair(tt.first, tt.second))
		})
	}
}

func TestCanPair_ReadsTheFrontFace(t *testing.T) {
	front := Printing{
		Name:       "Front Partner // Back Side",
		TypeLine:   "Legendary Creature — Human // Legendary Creature — Werewolf",
		OracleText: "Partner (You can have two commanders if both have partner.)\n//\nTrample",
	}
	back := Printing{
		Name:       "Front Side // Back Partner",
		TypeLine:   "Legendary Creature — Human // Legendary Creature — Werewolf",
		OracleText: "Trample\n//\nPartner (You can have two commanders if both have partner.)",
	}

	assert.True(t, canPair(front, tymna))
	assert.False(t, canPair(back, tymna))
}
