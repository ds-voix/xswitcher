package scancodes

// Tables lifted from:
//     http://www.win.tue.nl/~aeb/linux/kbd/scancodes-1.html#ss1.4

// Unshifted codes.
var baseCodes = []string{
  "Error",
  "Esc", "1", "2", "3", "4", "5", "6", "7", "8", "9", "0", "-", "=", "Backspace",
  "Tab", "Q", "W", "E", "R", "T", "Y", "U", "I", "O", "P", "[", "]",
  "Enter",
  "LCtrl",
  "A", "S", "D", "F", "G", "H", "J", "K", "L", ";", "'",
  "`",
  "LShift", "\\",
  "Z", "X", "C", "V", "B", "N", "M", ",", ".", "/", "RShift",
  "Keypad_*",
  "LAlt", "Space",
  "CapsLock",
  "F1", "F2", "F3", "F4", "F5", "F6", "F7", "F8", "F9", "F10",
  "NumLock", "ScrollLock",
  "Keypad_7", "Keypad_8", "Keypad_9",
  "Keypad_-",
  "Keypad_4", "Keypad_5", "Keypad_6", "Keypad_Plus",
  "Keypad_1", "Keypad_2", "Keypad_3",
  "Keypad_0", "Keypad-.",
  "Alt_SysRq",
}

// Shifted codes.
// NOTE: Uppercase letters are special-cased in code. They're only here because
//       they help us align the other keys.
var shiftCodes = []string{
  "",
  "", "!", "@", "#", "$", "%", "^", "&", "*", "(", ")", "_", "+", "",
  "", "Q", "W", "E", "R", "T", "Y", "U", "I", "O", "P", "{", "}",
  "Enter",
  "LCtrl",
  "A", "S", "D", "F", "G", "H", "J", "K", "L", ":", "\"",
  "~",
  "", "|",
  "Z", "X", "C", "V", "B", "N", "M", "<", ">", "?", "",
  "PrintScreen",
  "", "",
  "",
  "", "", "", "", "", "", "", "", "", "",
  "", "",
  "", "", "",
  "",
  "", "", "", "",
  "", "", "",
  "", "",
  "",
}

var numLockCodes = []string{
  "",
  "", "", "", "", "", "", "", "", "", "", "", "", "", "",
  "", "", "", "", "", "", "", "", "", "", "", "", "",
  "",
  "",
  "", "", "", "", "", "", "", "", "", "", "",
  "",
  "", "",
  "", "", "", "", "", "", "", "", "", "", "",
  "",
  "", "",
  "",
  "", "", "", "", "", "", "", "", "", "",
  "", "",
  "Home", "Up", "PageUp",
  "",
  "Left", "", "Right", "",
  "End", "Down", "PageDown",
  "Ins", "Del",
  "",
}

var escapeTable = map[string][]int{
  "Keypad_Enter": []int{0xe0, 0x1c},
  "RCtrl": []int{0xe0, 0x1d},
  "Keypad_/": []int{0xe0, 0x35},
  "Ctrl_PrintScreen": []int{0xe0, 0x37},
  "RAlt": []int{0xe0, 0x38},
  "Ctrl_Break": []int{0xe0, 0x46},
  "LeftWindow": []int{0xe0, 0x5b},
  "RightWindow": []int{0xe0, 0x5c},
  "Menu": []int{0xe0, 0x5d},
  "Power": []int{0xe0, 0x5e},
  "Sleep": []int{0xe0, 0x5f},
  "Wake": []int{0xe0, 0x63},
}
