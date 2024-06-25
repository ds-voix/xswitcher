module xswitcher

go 1.22.4


replace github.com/micmonay/keybd_event => ./src/keybd_event/

require (
	github.com/fsnotify/fsnotify v1.7.0
	github.com/gvalkov/golang-evdev v0.0.0-20220815104727-7e27d6ce89b6
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51
	github.com/micmonay/keybd_event v1.1.2
	github.com/pelletier/go-toml v1.9.5
	github.com/spf13/pflag v1.0.5
)

require golang.org/x/sys v0.4.0 // indirect
