package sol

import (
	"fmt"
	"hash/crc32"
	"image"
	"log"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"oddstream.games/gomps5/input"
	"oddstream.games/gomps5/sound"
	"oddstream.games/gomps5/ui"
	"oddstream.games/gomps5/util"
)

const (
	baizemagic           uint32 = 0x19910920
	dirtyWindowSize      uint32 = 0b000001
	dirtyPilePositions   uint32 = 0b000010
	dirtyCardSizes       uint32 = 0b000100
	dirtyPileBackgrounds uint32 = 0b001000
	dirtyCardPositions   uint32 = 0b010000
)

// Baize object describes the baize
type Baize struct {
	magic        uint32
	cardLibrary  []Card // the place where Card exists, everything else is a *Card
	script       ScriptInterface
	piles        []*Pile
	stock        *Pile
	waste        *Pile
	foundations  []*Pile
	tableaux     []*Pile
	discards     []*Pile
	tail         []*Card // array of cards currently being dragged
	bookmark     int
	undoStack    []*SavableBaize
	dirtyFlags   uint32
	stroke       *input.Stroke
	dragStart    image.Point
	dragOffset   image.Point
	WindowWidth  int // the most recent window width given to Layout
	WindowHeight int // the most recent window height given to Layout
}

//--+----1----+----2----+----3----+----4----+----5----+----6----+----7----+----8

// NewBaize is the factory func for the single Baize object
func NewBaize() *Baize {
	// let WindowWidth,WindowHeight be zero, so that the first Layout will trigger card scaling and pile placement
	return &Baize{magic: baizemagic, dragOffset: image.Point{0, 0}, dirtyFlags: 0xffffffff}
}

func (b *Baize) flagSet(flag uint32) bool {
	return b.dirtyFlags&flag == flag
}

func (b *Baize) setFlag(flag uint32) {
	b.dirtyFlags |= flag
}

func (b *Baize) clearFlag(flag uint32) {
	b.dirtyFlags &= ^flag
}

func (b *Baize) Valid() bool {
	return b.magic == baizemagic
}

func (b *Baize) CRC() uint32 {
	/*
		var crc uint = 0xFFFFFFFF
		var mask uint
		for _, p := range b.piles {
			crc = crc ^ uint(p.Len())
			for j := 7; j >= 0; j-- {
				mask = -(crc & 1)
				crc = (crc >> 1) ^ (0xEDB88320 & mask)
			}
		}
		return ^crc // bitwise NOT
	*/
	var lens []byte
	for _, p := range b.piles {
		lens = append(lens, byte(p.Len()))
	}
	return crc32.ChecksumIEEE(lens)
}

func (b *Baize) Reset() {
	b.StopSpinning()
	for _, p := range b.piles {
		p.subtype.Reset()
	}
	b.tail = nil
	b.undoStack = nil
	b.bookmark = 0
	b.dragStart = image.Point{0, 0}
	b.dragOffset = image.Point{0, 0}
}

func (b *Baize) Refan() {
	b.setFlag(dirtyCardPositions)
}

// NewGame resets Baize and restarts current variant with a new seed
func (b *Baize) NewDeal() {

	// a virgin game has one state on the undo stack
	if len(b.undoStack) > 1 && !b.Complete() {
		TheStatistics.RecordLostGame()
	}

	b.Reset()
	b.script.StartGame()
	b.UndoPush()
	sound.Play("Fan")

	b.setFlag(dirtyPilePositions | dirtyCardPositions)
	TheStatistics.WelcomeToast()
}

func (b *Baize) ShowVariantPicker() {
	TheUI.ShowVariantPicker(VariantNames())
}

func (b *Baize) Mirror() {
	/*
		0 1 2 3 4 5
		5 4 3 2 1 0

		0 1 2 3 4
		4 3 2 1 0
	*/
	var minX int = 32767
	var maxX int = 0
	for _, p := range b.piles {
		if p.slot.X < 0 {
			continue // ignore hidden pile
		}
		if p.slot.X < minX {
			minX = p.slot.X
		}
		if p.slot.X > maxX {
			maxX = p.slot.X
		}
	}
	for _, p := range b.piles {
		if p.slot.X < 0 {
			continue // ignore hidden pile
		}
		p.slot.X = maxX - p.slot.X + minX
		switch p.fanType {
		case FAN_RIGHT:
			p.fanType = FAN_LEFT
		case FAN_LEFT:
			p.fanType = FAN_RIGHT
		case FAN_RIGHT3:
			p.fanType = FAN_LEFT3
		case FAN_LEFT3:
			p.fanType = FAN_RIGHT3
		}
	}
}

