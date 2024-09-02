// Package scancodes computes the Set 1 (IBM PC XT) keyboard scancodes produced
// by typing in a string.
package scancodes

import (
  "bytes"
  "errors"
  "strings"
  "unicode"
)

// ForString obtains the series of scancodes needed to produce a string.
func ForString(input string) ([]int, error) {
  return ForSequence(SequenceForString(input))
}

// ForSequence obtains the series of scancodes for a sequence of keypresses.
func ForSequence(sequence string) ([]int, error) {
  codes := make([]int, 0, len(sequence))
  depress := make([]int, 0, 2)
  for _, chord := range strings.Split(sequence, " ") {
    chordLoop: for _, key := range strings.Split(chord, "+") {
      for code, codeKey := range baseCodes {
        if key == codeKey || key == numLockCodes[code] {
          codes = append(codes, code)
          depress = append(depress, 0x80 | code)
          continue chordLoop
        }
      }
      for codeKey, codeSequence := range escapeTable {
        if key == codeKey {
          codes = append(codes, codeSequence[0], codeSequence[1])
          depress = append(depress, codeSequence[0], 0x80 | codeSequence[1])
          continue chordLoop
        }
      }
      return nil, errors.New("Unknown keyboard key " + key)
    }
    codes = append(codes, depress...)
    depress = depress[:0]
  }
  return codes, nil
}

// SequenceForString converts an input string into a sequence of keypresses.
func SequenceForString(input string) string {
  sequence := bytes.Buffer{}
  wroteFirst := false
  for _, char := range input {
    if wroteFirst {
      sequence.WriteRune(' ')
    } else {
      wroteFirst = true
    }
    sequence.WriteString(SequenceForChar(char))
  }
  return sequence.String()
}

// SequenceForChar converts an input character into a sequence of keypresses.
func SequenceForChar(char rune) string {
  // Letters.
  if unicode.IsLower(char) {
    return string(unicode.ToUpper(char))
  } else if unicode.IsUpper(char) {
    return "LShift+" + string(char)
  }
  switch char {
  case '\n':
    return "Enter"
  case '\t':
    return "Tab"
  case ' ':
    return "Space"
  }
  for code, keyName := range shiftCodes {
    if keyName == string(char) {
      return "LShift+" + baseCodes[code]
    }
  }
  return string(char)
}
