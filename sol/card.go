package sol

import (
	"fmt"
	"image"
	"log"
	"math/rand"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"oddstream.games/gosol/util"
)

const (
	cardmagic    = 0x29041962
	LERP_SECONDS = 0.5
	FLIP_SECONDS = LERP_SECONDS / 3
	SPIN_SECONDS = LERP_SECONDS * 2
)

/*
	Cards have several states: idle, being dragged, transitioning, shaking, spinning, flipping
	You'd think that cards should have a 'state' enum, but the states can overlap (eg a card
	can transition and flip at the same time)
*/

// Card object
type Card struct {
	// static things
	magic uint32
	ID    CardID // contains pack, ordinal, suit, ordinal (and bonus prone and joker flag bits)

	// dynamic things
	owner        *Pile
	pos          image.Point
	destinations []CardDestination

	// lerping things
	src           image.Point // lerp origin
	dst           image.Point // lerp destination
	lerpStartTime time.Time
	lerping       bool

	// dragging things
	dragStart    image.Point // starting point for dragging
	beingDragged bool        // true if this card is being dragged, or is in a dragged tail

	// flipping things
	flipWidth     float64 // scale of the card width while flipping
	flipDirection int
	flipStartTime time.Time

	// spinning things
	directionX, directionY int     // direction vector when card is spinning
	angle, spin            float64 // current angle and spin when card is spinning
	spinStartAfter         time.Time
}

// NewCard is a factory for Card objects
func NewCard(pack, suit, ordinal int) Card {
	// be nice to start the cards in the middle of the screen, but the screen will be 0,0 when app starts
	c := Card{magic: cardmagic, ID: NewCardID(pack, suit, ordinal), pos: image.Point{0, 0}}
	// a joker ID will be created by having NOSUIT (0) and ordinal == 0
	c.SetProne(true)
	// could do c.lerpStep = 1.0 here, but a freshly created card is soon SetPosition()'ed
	return c
}

func (c *Card) Valid() bool {
	return c != nil && c.magic == cardmagic
}

func (c *Card) SetOwner(p *Pile) {
	// p may be nil if we have just popped the card
	c.owner = p
}

func (c *Card) Owner() *Pile {
	return c.owner
}

// String satisfies the Stringer interface (defined by fmt package)
func (c *Card) String() string {
	return fmt.Sprintf("%s %s", util.OrdinalToShortString(c.Ordinal()), SuitIntToString(c.Suit()))
}

// Pos returns the x,y baize coords of this card
func (c *Card) BaizePos() image.Point {
	return c.pos
}

// SetPosition sets the position of the Card
func (c *Card) SetBaizePos(pos image.Point) {
	c.pos = pos
	c.lerping = false
}

// Rect gives the x,y baize coords of the card's top left and bottom right corners
func (c *Card) BaizeRect() image.Rectangle {
	var r image.Rectangle
	r.Min = c.pos
	r.Max = r.Min.Add(image.Point{CardWidth, CardHeight})
	return r
}

// ScreenRect gives the x,y screen coords of the card's top left and bottom right corners
func (c *Card) ScreenRect() image.Rectangle {
	var r image.Rectangle = c.BaizeRect()
	r.Min = r.Min.Add(TheBaize.dragOffset)
	r.Max = r.Max.Add(TheBaize.dragOffset)
	return r
}

func (c *Card) NearEnough() bool {
	return util.Abs(c.pos.X-c.dst.X) <= 2 && util.Abs(c.pos.Y-c.dst.Y) <= 2
}

// TransitionTo starts the transition of this Card to pos
func (c *Card) TransitionTo(pos image.Point) {

	if c.Spinning() {
		return
	}

	if NoCardLerp || pos.Eq(c.pos) {
		c.SetBaizePos(pos)
		return
	}

	if c.lerping {
		if c.NearEnough() {
			c.SetBaizePos(pos)
			return // repeat request
		}
	}

	c.src = c.pos
	c.dst = pos
	c.lerping = true
	c.lerpStartTime = time.Now()
}

// StartDrag informs card that it is being dragged
func (c *Card) StartDrag() {
	if c.Transitioning() {
		log.Printf("StartDrag a transitioning card %s", c.String())
		// set the drag origin to the be transition destination,
		// so that cancelling this drag will return the card
		// to where it thought it was going
		c.dragStart = c.dst
	} else {
		c.dragStart = c.pos
	}
	c.beingDragged = true
	// println("start drag", c.ID.String(), "start", c.dragStartX, c.dragStartY)
}

// DragBy repositions the card by the distance it has been dragged
func (c *Card) DragBy(dx, dy int) {
	// println("Card.DragBy(", c.dragStartX+dx-c.baizeX, c.dragStartY+dy-c.baizeY, ")")
	c.SetBaizePos(c.dragStart.Add(image.Point{dx, dy}))
}