// NewVariant resets Baize and starts a new game with a new variant and seed
func (b *Baize) NewVariant() {

	// a virgin game has one state on the undo stack
	if len(b.undoStack) > 1 && !b.Complete() {
		TheStatistics.RecordLostGame()
	}

	b.Reset()

	b.piles = nil

	b.stock = nil
	b.waste = nil
	b.foundations = nil
	b.discards = nil
	b.tableaux = nil

	b.script = GetVariantInterface(ThePreferences.Variant)
	b.script.BuildPiles()
	b.BuildAuxPiles()
	if ThePreferences.MirrorBaize {
		b.Mirror()
	}
	b.FindBuddyPiles()

	TheUI.SetTitle(ThePreferences.Variant)
	sound.Play("Fan")

	w, h := ebiten.WindowSize()
	b.Layout(w, h)

	b.script.StartGame()
	b.UndoPush()

	b.setFlag(dirtyPilePositions | dirtyPileBackgrounds | dirtyCardPositions | dirtyCardSizes | dirtyCardPositions)

	TheStatistics.WelcomeToast()
}

func (b *Baize) SetUndoStack(undoStack []*SavableBaize) {
	b.undoStack = undoStack
	sav := b.UndoPeek()
	b.UpdateFromSavable(sav)
	b.UpdateStatusbar()
}

// findPileAt finds the Pile under the mouse click or touch
func (b *Baize) FindPileAt(pt image.Point) *Pile {
	for _, p := range b.piles {
		if pt.In(p.FannedScreenRect()) {
			return p
		}
	}
	return nil
}

// FindCardAt finds the Card under the mouse click or touch
func (b *Baize) FindCardAt(pt image.Point) *Card {
	// go backwards, for King Albert's overlapping reserve piles
	for j := len(b.piles) - 1; j >= 0; j-- {
		p := b.piles[j]
		for i := p.Len() - 1; i >= 0; i-- {
			c := p.Get(i)
			if pt.In(c.ScreenRect()) {
				return c
			}
		}
	}
	return nil
}

func (b *Baize) BuildAuxPiles() {
	for _, p := range b.piles {
		switch (p.subtype).(type) {
		case *Stock:
			b.stock = p
		case *Waste:
			b.waste = p
		case *Discard:
			b.discards = append(b.discards, p)
		case *Foundation:
			b.foundations = append(b.foundations, p)
		case *Tableau:
			b.tableaux = append(b.tableaux, p)
		}
	}
}

func (b *Baize) LargestIntersection(c *Card) *Pile {
	var largestArea int = 0
	var pile *Pile = nil
	cardRect := c.BaizeRect()
	for _, p := range b.piles {
		if p == c.owner {
			continue
		}
		pileRect := p.FannedBaizeRect()
		intersectRect := pileRect.Intersect(cardRect)
		area := intersectRect.Dx() * intersectRect.Dy()
		if area > largestArea {
			largestArea = area
			pile = p
		}
	}
	return pile
}

// StartDrag return true if the Baize can be dragged
func (b *Baize) StartDrag() bool {
	b.dragStart = b.dragOffset
	return true
}

// DragBy move ('scroll') the Baize by dragging it
// dx, dy is the difference between where the drag started and where the cursor is now
func (b *Baize) DragBy(dx, dy int) {
	b.dragOffset.X = b.dragStart.X + dx
	if b.dragOffset.X > 0 {
		b.dragOffset.X = 0 // DragOffsetX should only ever be 0 or -ve
	}
	b.dragOffset.Y = b.dragStart.Y + dy
	if b.dragOffset.Y > 0 {
		b.dragOffset.Y = 0 // DragOffsetY should only ever be 0 or -ve
	}
}

// StopDrag stop dragging the Baize
func (b *Baize) StopDrag() {
	w, h := ebiten.WindowSize()
	b.CalcScrunchDims(w, h)
	b.Scrunch()
}

