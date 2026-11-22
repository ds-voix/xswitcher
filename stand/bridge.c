// === stand/bridge.c ===
// The stand's "hand": replays the daemon's own virtual keyboard into X.
//
// The daemon reads evdev and writes evdev (its uinput keyboard, named "keybd interface"),
// and Xvfb attaches to no evdev device -- measured in G2: three taps straight into the
// source node leave an X client seeing nothing at all. So a GUI window can only observe a
// real switch if the daemon's output is fed back through XTEST.
//
// The replay is code-only: evdev KEY_* becomes the X keycode by the fixed offset of 8
// (G1, measured on eight codes and documented as "kernel keycode + 8"). Layout is not
// touched: the daemon owns it through XkbLockGroup, and the same code already decodes to
// 'a' in group 1 and to Cyrillic_ef in group 2.
//
//   build: gcc stand/bridge.c -o /tmp/bridge -lXtst -lX11
//   usage: DISPLAY=:96 /tmp/bridge --name "keybd interface" [--print]
//          DISPLAY=:96 /tmp/bridge /dev/input/event1 [--print]
#include <dirent.h>
#include <errno.h>
#include <fcntl.h>
#include <linux/input.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#include <X11/Xlib.h>
#include <X11/extensions/XTest.h>

#define EVDEV_TO_XKEYCODE(c) ((c) + 8)

/* The event node of a uinput device is /dev/input/eventN with N assigned by the kernel, so
 * the stand looks the node up by the device name it created. */
static int find_node_by_name(const char *name, char *out, size_t out_size)
{
	DIR *dir = opendir("/sys/class/input");
	if (!dir)
		return -1;

	char path[512];
	int found = -1;
	struct dirent *de;
	while ((de = readdir(dir))) {
		if (strncmp(de->d_name, "event", 5) != 0)
			continue;
		snprintf(path, sizeof(path), "/sys/class/input/%s/device/name", de->d_name);
		FILE *f = fopen(path, "r");
		if (!f)
			continue;
		char real[256];
		if (fgets(real, sizeof(real), f)) {
			real[strcspn(real, "\n")] = '\0';
			if (strcmp(real, name) == 0) {
				snprintf(out, out_size, "/dev/input/%s", de->d_name);
				found = 0;
			}
		}
		fclose(f);
		if (found == 0)
			break;
	}
	closedir(dir);
	return found;
}

int main(int argc, char **argv)
{
	const char *dev = NULL;
	const char *name = NULL;
	int print = 0;

	for (int i = 1; i < argc; i++) {
		if (strcmp(argv[i], "--name") == 0 && i + 1 < argc)
			name = argv[++i];
		else if (strcmp(argv[i], "--print") == 0)
			print = 1;
		else if (argv[i][0] != '-')
			dev = argv[i];
		else {
			fprintf(stderr, "bridge: unknown option %s\n", argv[i]);
			return 9;
		}
	}

	char node[256];
	if (!dev && name) {
		if (find_node_by_name(name, node, sizeof(node)) != 0) {
			fprintf(stderr, "bridge: no /dev/input/event* named \"%s\"\n", name);
			return 9;
		}
		dev = node;
	}
	if (!dev) {
		fprintf(stderr, "usage: bridge [--name \"keybd interface\"|/dev/input/eventN] [--print]\n");
		return 9;
	}

	Display *dpy = XOpenDisplay(NULL);
	if (!dpy) {
		fprintf(stderr, "bridge: cannot open display (DISPLAY=%s)\n", getenv("DISPLAY"));
		return 9;
	}
	int ev_base, err_base, maj, minor;
	if (!XTestQueryExtension(dpy, &ev_base, &err_base, &maj, &minor)) {
		fprintf(stderr, "bridge: no XTEST extension on this server\n");
		return 9;
	}

	int fd = open(dev, O_RDONLY);
	if (fd < 0) {
		fprintf(stderr, "bridge: open %s: %s\n", dev, strerror(errno));
		return 9;
	}

	fprintf(stderr, "bridge: %s -> XTEST keycode+8 on %s\n", dev, XDisplayString(dpy));

	struct input_event ev;
	ssize_t r;
	while ((r = read(fd, &ev, sizeof(ev))) == sizeof(ev)) {
		if (ev.type == EV_SYN) {
			XFlush(dpy);
			continue;
		}
		if (ev.type != EV_KEY)
			continue;
		/* KEY_RESERVED carries no key; every other evdev code maps by the measured +8. */
		if (ev.code == 0)
			continue;

		int keycode = EVDEV_TO_XKEYCODE(ev.code);
		XTestFakeKeyEvent(dpy, (KeyCode)keycode, ev.value != 0, 0);
		if (print) {
			printf("evdev=%-3d keycode=%-3d %s\n", ev.code, keycode,
			       ev.value ? (ev.value == 2 ? "repeat" : "down") : "up");
			fflush(stdout);
		}
	}

	if (r < 0)
		perror("bridge: read");
	else
		fprintf(stderr, "bridge: %s stopped (short read)\n", dev);
	close(fd);
	XCloseDisplay(dpy);
	return 0;
}
