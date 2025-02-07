package main

import "fmt"

func main() {
	var a A
	for _, z := range []int{1} { // want `function call at line 9 may store a reference to z`
		var y int
		a.unsafeWrites(&z, &y)
	}
	for _, x := range []int{1, 2} { // want `function call which takes a reference to x at line 12 may start a goroutine`
		unsafeAsync(&x)
	}
	for _, x := range []int{1, 2} {
		a.safe("hello", &x)
	}
	for _, x := range []int{1, 2, 3, 4} { // want `function call at line 18 may store a reference to x`
		unsafeCallsAWrite(a, &x)
	}
	for _, y := range []int{1} { // want `function call at line 21 may store a reference to y`
		unsafeCallsAWriteViaPointerLabyrinth(&y)
	}
	for _, w := range []int{1} {
		safe(&w)
	}
	for _, x := range []int{1} { // want `function call at line 27 passes reference to x to third-party code`
		callThirdParty(&x)
	}
	for _, y := range []int{1} {
		callThirdPartyAcceptListed(&y)
	}
	var y UnsafeStruct
	for _, x := range []int{1, 2, 3} { // want `reference to x was used in a composite literal at line 34`
		y = UnsafeStruct{&x}
	}
	for _, y := range []int{1} { // want `reference to y was used in a composite literal at line 37`
		useUnsafeStruct(UnsafeStruct{&y})
	}
	var x *int
	for _, z := range []int{1} { // want `reference to z is reassigned at line 41`
		x = &z
	}
	fmt.Println(x, y) // for use
}

func useUnsafeStruct(x UnsafeStruct) {
	fmt.Println(x)
}

type UnsafeStruct struct {
	x *int
}

type A struct {
}
type B struct {
	a A
}

func (b *B) unsafeWritesNoArgs() {
	b.a.unsafeAsyncToWrite(2)
}

func (a *A) veryUnsafeNoArgs() {
	var x, y int
	z := a
	z.unsafeAsyncToWrite(x)

	a.unsafeWrites(&x, &y)
	a.safe("1", &x)
	unsafeAsync(&x)
	unsafeAsync(&x)
}

func (a *A) unsafeAsyncToWrite(x int) {
	go a.unsafeWrites(&x, &x)
}

func (a *A) unsafeWrites(x, y *int) *int {
	return struct {
		x, y *int
	}{x, y}.x // why not?
}

func (a *A) safe(x string, y *int) *int {
	return nil
}

func unsafeCallsAWrite(a A, x *int) {
	a.unsafeWrites(x, x)
}

func safe(x *int) {
	safe1(x)
}
func safe1(x *int) {
	safe2(*x)
}

// callgraph should be cut off here, since safe2 does not pass a pointer.
func safe2(x int) {
	unsafeAsync(&x)
	unsafeCallsAWriteViaPointerLabyrinth(&x)
}

func unsafeAsync(x *int) {
	unsafeAsync1(x)
}
func unsafeAsync1(x *int) {
	unsafeAsync2(x)
}
func unsafeAsync2(x *int) {
	unsafeAsync3(x)
}
func unsafeAsync3(x *int) {
	go func() {

	}()
}

func unsafeCallsAWriteViaPointerLabyrinth(x *int) {
	labyrinth1(3, "hello", x, 4.0)
}

func labyrinth1(x int, y string, z *int, w float32) { // z unsafe
	forPtr := 3
	labyrinth2(y, z, &forPtr)
}

func labyrinth2(y string, z *int, w *int) { // z unsafe
	labyrinth3(w, z, w)
}

func labyrinth3(x *int, z *int, y *int) {
	labyrinth4(z, x, y)
}

func labyrinth4(z *int, x *int, y *int) {
	writePtr(z)
} // okay so it's only a tiny labyrinth... :shrug:

func writePtr(x *int) {
	var y *int
	y = x // 'write' is triggered here
	fmt.Println(y)
}

func callThirdParty(x *int) {
	callThirdParty1(x)
}

func callThirdParty1(x *int) {
	callThirdParty2(x)
}

func callThirdParty2(x *int) {
	fmt.Println(x) // fmt.Println is not accept-listed;
}

func callThirdPartyAcceptListed(x *int) {
	callThirdPartyAcceptListed1(x)
}

func callThirdPartyAcceptListed1(x *int) {
	callThirdPartyAcceptListed2(x)
}

func callThirdPartyAcceptListed2(x *int) {
	fmt.Printf("%v", x) // fmt.Printf *is* accept-listed;
}
