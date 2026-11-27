/* Reads and changes the XKB layout group of the core keyboard.
 *
 * xswitcher's X11 path switches layouts with XkbLockGroup(), and its event gate compares
 * the group reported by XkbGetState() with the configured [ActionKeys] Layouts. The stand
 * needs the same view from the outside, otherwise the whole X11 branch is unobservable.
 *
 *   xgroup get                  -> "group <n> locked <n> base <n>"
 *   xgroup set <n>              -> "locked <n> group <m>"   (m is what the server kept)
 *   xgroup get until <n> [SEC]  -> polls until the effective group is <n> (default 5s)
 *   xgroup info [keycode]       -> "groups <n> key <kc> groups <n>"
 *
 * "groups" is what the compiled keymap declares: XkbLockGroup() cannot reach a group the
 * map does not have, so the stand has to check it before blaming the daemon. Exit code is
 * 0 only when the requested group is reached.
 *
 * Group numbers are the 0-based XkbStateRec.group, which is what xswitcher compares the
 * configured [ActionKeys] Layouts against. Measured on Xvfb with a "us,ru,de" map: set 0, 1
 * and 2 are accepted and locked_group follows the request, set 3 is refused (XkbBadMatch).
 * Reading the state back needs a server that keeps its map between clients, so Xvfb has to
 * run with -noreset; see the phase-12 comment in stand/run.sh.
 */
#include <X11/Xlib.h>
#include <X11/XKBlib.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

static Display *open_display(void) {
	Display *d = XOpenDisplay(NULL); /* $DISPLAY */
	if (!d) {
		fprintf(stderr, "xgroup: cannot open display\n");
		exit(2);
	}
	int op = 0, evt = 0, major = 0, minor = 0;
	/* The Xkb extension must be negotiated before its requests are usable. */
	if (!XkbQueryExtension(d, &op, &evt, &major, &major, &minor)) {
		fprintf(stderr, "xgroup: the X server has no XKB extension\n");
		exit(2);
	}
	return d;
}

static void read_state(Display *d, XkbStateRec *state) {
	memset(state, 0, sizeof(*state));
	if (XkbGetState(d, XkbUseCoreKbd, state) != Success) {
		fprintf(stderr, "xgroup: XkbGetState failed\n");
		exit(2);
	}
}

static int group_count(Display *d, int keycode) {
	XkbDescPtr xkb = XkbGetKeyboard(d, XkbAllComponentsMask, XkbUseCoreKbd);
	if (!xkb) {
		fprintf(stderr, "xgroup: XkbGetKeyboard failed\n");
		exit(2);
	}
	int max_groups = 1, for_key = -1;
	for (int kc = xkb->min_key_code; kc <= xkb->max_key_code; kc++) {
		int n = XkbKeyNumGroups(xkb, (KeyCode)kc);
		if (n > max_groups) { max_groups = n; }
		if (kc == keycode) { for_key = n; }
	}
	XkbFreeKeyboard(xkb, XkbAllComponentsMask, True);

	/* The server keeps the number of groups of the keyboard in its controls, and that is
	 * what XkbLockGroup() clamps against -- a map with two groups per key is not enough. */
	XkbDescPtr ctrls = XkbAllocKeyboard();
	int ctrls_groups = -1;
	if (ctrls && XkbGetControls(d, XkbGroupsWrapMask, ctrls) == Success && ctrls->ctrls) {
		ctrls_groups = ctrls->ctrls->num_groups;
	}
	if (ctrls) { XkbFreeKeyboard(ctrls, 0, True); }

	printf("groups %d ctrls %d", max_groups, ctrls_groups);
	if (for_key >= 0) {
		printf(" key %d groups %d", keycode, for_key);
	}
	printf("\n");
	return max_groups;
}

int main(int argc, char **argv) {
	if (argc < 2) {
		fprintf(stderr, "usage: xgroup get|set <n>|get until <n> [sec]|info [keycode]\n");
		return 2;
	}
	Display *d = open_display();

	if (!strcmp(argv[1], "info")) {
		int kc = argc >= 3 ? atoi(argv[2]) : -1;
		return group_count(d, kc) > 0 ? 0 : 1;
	}

	if (!strcmp(argv[1], "set")) {
		if (argc < 3) { fprintf(stderr, "xgroup: set needs a group number\n"); return 2; }
		int want = atoi(argv[2]);
		if (!XkbLockGroup(d, XkbUseCoreKbd, want)) {
			fprintf(stderr, "xgroup: XkbLockGroup(%d) refused by the server\n", want);
			return 1;
		}
		XSync(d, False);
		XkbStateRec state;
		read_state(d, &state);
		printf("locked %d group %d\n", want, state.group);
		return state.group == want ? 0 : 1;
	}

	if (!strcmp(argv[1], "get")) {
		XkbStateRec state;
		read_state(d, &state);
		if (argc >= 3 && !strcmp(argv[2], "until")) {
			if (argc < 4) { fprintf(stderr, "xgroup: get until needs a group number\n"); return 2; }
			int want = atoi(argv[3]);
			int sec = argc >= 5 ? atoi(argv[4]) : 5;
			for (int i = 0; i < sec * 10; i++) {
				read_state(d, &state);
				if ((int)state.group == want) {
					printf("group %d\n", want);
					return 0;
				}
				usleep(100000);
			}
			printf("group %d (wanted %d)\n", state.group, want);
			return 1;
		}
		printf("group %d locked %u base %u\n", state.group, state.locked_group, state.base_group);
		return 0;
	}

	fprintf(stderr, "xgroup: unknown command \"%s\"\n", argv[1]);
	return 2;
}