// StartSpinning tells all the cards to start spinning
func (b *Baize) StartSpinning() {
	for _, p := range b.piles {
		// use a method expression, which yields a function value with a regular first parameter taking the place of the receiver
		p.ApplyToCards((*Card).StartSpinning)
	}
}

// StopSpinning tells all the cards to stop spinning and return to their upright position
// debug only
func (b *Baize) StopSpinning() {
	for _, p := range b.piles {
		// use a method expression, which yields a function value with a regular first parameter taking the place of the receiver
		p.ApplyToCards((*Card).StopSpinning)
	}
	b.setFlag(dirtyCardPositions)
}

func (b *Baize) MakeTail(c *Card) bool {
	b.tail = c.Owner().MakeTail(c)
	return len(b.tail) > 0
}

func (b *Baize) AfterUserMove() {
	b.script.AfterMove()
	b.UndoPush()
	if b.Complete() {
		sound.Play("Complete")
		TheStatistics.RecordWonGame()
		TheStatistics.WonToast()
		TheUI.ShowFAB("star", ebiten.KeyN)
		b.StartSpinning()
	} else if b.Conformant() {
		TheUI.ShowFAB("done_all", ebiten.KeyA)
	} else {
		TheUI.HideFAB()
	}
}

/*
	InputStart finds out what object the user input is starting on
	(UI Container > Card > Pile > Baize, in that order)
	then tells that object.

	If the Input starts on a Card, then a tail of cards is formed.
*/
func (b *Baize) InputStart(v input.StrokeEvent) {
	b.stroke = v.Stroke

	if con := TheUI.FindContainerAt(v.X, v.Y); con != nil {
		if con.StartDrag(b.stroke) {
			b.stroke.SetDraggedObject(con)
		} else {
			b.stroke.Cancel()
		}
	} else {
		pt := image.Pt(v.X, v.Y)
		if c := b.FindCardAt(pt); c != nil {
			b.StartTailDrag(c)
			b.stroke.SetDraggedObject(c)
		} else {
			if p := b.FindPileAt(pt); p != nil {
				b.stroke.SetDraggedObject(p)
			} else {
				if b.StartDrag() {
					b.stroke.SetDraggedObject(b)
				} else {
					v.Stroke.Cancel()
				}
			}
		}
	}
}

func (b *Baize) InputMove(v input.StrokeEvent) {
	if v.Stroke.DraggedObject() == nil {
		log.Panic("*** move stroke with nil dragged object ***")
	}
	for _, p := range b.piles {
		p.target = false
	}
	switch v.Stroke.DraggedObject().(type) {
	case ui.Container:
		con := v.Stroke.DraggedObject().(ui.Container)
		con.DragBy(v.Stroke.PositionDiff())
	case *Card:
		b.DragTailBy(v.Stroke.PositionDiff())
		if c, ok := v.Stroke.DraggedObject().(*Card); ok {
			if p := b.LargestIntersection(c); p != nil {
				p.target = true
			}
		}
	case *Pile:
		// do nothing
	case *Baize:
		b.DragBy(v.Stroke.PositionDiff())
	default:
		log.Panic("*** unknown move dragging object ***")
	}
}

func (b *Baize) InputStop(v input.StrokeEvent) {
	if v.Stroke.DraggedObject() == nil {
		log.Panic("*** stop stroke with nil dragged object ***")
	}
	for _, p := range b.piles {
		p.target = false
	}
	switch v.Stroke.DraggedObject().(type) {
	case ui.Container:
		con := v.Stroke.DraggedObject().(ui.Container)
		con.StopDrag()
	case *Card:
		c := v.Stroke.DraggedObject().(*Card)
		if c.WasDragged() {
			src := c.Owner()
			// tap handled elsewhere
			// tap is time-limited
			if dst := b.LargestIntersection(c); dst == nil {
				println("no intersection for", c.String())
				b.CancelTailDrag()
			} else {
				if ok, err := src.subtype.CanMoveTail(b.tail); !ok {
					if err != nil {
						TheUI.Toast(err.Error())
					} else {
						println("*** NIL ERROR RETURN FROM CanMoveTail ***")
					}
					b.CancelTailDrag()
				} else {
					// it's ok to move this tail
					if src == dst {
						println("putting the tail back")
						b.CancelTailDrag()
					} else if ok, err := dst.subtype.CanAcceptTail(b.tail); !ok {
						if err != nil {
							TheUI.Toast(err.Error())
						} else {
							println("*** NIL ERROR RETURN FROM CanAcceptTail ***")
						}
						b.CancelTailDrag()
					} else {
						crc := b.CRC()
						if len(b.tail) == 1 {
							MoveCard(src, dst)
						} else {
							MoveCards(c, dst)
						}
						if crc != b.CRC() {
							b.AfterUserMove()
						}
						b.StopTailDrag()
					}
				}
			}
		}
	case *Pile:
		// do nothing
	case *Baize:
		// println("stop dragging baize")
		b.StopDrag()
	default:
		log.Panic("*** stop dragging unknown object ***")
	}
}

