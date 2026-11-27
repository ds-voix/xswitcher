// === stand/echo.c ===
// The stand's "eyes": what a real X client actually receives for one key.
//
// Xvfb attaches to no evdev device, so the daemon's own uinput keyboard never reaches X:
// XTEST is the only way in. This prints the delivered keycode (and its evdev equivalent),
// the keysym and its name, the Latin-1 decoding and the live Xkb group -- so the evdev ->
// XTEST offset and the per-group layout are measured instead of guessed. The keysym name
// is the readable form ("a" vs "Cyrillic_ef"); the image ships libX11-dev without
// X11/Xutf8.h, so no UTF-8 decoding is attempted here.
//
//   build: gcc stand/echo.c -o /tmp/echo -lX11 -lXtst
//   probe: DISPLAY=:96 /tmp/echo probe 30        # evdev KEY_A -> X keycode 38
//   listen: DISPLAY=:96 /tmp/echo                # tap into an already focused window
#include <X11/Xlib.h>
#include <X11/Xutil.h>
#include <X11/XKBlib.h>
#include <X11/keysym.h>
#include <X11/extensions/XTest.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

/* Measured in G1 (stand/gui-stand.sh): evdev KEY_A is 30 and X reports keycode 38. */
#define EVDEV_TO_XKEYCODE(c) ((c) + 8)

static void report(Display *dpy, XKeyEvent *ev, const char *src)
{
	KeySym ks = NoSymbol;
	char latin[32];
	int n = XLookupString(ev, latin, sizeof(latin) - 1, &ks, NULL);
	latin[n] = '\0';

	const char *name = XKeysymToString(ks);

	XkbStateRec st;
	XkbGetState(dpy, XkbUseCoreKbd, &st);

	printf("%s keycode=%-3d evdev=%-3d state=0x%04x keysym=0x%-6lx name=%-12s "
	       "latin=%-4s group=%d base=%d\n",
	       src, ev->keycode, ev->keycode - 8, ev->state, (unsigned long)ks,
	       name ? name : "-", n ? latin : "-", st.locked_group, st.base_group);
	fflush(stdout);
}

int main(int argc, char **argv)
{
	Display *dpy = XOpenDisplay(NULL);
	if (!dpy) {
		fprintf(stderr, "echo: cannot open display (is Xvfb up and DISPLAY set?)\n");
		return 9;
	}

	int ev_base, err_base, maj, minor;
	if (!XTestQueryExtension(dpy, &ev_base, &err_base, &maj, &minor)) {
		fprintf(stderr, "echo: no XTEST extension on this server\n");
		return 9;
	}

	int probe = -1;
	if (argc >= 3 && strcmp(argv[1], "probe") == 0)
		probe = atoi(argv[2]);

	Window root = DefaultRootWindow(dpy);
	Window win = XCreateSimpleWindow(dpy, root, 10, 10, 400, 120, 1,
	                                 BlackPixel(dpy, 0), WhitePixel(dpy, 0));
	XSelectInput(dpy, win, KeyPressMask);
	XMapWindow(dpy, win);
	XSync(dpy, False);
	XSetInputFocus(dpy, win, PointerRoot, CurrentTime);
	XFlush(dpy);

	if (probe >= 0) {
		XTestFakeKeyEvent(dpy, EVDEV_TO_XKEYCODE(probe), True, 0);
		XTestFakeKeyEvent(dpy, EVDEV_TO_XKEYCODE(probe), False, 0);
		XFlush(dpy);
	}

	for (;;) {
		XEvent ev;
		XNextEvent(dpy, &ev);
		if (ev.type == KeyPress)
			report(dpy, &ev.xkey, "key");
	}
}
