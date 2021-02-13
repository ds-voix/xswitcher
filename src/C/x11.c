#include <X11/Xlib.h>        // (libX11-devel | libx11-dev) + (libXmu-devel | libxmu-dev >> WinUtil.h)
#include <X11/Xmu/WinUtil.h> // Query windows (focus, name, etc.)
#include <X11/XKBlib.h>      // Switch window language ("layout")

Bool xerror = False;

extern int handle_error(Display* display, XErrorEvent* error) {
  xerror = True;
  return 1;
}

void set_handle_error() {
  XSetErrorHandler(handle_error);
}