func (b *Baize) InputCancel(v input.StrokeEvent) {
	if v.Stroke.DraggedObject() == nil {
		log.Panic("*** cancel stroke with nil dragged object ***")
	}
	switch v.Stroke.DraggedObject().(type) { // type switch
	case ui.Container:
		con := v.Stroke.DraggedObject().(ui.Container)
		con.StopDrag()
	case *Card:
		b.CancelTailDrag()
	case *Pile:
		// p := v.Stroke.DraggedObject().(*Pile)
		// println("stop dragging pile", p.Class)
		// do nothing
	case *Baize:
		// println("stop dragging baize")
		b.StopDrag()
	default:
		log.Panic("*** cancel dragging unknown object ***")
	}
}

func (b *Baize) InputTap(v input.StrokeEvent) {
	// println("Baize.NotifyCallback() tap", v.X, v.Y)
	// a tap outside any open ui drawer (ie on the baize) closes the drawer
	switch obj := v.Stroke.DraggedObject().(type) { // type switch
	case *Card:
		crc := b.CRC()
		// offer TailTapped to the script first
		// to implement things like Stock.TailTapped
		// if the script doesn't want to do anything, it can call pile.subtype.TailTapped
		// which will either ignore it (eg Foundation, Discard)
		// or use Pile.GenericTailTapped to try to collect a card to Foundation (eg Tableau)
		b.script.TailTapped(b.tail)
		if crc != b.CRC() {
			sound.Play("Slide")
			b.AfterUserMove()
		}
		b.StopTailDrag()
	case *Pile:
		crc := b.CRC()
		b.script.PileTapped(obj)
		if crc != b.CRC() {
			sound.Play("Slide")
			b.AfterUserMove()
		}
	case *Baize:
		pt := image.Pt(v.X, v.Y)
		if con := TheUI.VisibleDrawer(); con != nil && !pt.In(image.Rect(con.Rect())) {
			con.Hide()
		}
	}
}

// NotifyCallback is called by the Subject (Input/Stroke) when something interesting happens
func (b *Baize) NotifyCallback(v input.StrokeEvent) {
	switch v.Event {
	case input.Start:
		b.InputStart(v)
	case input.Move:
		b.InputMove(v)
	case input.Stop:
		b.InputStop(v)
	case input.Cancel:
		b.InputCancel(v)
	case input.Tap:
		b.InputTap(v)
	default:
		log.Panic("*** unknown stroke event ***", v.Event)
	}
}

// ApplyToTail applies a method func to this card and all the others after it in the tail
func (b *Baize) ApplyToTail(fn func(*Card)) {
	// https://golang.org/ref/spec#Method_expressions
	// (*Card).CancelDrag yields a function with the signature func(*Card)
	// fn passed as a method expression so add the receiver explicitly
	for _, tc := range b.tail {
		fn(tc)
	}
}

// DragTailBy repositions all the cards in the tail: dx, dy is the position difference from the start of the drag
func (b *Baize) DragTailBy(dx, dy int) {
	// println("Pile.DragTailBy(", dx, dy, ")")
	for _, tc := range b.tail {
		tc.DragBy(dx, dy)
	}
}

func (b *Baize) StartTailDrag(c *Card) {
	if b.MakeTail(c) {
		b.ApplyToTail((*Card).StartDrag)
		ebiten.SetCursorMode(ebiten.CursorModeHidden)
	} else {
		println("failed to make a tail")
	}
}

func (b *Baize) StopTailDrag() {
	ebiten.SetCursorMode(ebiten.CursorModeVisible)
	b.ApplyToTail((*Card).StopDrag)
	b.tail = nil
}