// DragStartPosition returns the x,y screen coords of this card before dragging started
// func (c *Card) DragStartPosition() (int, int) {
// return c.dragStartX, c.dragStartY
// }

// StopDrag informs card that it is no longer being dragged
func (c *Card) StopDrag() {
	c.beingDragged = false
	// println("stop drag", c.ID.String())
}

// CancelDrag informs card that it is no longer being dragged
func (c *Card) CancelDrag() {
	c.beingDragged = false
	// println("cancel drag", c.ID.String(), "start", c.dragStartX, c.dragStartY, "screen", c.screenX, c.screenY)
	c.TransitionTo(c.dragStart)
}

// WasDragged returns true of this card has been dragged
func (c *Card) WasDragged() bool {
	// could use .beingDragged
	return !c.pos.Eq(c.dragStart)
}

func (c *Card) startFlip() {
	c.flipWidth = 1.0    // card starts full width
	c.flipDirection = -1 // start by making card narrower
	c.flipStartTime = time.Now()
}

// FlipUp flips the card face up
func (c *Card) FlipUp() {
	if c.Prone() {
		c.SetProne(false) // card is immediately face up, else fan isn't correct
		if !NoCardFlip {
			c.startFlip()
		}
	}
}

// FlipDown flips the card face down
func (c *Card) FlipDown() {
	if !c.Prone() {
		c.SetProne(true) // card is immediately face down, else fan isn't correct
		if !NoCardFlip {
			c.startFlip()
		}
	}
}

// Flip turns the card over
func (c *Card) Flip() {
	if c.Prone() {
		c.FlipUp()
	} else {
		c.FlipDown()
	}
}

// SetFlip turns the card over
func (c *Card) SetFlip(prone bool) {
	if prone {
		c.FlipDown()
	} else {
		c.FlipUp()
	}
}

// StartSpinning tells the card to start spinning
func (c *Card) StartSpinning() {
	c.directionX = rand.Intn(9) - 4
	c.directionY = rand.Intn(9) - 3 // favor falling downwards
	c.spin = rand.Float64() - 0.5
	c.destinations = nil
	// delay start of spinning to allow cards to be seen to go to foundations
	// https://stackoverflow.com/questions/67726230/creating-a-time-duration-from-float64-seconds
	d := time.Duration(SPIN_SECONDS * float64(time.Second))
	c.spinStartAfter = time.Now().Add(d)
}

// StopSpinning tells the card to stop spinning and return to it's upright state
func (c *Card) StopSpinning() {
	c.directionX, c.directionY = 0, 0
	c.angle, c.spin = 0, 0
}

// Spinning returns true if this card is spinning
func (c *Card) Spinning() bool {
	return c.spin != 0.0
}

// Transitioning returns true if this card is lerping
func (c *Card) Transitioning() bool {
	return c.lerping
}

// Dragging returns true if this card is being dragged
func (c *Card) Dragging() bool {
	return c.beingDragged
}

// Flipping returns true if this card is flipping
func (c *Card) Flipping() bool {
	if NoCardFlip {
		return false
	}
	return c.flipDirection != 0 // will be -1 or +1 if flipping
}

// Layout implements ebiten.Game's Layout.
// func (c *Card) Layout(outsideWidth, outsideHeight int) (int, int) {
// 	return outsideWidth, outsideHeight
// }

// Update the card state (transitions)
func (c *Card) Update() error {

	if c.Transitioning() {
		if !c.NearEnough() {
			t := time.Since(c.lerpStartTime).Seconds() / LERP_SECONDS
			c.pos.X = int(util.Smoothstep(float64(c.src.X), float64(c.dst.X), t))
			c.pos.Y = int(util.Smoothstep(float64(c.src.Y), float64(c.dst.Y), t))
		} else {
			c.SetBaizePos(c.dst) // also stops lerping
		}
	}

	if c.Flipping() {
		t := time.Since(c.flipStartTime).Seconds() / FLIP_SECONDS
		if c.flipDirection < 0 {
			c.flipWidth = util.Lerp(1.0, 0.0, t)
			if c.flipWidth <= 0.0 {
				// reverse direction, make card wider
				c.flipDirection = 1
				c.flipStartTime = time.Now()
			}
		} else if c.flipDirection > 0 {
			c.flipWidth = util.Lerp(0.0, 1.0, t)
			if c.flipWidth >= 1.0 {
				c.flipDirection = 0
				c.flipWidth = 1.0
			}
		}
	}

	if c.Spinning() {
		if time.Now().After(c.spinStartAfter) {
			c.lerping = false
			c.pos.X += c.directionX
			c.pos.Y += c.directionY
			// pearl from the mudbank:
			// cannot flip card here (or anytime while spinning)
			// because Baize.Complete() will fail (and record a lost game)
			// because UnsortedPairs will "fail" because some cards will be face down
			// so do not call c.Flip() here
			c.angle += c.spin
			if c.angle > 360 {
				c.angle -= 360
			} else if c.angle < 0 {
				c.angle += 360
			}
		}
	}
	return nil
}

