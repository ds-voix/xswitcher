/* Two top-level windows with distinct WM_CLASS, and focus control by signals.
 *
 * The stand needs the X input focus to move while keys are being injected, because
 * xswitcher re-reads it on every key event. Without a window manager, XSetInputFocus
 * from a client is the only way to do that.
 *
 *   SIGUSR1 -> focus window B ("terminal")
 *   SIGUSR2 -> focus window A ("editor")
 *   SIGHUP  -> focus None, i.e. the transient state a real desktop shows during a switch
 */
#include <X11/Xlib.h>
#include <X11/Xutil.h>
#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
#include <signal.h>

static Display *d;
static Window wA, wB;
static volatile sig_atomic_t pending = 0;

static void on_signal(int sig) { pending = sig; }

static void set_focus(Window w) {
	XSetInputFocus(d, w, RevertToNone, CurrentTime);
	XSync(d, False);
}

static Window make(Window root, int x, const char *name, const char *class_) {
	XSetWindowAttributes attrs = {0};
	XClassHint *hint = XAllocClassHint();
	attrs.event_mask = ExposureMask | KeyPressMask | FocusChangeMask;
	attrs.override_redirect = True; /* no WM here, but keeps the window unmanaged */
	Window w = XCreateWindow(d, root, x, 60, 300, 200, 0, CopyFromParent, InputOutput,
	                          CopyFromParent, CWEventMask | CWOverrideRedirect, &attrs);
	XStoreName(d, w, name);
	hint->res_name = (char *)name;
	hint->res_class = (char *)class_;
	XSetClassHint(d, w, hint);
	XFree(hint);
	XMapRaised(d, w);
	XSync(d, False);
	return w;
}

int main(void) {
	d = XOpenDisplay(NULL);
	if (!d) { fprintf(stderr, "mkwin: no display\n"); return 2; }
	Window root = DefaultRootWindow(d);
	wA = make(root, 10, "editor", "Testeditor");
	wB = make(root, 330, "terminal", "Testterminal");

	struct sigaction sa = {0};
	sa.sa_handler = on_signal;
	sigaction(SIGUSR1, &sa, NULL);
	sigaction(SIGUSR2, &sa, NULL);
	sigaction(SIGHUP, &sa, NULL);

	set_focus(wA);
	fprintf(stderr, "mkwin: focused editor (0x%lx)\n", wA);

	for (;;) {
		sig_atomic_t s = pending;
		pending = 0;
		if (s == SIGUSR1) { set_focus(wB); fprintf(stderr, "mkwin: focused terminal\n"); }
		else if (s == SIGUSR2) { set_focus(wA); fprintf(stderr, "mkwin: focused editor\n"); }
		else if (s == SIGHUP) { set_focus(None); fprintf(stderr, "mkwin: focused None\n"); }
		usleep(50000);
	}
	return 0;
}
