package scancodes

import (
  "fmt"
)

func ExampleSequenceForChar_letters() {
  fmt.Printf("%s %s\n", SequenceForChar('a'), SequenceForChar('A'))
  fmt.Printf("%s %s\n", SequenceForChar('v'), SequenceForChar('V'))
  // Output:
  // A LShift+A
  // V LShift+V
}
func ExampleSequenceForChar_symbols() {
  fmt.Printf("%s %s\n", SequenceForChar(';'), SequenceForChar(':'))
  // Output:
  // ; LShift+;
}
func ExampleSequenceForChar_whitespace() {
  fmt.Printf("%s %s\n", SequenceForChar(' '), SequenceForChar('\n'))
  // Output:
  // Space Enter
}

func ExampleSequenceForString() {
  fmt.Println(SequenceForString("Hello world!\n"))
  // Output: LShift+H E L L O Space W O R L D LShift+1 Enter
}

func ExampleForSequence_key() {
  codes, err := ForSequence("Esc")
  fmt.Printf("%x %v", codes, err)
  // Output:
  // [1 81] <nil>
}
func ExampleForSequence_konami() {
  codes, err := ForSequence("Up Up Down Down Left Right Left Right B A")
  fmt.Printf("%x %v", codes, err)
  // Output:
  // [48 c8 48 c8 50 d0 50 d0 4b cb 4d cd 4b cb 4d cd 30 b0 1e 9e] <nil>
}
func ExampleForSequence_reboot() {
  codes, err := ForSequence("LCtrl+LAlt+Del")
  fmt.Printf("%x %v", codes, err)
  // Output: [1d 38 53 9d b8 d3] <nil>
}
func ExampleForSequence_error() {
  codes, err := ForSequence("Food")
  fmt.Printf("%x %v", codes, err)
  // Output: [] Unknown keyboard key Food
}

func ExampleForString() {
  codes, err := ForString("Hello")
  fmt.Printf("%x %v", codes, err)
  // Output: [2a 23 aa a3 12 92 26 a6 26 a6 18 98] <nil>
}
