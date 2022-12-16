package sol

//lint:file-ignore ST1005 Error messages are toasted, so need to be capitalized
//lint:file-ignore ST1006 Receiver name will be anything I like, thank you

import (
	"errors"
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

type Reserve struct {
	parent *Pile
}

func NewReserve(slot image.Point, fanType FanType) *Pile {
	reserve := NewPile("Reserve", slot, fanType, MOVE_ONE)
	reserve.vtable = &Reserve{parent: &reserve}
	TheBaize.AddPile(&reserve)
	return &reserve
}

func (*Reserve) CanAcceptTail(tail []*Card) (bool, error) {
	return false, errors.New("Cannot add a card to a Reserve")
}

func (self *Reserve) TailTapped(tail []*Card) {
	self.parent.DefaultTailTapped(tail)
}

// Conformant when contains zero or one card(s), same as Waste
func (self *Reserve) Conformant() bool {
	return self.parent.Len() < 2
}

// UnsortedPairs - cards in a reserve pile are always considered to be unsorted
func (self *Reserve) UnsortedPairs() int {
	if self.parent.Empty() {
		return 0
	}
	return self.parent.Len() - 1
}

func (self *Reserve) MovableTails() []*MovableTail {
	// nb same as Cell.MovableTails
	var tails []*MovableTail = []*MovableTail{}
	if self.parent.Len() > 0 {
		var card *Card = self.parent.Peek()
		var tail []*Card = []*Card{card}
		var homes []*Pile = TheBaize.FindHomesForTail(tail)
		for _, home := range homes {
			tails = append(tails, &MovableTail{dst: home, tail: tail})
		}
	}
	return tails
}

func (self *Reserve) Placeholder() *ebiten.Image {
	return nil
}
