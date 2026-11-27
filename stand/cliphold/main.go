// cliphold owns the CLIPBOARD selection with a fixed text until it is killed.
//
// The stand needs a real selection owner: clipboard.Read only returns bytes when some *other* client
// currently holds CLIPBOARD, and an owner that exits hands the selection over to nobody. The holder is
// written with the same library the daemon reads through, so the phase exercises the protocol rather
// than a hand-rolled approximation of it.
//
// Used as: /tmp/cliphold <text>   (stays in the foreground; kill it to release the selection)
package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"golang.design/x/clipboard"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: cliphold <text>")
		os.Exit(2)
	}
	if err := clipboard.Init(); err != nil {
		fmt.Printf("cliphold: %v\n", err)
		os.Exit(1)
	}
	lost := clipboard.Write(clipboard.FmtText, []byte(os.Args[1]))
	fmt.Printf("cliphold: holding %d bytes\n", len(os.Args[1]))

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
	select {
	case <-lost: // another client took the clipboard, so a read now sees someone else's text
		fmt.Println("cliphold: lost the selection")
		os.Exit(1)
	case <-signals:
	}
}