// Draw renders the card into the screen
func (c *Card) Draw(screen *ebiten.Image) {

	op := &ebiten.DrawImageOptions{}

	var img *ebiten.Image
	// card prone has already been set to destination state
	if c.flipDirection < 0 {
		if c.Prone() {
			// card is getting narrower, and it's going to show face down, but show face up
			img = TheCardFaceImageLibrary[(c.Suit()*13)+(c.Ordinal()-1)]
		} else {
			// card is getting narrower, and it's going to show face up, but show face down
			img = CardBackImage
		}
	} else {
		if c.Prone() {
			img = CardBackImage
		} else {
			img = TheCardFaceImageLibrary[(c.Suit()*13)+(c.Ordinal()-1)]
		}
	}

	if img == nil {
		log.Panic("Card.Draw no image for ", c.String(), " prone: ", c.Prone())
	}

	if c.Flipping() {
		// img = ebiten.NewImageFromImage(img)
		op.GeoM.Translate(float64(-CardWidth/2), 0)
		op.GeoM.Scale(c.flipWidth, 1.0)
		op.GeoM.Translate(float64(CardWidth/2), 0)
	}

	if c.Spinning() {
		// do this before the baize position translate
		op.GeoM.Translate(float64(-CardWidth/2), float64(-CardHeight/2))
		op.GeoM.Rotate(c.angle * 3.1415926535 / 180.0)
		op.GeoM.Translate(float64(CardWidth/2), float64(CardHeight/2))

		// naughty to do this here instead of Update(), but Draw() knows the screen dimensions and Update() doesn't
		w, h := screen.Size()
		w -= TheBaize.dragOffset.X
		h -= TheBaize.dragOffset.Y
		switch {
		case c.pos.X+CardWidth > w:
			c.directionX = -rand.Intn(5)
			c.spin = rand.Float64() - 0.5
		case c.pos.X < 0:
			c.directionX = rand.Intn(5)
			c.spin = rand.Float64() - 0.5
		case c.pos.Y > h+CardHeight:
			c.directionX = rand.Intn(5) // go downwards
			c.pos.Y = -CardHeight       // start from off screen at top
		case c.pos.Y < -CardHeight:
			c.directionY = rand.Intn(5) // go downwards
		}
	}

	op.GeoM.Translate(float64(c.pos.X+TheBaize.dragOffset.X), float64(c.pos.Y+TheBaize.dragOffset.Y))

	if CardShadowImage != nil { // why would this be nil?
		if !c.Flipping() {
			switch {
			case c.Transitioning():
				xoffset, yoffset := 4.0, 4.0
				op.GeoM.Translate(xoffset, yoffset)
				screen.DrawImage(CardShadowImage, op)
				xoffset = -xoffset
				yoffset = -yoffset
				op.GeoM.Translate(xoffset, yoffset)
			case c.Dragging():
				// op.ColorM.Scale(1, 0.95, 1, 1)
				xoffset, yoffset := 4.0, 4.0
				op.GeoM.Translate(xoffset, yoffset)
				screen.DrawImage(CardShadowImage, op)
				// move the offset PARTIALLY back, making the card appear "pressed" when pushed with the mouse (like a button)
				xoffset = -xoffset * 0.5
				yoffset = -yoffset * 0.5
				op.GeoM.Translate(xoffset, yoffset)
				// this looks intuitively better than "lifting" the card with
				// op.GeoM.Translate(-offset*2, -offset*2)
				// even though "lifting" it (moving it up/left towards the light source) would be more "correct"
			}
		}
	}

	// if c.Owner().target && c == c.Owner().Peek() {
	// 	// op.GeoM.Translate(2, 2)
	// 	op.ColorM.Scale(0.95, 0.95, 0.95, 1)
	// }

	if ThePreferences.ShowMovableCards {
		if c.Owner().IsStock() {
			// card will be prone because Stock
			img = MovableCardBackImage
		} else {
			if len(c.destinations) > 0 {
				// c.destinations has been sorted so weightiest is first
				switch c.destinations[0].weight {
				case -1: // Cell
					op.ColorM.Scale(1.0, 1.0, 0.9, 1)
				case 0: // Normal
					op.ColorM.Scale(1.0, 1.0, 0.8, 1)
				case 1: // Suit match or Foundation
					op.ColorM.Scale(1.0, 1.0, 0.7, 1)
				case 2: // Discard or Foundation
					op.ColorM.Scale(1.0, 1.0, 0.65, 1)
				default:
					op.ColorM.Scale(0.9, 0.9, 0.9, 1)
				}
			}
		}
	}

	if img != nil {
		screen.DrawImage(img, op)
	}
}