func (b *Baize) CancelTailDrag() {
	ebiten.SetCursorMode(ebiten.CursorModeVisible)
	b.ApplyToTail((*Card).CancelDrag)
	b.tail = nil
}

func (b *Baize) Collect() {
	crc := b.CRC()
	for _, p := range b.piles {
		p.subtype.Collect()
	}
	if crc != b.CRC() {
		b.AfterUserMove()
	}
}

func (b *Baize) CollectAll() {
	outerCRC := b.CRC()
	for {
		innerCRC := b.CRC()
		for _, p := range b.piles {
			p.subtype.Collect()
		}
		if b.CRC() == innerCRC {
			break
		}
	}
	if b.CRC() != outerCRC {
		b.AfterUserMove()
	}
}

// ScaleCards calculates new width/height of cards and margins
// returns true if changes were made
func (b *Baize) ScaleCards() bool {

	print("ScaleCards ... ")

	// const (
	// 	DefaultRatio = 1.444
	// 	BridgeRatio  = 1.561
	// 	PokerRatio   = 1.39
	// 	OpsoleRatio  = 1.5556 // 3.5/2.25
	// )

	var OldWidth = CardWidth
	var OldHeight = CardHeight

	var maxX int
	for _, p := range b.piles {
		if p.Slot().X > maxX {
			maxX = p.Slot().X
		}
	}

	// "add" two extra piles and a LeftMargin to make a half-card-width border

	/*
		71 x 96 = 1:1.352 (Microsoft retro)
		140 x 190 = 1:1.357 (kenney, large)
		64 x 89 = 1:1.390 (official poker size)
		90 x 130 = 1:1.444 (nice looking scalable)
		89 x 137 = 1:1.539 (measured real card)
		57 x 89 = 1:1.561 (official bridge size)
	*/

	// Card padding is 10% of card height/width

	if ThePreferences.FixedCards {
		CardWidth = 90
		PilePaddingX = 9
		CardHeight = 122
		PilePaddingY = 13
		cardsWidth := PilePaddingX + CardWidth*(maxX+2)
		LeftMargin = (b.WindowWidth - cardsWidth) / 2
	} else {
		slotWidth := float64(b.WindowWidth) / float64(maxX+2)
		PilePaddingX = int(slotWidth / 10)
		CardWidth = int(slotWidth) - PilePaddingX
		slotHeight := slotWidth * 1.357
		PilePaddingY = int(slotHeight / 10)
		CardHeight = int(slotHeight) - PilePaddingY
		LeftMargin = (CardWidth / 2) + PilePaddingX
	}
	CardCornerRadius = float64(CardWidth) / 15.0
	TopMargin = 48 + CardHeight/3

	if CardWidth != OldWidth || CardHeight != OldHeight {
		println("did something")
	} else {
		println("did nothing")
	}
	return CardWidth != OldWidth || CardHeight != OldHeight
}

func (b *Baize) PercentComplete() int {
	var pairs, unsorted, percent int
	for _, p := range b.piles {
		if p.Len() > 1 {
			pairs += p.Len() - 1
		}
		unsorted += p.subtype.UnsortedPairs()
	}
	percent = (int)(100.0 - util.MapValue(float64(unsorted), 0, float64(pairs), 0.0, 100.0))
	return percent
}

func (b *Baize) UpdateStatusbar() {
	if !b.stock.Hidden() {
		TheUI.SetStock(b.stock.Len())
	}
	if b.waste != nil {
		TheUI.SetWaste(b.waste.Len())
	}
	TheUI.SetMoves(len(b.undoStack) - 1)
	TheUI.SetPercent(b.script.PercentComplete())
}

func (b *Baize) Conformant() bool {
	for _, p := range b.piles {
		if !p.subtype.Conformant() {
			return false
		}
	}
	return true
}

func (b *Baize) Complete() bool {
	for _, p := range b.piles {
		if !p.subtype.Complete() {
			return false
		}
	}
	return true
}

