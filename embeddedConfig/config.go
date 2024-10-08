package embeddedConfig

const Toml string = `
# xswitcher config file. Uses TOML markup language.
[ScanDevices]
 # Must exist on start. Self-respawn in case it is younger then 30s
 Test = "/dev/input/event0"
 Respawn = 30
 # Search mask
 Search = "/dev/input/event*"
 # In my thinkPads there are such a pseudo-keyboards whith tons of unnecessary events.
 Bypass = "(?i)Video|Camera" # "(?i)" obviously differs from "classic" pcre's.

[Keyboard]
  Delay = 5 # Delay in the virtual keyboard before sending the next event

[Templates] # "@name@" to simplify expressions
 # Words can consist of these chars (regex) !!! There must be only "canonic" key names, or regex will fail to match !!!
 "WORD" = "([0-9A-Z=-]|GRAVE|APOSTROPHE|SEMICOLON|[LR]_BRACE|COMMA|DOT|(BACK)?SLASH|KP[0-9])"
 "SEPARATOR" = "(((BACK)?SPACE)|([=-]|(KP)?MINUS|(KP)?ENTER|ESC|TAB))"
 "COMPOSE" = "([0-9=-]|[LR]_BRACE)"

# actions = (Add, Drop, Test), key codes are defined in "keys.go".
# !!! To be converted into jump matrix inside. Avoid stupid cyclic tests!!!
[ActionKeys]
 Layouts = [0, 1] # Don't impact extra languages (e.g., Chinese)

 # Collect key and do the test for command sequence
 # !!! Repeat codes (code=2) must be collected once per key!
 Add = ["1..0", "-", "=", "BACKSPACE", "Q..]", "L_CTRL..CAPS", "N_LOCK", "S_LOCK",
        "KP7..KPDOT", "R_CTRL", "KPSLASH", "R_ALT", "KPEQUAL..PAUSE",
        "KPCOMMA", "L_META..COMPOSE", "KPLEFTPAREN", "KPRIGHTPAREN"]

 # Drop all collected keys, including this.  This is default action.
 Drop = ["ESC", "TAB", "ENTER", "KPENTER", "LINEFEED..POWER"]

 # Store extra map for these keys, when any is in "down" state.
 # State is checked via "OFF:"|"ON:" conditions in action.
 # (Also, state of these keys must persist between buffer drops.)
 # ToDo: StateKeys are hardcoded now.
 StateKeys = ["L_CTRL", "L_SHIFT", "L_ALT", "L_META", "CAPS", "N_LOCK", "S_LOCK",
              "R_CTRL", "R_SHIFT", "R_ALT", "R_META"]

 # Test only, but don't collect.
 # E.g., I use F12 instead of BREAK on dumb laptops whith shitty keyboards (new ThinkPads)
 Test = ["F1..F10", "ZENKAKUHANKAKU", "102ND", "F11", "F12",
          "RO..KPJPCOMMA", "SYSRQ", "SCALE", "HANGEUL..YEN",
          "STOP..SCROLLDOWN", "NEW..MAX"]


# Some behaviour can depend on application currently doing the input.
# During runtime, [WindowClasses] are checked in the order in wich they are declared in this config.
[[WindowClasses]]
 # VNC, VirtualBox, qemu etc. emulates there input independently, so never intercept.
 # With the exception of some stupid VNC clients, which does high-level (layout-based) keyboard input.
 Regex = "^(VirtualBox|Focus-Proxy-Window)"
 Actions = "" # Do nothing while focus stays in VirtualBox | ATEN-based java VNC-console

[[WindowClasses]]
 Regex = "^konsole"
 # In general, mouse clicks leads to unpredictable (at the low-level where xswitcher resides) cursor jumps.
 # So, it's good choise to drop all buffers after click.
 # But some windows, e.g. terminals, can stay out of this problem.
 MouseClickDrops = false
 Actions = "Actions"

[[WindowClasses]] # Default behaviour: no Regex (or wildcard like ".")
 MouseClickDrops = true
 Actions = "Actions"


# action = [ test1, test2, ... ]
# test = (ON|OFF|SEQ):(regex)
# "ON:(regex)"  - True if at least one of pressed state keys matches regex.
# "OFF:(regex)" - True if no one of state keys matches regex.
# Note the virtual state key "WORD". It's "pressed" just after the RetypeWord and stays down until NewWord.
# "SEQ:(regex)" — True if last key events (the comma-separated chain of up to SeqLength events) matches regex.
# Use "xswitcher -t" in command line to view typed chain in STDERR.
[Actions]
# Inverse regex is hard to understand, so extract negation to external condition.
# Expresions will be checked in direct order, one-by-one. Condition succceds when ALL results are True.
 # Maximum key sequence length, extra keys will be dropped. More length - more CPU.
 SeqLength = 12
 # Count only those chars in word which must be removed (by sending the same count of BS keys) while "RetypeWord".
 WordChars = "(^@WORD@:0$)"
 # Drop word buffer and start collecting new one
 # !!! Note that (@WORD@:1) /key-press/ at the end of regex allows to collect the last char,
 #     while (@WORD@:0) cuts the WORD strictly at the end of sequence. !!!
 # --- In my layout, R_META selects 5'th layout and R_ALT does "compose". ---
 NewWord = [ "SEQ:(@SEPARATOR@:[12]),(((CAPS:[012])|([LR]_SHIFT:[12])|(R_META:0)|((@WORD@|@SEPARATOR@):0)),)*(@WORD@:1)", # "@WORD@:0" then collects the char
             "SEQ:(@WORD@:2,@WORD@:0)", # Drop repeated char at all: unlikely it needs correction
             "SEQ:(BACKSPACE:0)", # Drop buffer just afrer BACKSPACE
             # Control sequences. !!! Must be dropped ASAP.
             "SEQ:((([LR]_CTRL|L_ALT|[LR]_META):[12])(,((@WORD@|@SEPARATOR@):[012]))+(,(([LR]_CTRL|L_ALT|[LR]_META):0))+(,@WORD@:1)?)", # "@WORD@:0" then collects the char
             "ON:(WORD) SEQ:(,@WORD@:1)" ] # New input after previous correction. "@WORD@:0" then collects the char
 # Drop all buffers
 NewSentence = [ "SEQ:(ENTER:0)" ]

 # In some windows the single char must be deleted by single BS, so there is need in compose sequence detector.
 Compose = [ "OFF:(CTRL|L_ALT|META|SHIFT)  SEQ:(R_ALT:1(,R_ALT:2)?(,[LR]_SHIFT:[12])*(,@COMPOSE@:1,@COMPOSE@:0)(,[LR]_SHIFT:0)?(,@COMPOSE@:1,@COMPOSE@:0),R_ALT:0)",
             "OFF:(CTRL|L_ALT|META|SHIFT)  SEQ:(R_ALT:1(,R_ALT:2)?(,[LR]_SHIFT:[12])*,@COMPOSE@:1,@COMPOSE@:0(,[LR]_SHIFT:0)?,R_ALT:0(,[LR]_SHIFT:0)?,@COMPOSE@:1,@COMPOSE@:0)" ]

 # Try to type the text from clipboard to virtual keyboard.
 # Shift+Shift+L_Ctrl
# TypeClipboard = [ "OFF:(CTRL|L_ALT|META) SEQ:(L_CTRL:1,L_CTRL:0,[LR]_SHIFT:0,[LR]_SHIFT:0)"]

 # !!! Note that xswitcher counts extra keys as the length of matched SEQ:() !!!
 # So, any regex for "RetypeWord" SEQ must be EXACT ONE. Otherwise, xswitcher will generate the troubles for You.
 "Action.RetypeWord" = [ "OFF:(CTRL|ALT|META|SHIFT)  SEQ:(PAUSE:1,PAUSE:0)",
                         "OFF:(CTRL|ALT|META|SHIFT)  SEQ:(F12:1,F12:0)" ]

 "Action.CyclicSwitch" = [ "OFF:(R_CTRL|ALT|META|SHIFT)  SEQ:(L_CTRL:1,L_CTRL:0)" ] # Single short LEFT CONTROL
 "Action.Respawn" = [ "OFF:(CTRL|ALT|META|SHIFT)  SEQ:(S_LOCK:2,S_LOCK:0)" ] # Long-pressed SCROLL LOCK

 "Action.Layout0" = [ "OFF:(CTRL|ALT|META|R_SHIFT)  SEQ:(L_SHIFT:1,L_SHIFT:0)" ] # Single short LEFT SHIFT
 "Action.Layout1" = [ "OFF:(CTRL|ALT|META|L_SHIFT)  SEQ:(R_SHIFT:1,R_SHIFT:0)" ] # Single short RIGHT SHIFT

 "Action.Hook1" = [ "OFF:(CTRL|R_ALT|META|SHIFT)  SEQ:(L_ALT:1,L_ALT:0)" ]


# Action is the array, so actions could be chained (m.b., infinitely... Have I to check this?).
# For each action type, extra named parameters could be collected. Invalid parameters will be ignored(?).
[Action.RetypeWord] # Switch layout, drop last word and type it again
 Action = [ "Action.CyclicSwitch", "RetypeWord" ] # Call Switch() between layouts tuned below, then RetypeWord()

[Action.CyclicSwitch] # Cyclic layout switching
 Action = [ "Switch" ] # Internal layout switcher func
 Layouts = [0, 1]

[Action.Layout0] # Direct layout selection
 Action = [ "Layout" ] # Internal layout selection func
 Layout = 0

[Action.Layout1] # Direct layout selection
 Action = [ "Layout" ] # Internal layout selection func
 Layout = 1

[Action.Respawn] # Completely respawn xswitcher. Reload config as well
 Action = [ "Respawn" ]

[Action.Hook1] # Run external commands
  Action = [ "Exec" ]
  Exec = "cat > /tmp/xxx" # All the input after the 1'st space will be reassembled into the CLI args.
#  Timeout = 1000 # Wait up to 1 second, then kill the executing process.
#  Wait = true # Wait for termination, then output the result to STDOUT|STDERR.
  # External hook can process collected buffer by it's own means.
  # The last typed word, or sentence, or just the custom string could be passed to stdIn.
  SendBuffer = "WORD" # "WORD"|"SENTENCE"|"any custom input\n"

  UseShell = true # Execute inside /bin/bash
#  Directory = "/path/to/" # Change the working directory to this one.
  CleanEnv = true # Run command inside the clean environment.
  Environment = [ "var=value" ] # A set of environment variables.

# Use the specific user/group.
  UID = "root"
  GID = "root"
`
