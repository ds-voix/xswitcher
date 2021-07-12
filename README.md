# X language switcher v1.0 ("xswitcher")
Xswitcher is the fully customizable low-level keyboard helper for X.Org-based Linux desktops.  
It consists of two main parts: low-level keylogger and the virtual keyboard.  
On each key up/down event, xswitcher checks the last events chain against configured regex-based patterns.  
If one matches, xswitcher triggers the associated action. Action chains can be joined from simple actions.  
Look at TOML-based explained config file "xswitcher.conf" for further details.

## Retype
One of the most powerful actions of xswitcher is to retype the collected event buffer
after switching the language. Because of it's low-level nature, it may know nothing about layout details.  
So, xswitcher does nothing strange, in comparison with punto switcher on windows.  
It just sends a number of BackSpaces to wipe the last input, then calls language switcher
(internal, via X API, or external, via call of the configured program), an then retypes the same
keyboard events by means of virtual keyboard.

## Gnome is the pain
Note that the users of actual gnome-based shells can't use the XOrg embedded language switcher.
Due to the gnome built-in bug, X language API is broken by gnome since 2011.
However, there's the workaround. Look under the "gnome3" subfolder for customized config file and the simple helper "switch.gnome".
The helper header contains the brief instruction on how to deal with the https://github.com/lyokha/g3kb-switch.git

## Precompiled xswitcher for the beginners
The statically compiled binary lays under "bin" folder.  
The unexperienced users *(except of those who used gnome-based interfaces, e.g. ubuntu)* can just download this binary.  
Then put it under e.g. "/usr/local/bin" path and make it executable:  

    sudo chown root:root /usr/local/bin/xswitcher
    sudo chmod +xs /usr/local/bin/xswitcher

Note the sticky bit "+s" in this example. Being the low-level keylogger, xswitcher needs the access to "/dev/input/" devices.  
Someone can make xswitcher slightly less dangerous by adding the privilege-separated code.  
Or You yourself can try add your GUI user to "input" group (but I'm not sure it's enough to attach the virtual keyboard).  
All improvements are welcome.

Also note that, being an honest keylogger by nature, xswitcher can be of course configured to do something strange.  
E.g. you can configure it to email all the input to e.g. "fbi@usa.gov". Nobody bans.

## CLI
There are a number of helpful CLI flags:

    xswitcher -h
    Usage of :
      -c, --conf string   Non-default config location (default "/etc/xswitcher/xswitcher.conf")
      -d, --debug         Debug log level
      -t, --test          Only output all key events to STDERR. No actions.
      -v, --verbose       Increase log level to NOTICE

Use "-v" or even "-d" to debug when You implements any config changes.  
Use "-t" to collect the keyboard events string to construct Your custom actions.

## Respawn
This version of xswitcher do the scan of input devices only once (on it's launch).  
But there is the internal "respawn" action in case You connects new keyboard/mouse.  
By default, the long press of "Scroll lock" triggers "respawn". (Respawn does the complete self-restart).

## How to build.
You must install (libX11-devel | libx11-dev) + (libXmu-devel | libxmu-dev) packages for X bindings.  
* The package name may, of course, be different in Your distro.  
And, of course, you must have the working go environment.  
Obtain dependencies:

    go get "github.com/spf13/pflag" # CLI keys
    go get "github.com/pelletier/go-toml"      # Actual TOML parser
    go get "github.com/gvalkov/golang-evdev"   # Keyboard and mouse events
    go get "github.com/micmonay/keybd_event"   # Virtual keyboard !!(must be improved to deal with complex input)
    go get "github.com/kballard/go-shellquote" # joining/splitting strings using sh's word-splitting rules

Unfortunately my pull request https://github.com/micmonay/keybd_event/pull/32 still stays unaccepted.  
You must put "src/keybd_event/keybd_linux_export.go" inside "keybd_event" sources for successful assembly.  
You must also move "embeddedConfig" and "exec" internal libraries under Your "src/xswitcher" to satisfy

    import (
    "xswitcher/embeddedConfig"
    "xswitcher/exec"
    )
declaration.

Now You are ready to build. I do the portable static build using the string below:

    go build -o xswitcher -ldflags "-s -w" --tags static_all src/*.go && chmod +xs xswitcher

Put the config to "/etc/xswitcher/xswitcher.conf" (or just use embedded one)
and install xswitcher executable under "/usr/local/bin/" or where You prefer.

Make it executable, e.g. "chmod +xs /usr/local/bin/xswitcher", configure the autostart inside Your X GUI and enjoy.

## Wayland
To deal with wayland-based GUI, (at least) there must be some driver to provide "/dev/input" character devices.  
I don't plan to write something for wayland, and also don't know any ready-made solution.  
So, feel free to do this job. In case of such a driver appears, I of course will adapt (or help to adapt) xswitcher.

## Packaging
I don't have enough time to maintain any distro package (rpm, dpkg, etc.).  
But it seems to be a good deed. So, everybody who is ready to do this job is welcome.

## Conclusion
Please, don't be shy to report the bug if something in this README looks unclear.