// Layout implements ebiten.Game's Layout.
func (b *Baize) Layout(outsideWidth, outsideHeight int) (int, int) {

	if outsideWidth == 0 || outsideHeight == 0 {
		println("Baize.Layout called with zero dimension")
		return outsideWidth, outsideHeight
	}

	if outsideWidth != b.WindowWidth {
		b.setFlag(dirtyWindowSize | dirtyCardSizes | dirtyPileBackgrounds | dirtyPilePositions | dirtyCardPositions)
		b.WindowWidth = outsideWidth
		ThePreferences.WindowWidth = outsideWidth
	}
	if outsideHeight != b.WindowHeight {
		b.setFlag(dirtyWindowSize)
		b.WindowHeight = outsideHeight
		ThePreferences.WindowHeight = outsideHeight
	}

	if b.dirtyFlags != 0 {
		NoDrawing = true
		if b.flagSet(dirtyCardSizes) {
			if b.ScaleCards() {
				CreateCardImages()
				b.setFlag(dirtyPilePositions | dirtyPileBackgrounds)
			}
			b.CalcScrunchDims(outsideWidth, outsideHeight)
			b.clearFlag(dirtyCardSizes)
		}
		if b.flagSet(dirtyPilePositions) {
			for _, p := range b.piles {
				p.SetBaizePos(image.Point{
					X: LeftMargin + (p.Slot().X * (CardWidth + PilePaddingX)),
					Y: TopMargin + (p.Slot().Y * (CardHeight + PilePaddingY)),
				})
			}
			b.CalcScrunchDims(outsideWidth, outsideHeight)
			b.clearFlag(dirtyPilePositions)
		}
		if b.flagSet(dirtyPileBackgrounds) {
			for _, p := range b.piles {
				p.img = p.CreateBackgroundImage()
			}
			b.clearFlag(dirtyPileBackgrounds)
		}
		if b.flagSet(dirtyWindowSize) {
			TheUI.Layout(outsideWidth, outsideHeight)
			b.clearFlag(dirtyWindowSize)
		}
		if b.flagSet(dirtyCardPositions) {
			for _, p := range b.piles {
				p.Scrunch() // always does a Refan()
			}
			b.clearFlag(dirtyCardPositions)
		}
		NoDrawing = false
	}

	return outsideWidth, outsideHeight
}

// Update the baize state (transitions, user input)
func (b *Baize) Update() error {

	if b.stroke == nil {
		input.StartStroke(b) // this will set b.stroke when "start" received
	} else {
		b.stroke.Update()
		if b.stroke.IsReleased() || b.stroke.IsCancelled() {
			b.stroke = nil
		}
	}

	for _, p := range b.piles {
		p.Update()
	}

	for k := ebiten.Key(0); k <= ebiten.KeyMax; k++ {
		if inpututil.IsKeyJustReleased(k) {
			Execute(k)
		}
	}

	TheUI.Update()

	return nil
}

// Draw renders the baize into the screen
func (b *Baize) Draw(screen *ebiten.Image) {

	screen.Fill(ExtendedColors[ThePreferences.BaizeColor])

	if NoDrawing {
		return
	}

	for _, p := range b.piles {
		p.Draw(screen)
		// for _, c := range p.cards {
		// 	c.Draw(screen)
		// }
	}
	for _, p := range b.piles {
		p.DrawStaticCards(screen)
	}
	for _, p := range b.piles {
		p.DrawTransitioningCards(screen)
	}
	for _, p := range b.piles {
		p.DrawFlippingCards(screen)
	}
	for _, p := range b.piles {
		p.DrawDraggingCards(screen)
	}

	TheUI.Draw(screen)
	// if DebugMode {
	// var ms runtime.MemStats
	// runtime.ReadMemStats(&ms)
	// ebitenutil.DebugPrint(screen, fmt.Sprintf("FPS %v, Alloc %v, NumGC %v", ebiten.CurrentTPS(), ms.Alloc, ms.NumGC))
	// ebitenutil.DebugPrint(screen, fmt.Sprintf("%v %v", b.bookmark, len(b.undoStack)))
	// bounds := screen.Bounds()
	// ebitenutil.DebugPrint(screen, bounds.String())
	// }

	if DebugMode {
		if ebiten.IsMouseButtonPressed(1) {
			if c := b.FindCardAt(image.Pt(ebiten.CursorPosition())); c != nil {
				p := c.owner
				index := p.IndexOf(c)
				ebitenutil.DebugPrint(screen, fmt.Sprintf("card=%s drag=%t pos=%s src=%s, dst=%s step=%0.f, cat=%s index=%d",
					c.String(),
					c.Dragging(),
					c.pos.String(),
					c.src.String(),
					c.dst.String(),
					c.lerpStep,
					p.category,
					index))
			}
		}
	}

}
