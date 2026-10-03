package main

// notify.go — daemon-side glue for the Punto Phase 4 feedback channels:
// sound events pushed over IPC (the GUI plays them, root never touches
// the sound server) and the opt-in typed-text diary.

import (
	"fmt"

	"xswitcher/internal/diary"
	"xswitcher/internal/ipc"
)

// ipcSrv is assigned in serve() before connectEvents() starts the event
// goroutines, so the key-event path never races its write.
var ipcSrv *ipc.Server

// diaryW stays nil while [Diary] Enabled = false — nothing is recorded.
var diaryW *diary.Writer

// sendSound pushes one event kind to subscribed GUI clients. Free when
// nobody listens: Broadcast over an empty subscription set is a no-op.
func sendSound(name string) {
	if ipcSrv != nil {
		ipcSrv.Broadcast(ipc.Event{Event: "sound", Name: name})
	}
}

// initDiary (re)builds the writer from the parsed [Diary] section.
func initDiary() {
	if Diary.Enabled {
		diaryW = diary.New(Diary)
	} else {
		diaryW = nil
	}
}

// recordDiary appends one completed word. It takes the raw buffer rather than
// text because it sits on the keystroke path and the diary is opt-in: rendering
// the word costs an allocation per word typed even when nothing records it.
// The diary is user text, so failures must be loud in the log but never disturb
// typing.
func recordDiary(word t_keys) {
	if diaryW == nil {
		return
	}
	text := bufferToText(word)
	if text == "" {
		return
	}
	if err := diaryW.Write(text); err != nil && (*VERBOSE || *DEBUG) {
		fmt.Printf("Diary write failed: %v\n", err)
	}
}

// ipcGetDiary serves the "get_diary" command for the GUI viewer.
func ipcGetDiary() (string, error) {
	if !Diary.Enabled {
		return "", fmt.Errorf("diary is not enabled in the config")
	}
	// 512 KiB is plenty for a viewer tab and caps one socket line.
	return diary.Read(Diary.Path, 512*1024)
}
