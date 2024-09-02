package scancodes

import (
  "testing"
)

func TestBaseTable(t *testing.T) {
  if len(baseCodes) != 0x55 {
    t.Errorf("Table misses %d entries", 0x55 - len(baseCodes))
  }
  if baseCodes[0x10] != "Q" {
    t.Error("Misalignment before Q")
  }
  if baseCodes[0x1e] != "A" {
    t.Error("Misalignment between Q-A")
  }
  if baseCodes[0x2c] != "Z" {
    t.Error("Misalignment between A-Z")
  }
  if baseCodes[0x3b] != "F1" {
    t.Error("Misalignment betwen Z-F1")
  }
  if baseCodes[0x47] != "Keypad_7" {
    t.Error("Misalignment between F1-Keypad_7")
  }
}
func TestShiftTable(t *testing.T) {
  if len(shiftCodes) != len(baseCodes) {
    t.Errorf("Table misses %d entries", len(baseCodes) - len(shiftCodes))
  }
  if shiftCodes[0x10] != "Q" {
    t.Error("Misalignment before Q")
  }
  if shiftCodes[0x1e] != "A" {
    t.Error("Misalignment between Q-A")
  }
  if shiftCodes[0x2c] != "Z" {
    t.Error("Misalignment between A-Z")
  }
  if shiftCodes[0x37] != "PrintScreen" {
    t.Error("Misalignment betwen Z-PrintScreen")
  }
}
func TestNumLockTable(t *testing.T) {
  if len(numLockCodes) != len(baseCodes) {
    t.Errorf("Table misses %d entries", len(baseCodes) - len(numLockCodes))
  }
  if numLockCodes[0x47] != "Home" {
    t.Error("Misalignment before Home")
  }
  if numLockCodes[0x53] != "Del" {
    t.Error("Misalignment between Home-Del")
  }
}


