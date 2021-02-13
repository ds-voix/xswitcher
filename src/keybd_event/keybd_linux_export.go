package keybd_event
// Extra API for xswitcher, by PnD!
// I'll try to commit to keybd_event, a little bit later.
// Right now, just put this to the folder with "keybd_event" sources. To success the build.
func (k *KeyBonding) Down(key uint16) error { return downKey(int(key)) }
func (k *KeyBonding) Up(key uint16) error { return upKey(int(key)) }
func (k *KeyBonding) Sync() error { return sync() }
