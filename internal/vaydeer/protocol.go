// Package vaydeer speaks the configuration protocol of Vaydeer macro keypads (USB 0483:5752).
// Tested with the 9-key "Smart Keypad"; the 4-key keypad uses the same protocol and USB ID
// according to other projects, but is untested here. Protocol facts: docs/protocol.md.
package vaydeer

import (
	"errors"
	"fmt"
	"runtime"
	"time"

	"ninekeypad/internal/hid"
)

const (
	VendorID  = 0x0483
	ProductID = 0x5752

	vendorUsagePage = 0xFF00
	CommandUsage    = 0x01 // interface 0: commands and answers
	EventUsage      = 0x02 // interface 2: key press reports

	cmdDeviceInfo = 0x60
	cmdWriteKey   = 0x61
	cmdReadKey    = 0x62
	cmdSetLayer   = 0x64
	cmdLayerName  = 0x65
	cmdCommit     = 0x66
	cmdReadName   = 0x67
	cmdFirmware   = 0xFC // never sent: a bad firmware write can brick the keypad
	cmdInit       = 0xFD

	startMarker = 0xFF
	endMarker   = 0xFE

	reportLen  = 65 // report ID + 64 bytes
	maxPayload = reportLen - 4
	chunkLen   = maxPayload - 1 // sequence byte + data
	timeout    = 2 * time.Second
)

var ErrNotFound = errors.New("keypad not found: is it plugged in?")

// KeyCounts are the keypads this program knows, with whether it was tested on a real one.
// The count comes from the keypad itself (device info). Other Vaydeer keypads (1 or 6 keys,
// knobs) are refused rather than guessed.
var KeyCounts = map[int]bool{9: true, 4: false}

// CheckKeyCount refuses key counts this program does not know.
func CheckKeyCount(n int) error {
	if _, ok := KeyCounts[n]; !ok {
		return fmt.Errorf("a keypad with %d keys is not supported; only the 9-key (tested) and the 4-key (untested) keypads are", n)
	}
	return nil
}

// BuildFrame makes one output report: [0x00, cmd, len, payload..., xor(cmd..payload), 0-padding].
func BuildFrame(cmd byte, payload []byte) ([]byte, error) {
	if cmd == cmdFirmware {
		return nil, errors.New("firmware commands are blocked")
	}
	if len(payload) > maxPayload {
		return nil, fmt.Errorf("payload too long: %d bytes (max %d)", len(payload), maxPayload)
	}
	frame := make([]byte, reportLen)
	frame[1], frame[2] = cmd, byte(len(payload))
	copy(frame[3:], payload)
	var x byte
	for _, b := range frame[1 : 3+len(payload)] {
		x ^= b
	}
	frame[3+len(payload)] = x
	return frame, nil
}

// ParseResponse checks an input report [cmd, len, status, data..., xor] and returns status and data.
func ParseResponse(cmd byte, resp []byte) (status byte, data []byte, err error) {
	if len(resp) < 4 {
		return 0, nil, fmt.Errorf("answer too short: % X", resp)
	}
	n := int(resp[1])
	if n < 1 || 3+n > len(resp) {
		return 0, nil, fmt.Errorf("answer has a bad length: % X", resp)
	}
	if resp[0] != cmd {
		return 0, nil, errWrongCommand // a stale answer: skipped, whatever its checksum
	}
	var x byte
	for _, b := range resp[:2+n] {
		x ^= b
	}
	if x != resp[2+n] {
		return 0, nil, fmt.Errorf("answer checksum is wrong: % X", resp[:3+n])
	}
	return resp[2], resp[3 : 2+n], nil
}

var errWrongCommand = errors.New("answer belongs to another command")

// Client runs commands on the keypad's command interface.
type Client struct {
	dev    hid.Device
	events hid.Device // Linux: the key-event interface, kept open while the client is (may be nil)
}

func NewClient(dev hid.Device) *Client { return &Client{dev: dev} }

// Open finds the keypad and opens its command interface (interface 0, usage page 0xFF00, usage 1)
// exclusively, so no other program's commands or answers can mix with ours.
//
// On Linux it also opens the key-event interface (interface 2) while the client is open. Linux
// fetches an interface's reports only while a program has it open, and an unfetched key press
// seems to stop the keypad from answering commands (seen once on Linux: after key presses with
// no program running, command 60 got no answer; after --keepalive had run, it worked).
func Open() (*Client, error) {
	path, err := FindInterface(CommandUsage)
	if err != nil {
		return nil, err
	}
	dev, err := hid.Open(path)
	if err != nil {
		return nil, err
	}
	c := NewClient(dev)
	if runtime.GOOS == "linux" {
		c.events = openEventDrain()
	}
	return c, nil
}

// openEventDrain opens the key-event interface read-only and shared (the page's key watcher may
// have it open too) and throws its reports away until it is closed. Errors are ignored: the
// commands then simply run as before.
func openEventDrain() hid.Device {
	path, err := FindInterface(EventUsage)
	if err != nil {
		return nil
	}
	dev, err := hid.OpenReader(path) // opening it lets Linux fetch a waiting key press
	if err != nil {
		return nil
	}
	go func() {
		for {
			if _, err := dev.Read(time.Minute); err != nil && err != hid.ErrTimeout {
				return // closed
			}
		}
	}()
	return dev
}

// FindInterface returns the path of the keypad's vendor collection with the given usage.
func FindInterface(usage uint16) (string, error) {
	list, err := hid.Enumerate(VendorID, ProductID)
	if err != nil {
		return "", err
	}
	var found []string
	for _, in := range list {
		if in.UsagePage == vendorUsagePage && in.Usage == usage {
			found = append(found, in.Path)
		}
	}
	switch len(found) {
	case 0:
		return "", ErrNotFound
	case 1:
		return found[0], nil
	default:
		return "", errors.New("more than one keypad is plugged in: please plug in only one")
	}
}

func (c *Client) Close() error {
	if c.events != nil {
		c.events.Close()
	}
	return c.dev.Close()
}

// Do sends one command and returns the answer's data. A non-zero device status is an error.
func (c *Client) Do(cmd byte, payload ...byte) ([]byte, error) {
	frame, err := BuildFrame(cmd, payload)
	if err != nil {
		return nil, err
	}
	if err := c.dev.Write(frame); err != nil {
		return nil, fmt.Errorf("sending command %02X: %w", cmd, err)
	}
	// Skip a few stale answers to an earlier command, if any.
	for tries := 0; tries < 8; tries++ {
		resp, err := c.dev.Read(timeout)
		if err != nil {
			return nil, fmt.Errorf("waiting for answer to %02X: %w", cmd, err)
		}
		status, data, err := ParseResponse(cmd, resp)
		if err == errWrongCommand {
			continue
		}
		if err != nil {
			return nil, err
		}
		if status != 0 {
			return nil, fmt.Errorf("keypad refused command %02X (status %d)", cmd, status)
		}
		return data, nil
	}
	return nil, fmt.Errorf("no matching answer to command %02X", cmd)
}

// Info is what command 0x60 reports.
type Info struct {
	Type        byte    `json:"type"`
	Keys        int     `json:"keys"`
	Firmware    [3]byte `json:"-"`
	Bootloader  [3]byte `json:"-"`
	ActiveLayer int     `json:"activeLayer"`
	LayerCount  int     `json:"layerCount"`
	MaxLayers   int     `json:"maxLayers"`
}

func (i Info) FirmwareString() string {
	return fmt.Sprintf("%d.%d.%d", i.Firmware[0], i.Firmware[1], i.Firmware[2])
}

func (i Info) BootloaderString() string {
	return fmt.Sprintf("%d.%d.%d", i.Bootloader[0], i.Bootloader[1], i.Bootloader[2])
}

func (c *Client) Info() (Info, error) {
	d, err := c.Do(cmdDeviceInfo)
	if errors.Is(err, hid.ErrTimeout) {
		// Info is the first command after opening. It only reads, so one more try is safe,
		// e.g. when the keypad was still busy with a key press nobody had fetched.
		d, err = c.Do(cmdDeviceInfo)
	}
	if err != nil {
		return Info{}, err
	}
	if len(d) < 11 {
		return Info{}, fmt.Errorf("device info too short: % X", d)
	}
	return Info{
		Type: d[0], Keys: int(d[1]),
		Firmware:    [3]byte{d[2], d[3], d[4]},
		Bootloader:  [3]byte{d[5], d[6], d[7]},
		ActiveLayer: int(d[8]), LayerCount: int(d[9]), MaxLayers: int(d[10]),
	}, nil
}

func (c *Client) LayerName(layer int) (string, error) {
	d, err := c.Do(cmdReadName, byte(layer))
	if err != nil {
		return "", err
	}
	return decodeUTF16BE(d), nil
}

// ReadKey reads one key: a header with type and label, then numbered data chunks until the end marker.
func (c *Client) ReadKey(layer, key int) (Key, error) {
	h, err := c.Do(cmdReadKey, startMarker, byte(layer), byte(key))
	if err != nil {
		return Key{}, err
	}
	if len(h) < 4 {
		return Key{}, fmt.Errorf("key %d header too short: % X", key, h)
	}
	var data []byte
	for seq := 0; seq < 16; seq++ {
		chunk, err := c.Do(cmdReadKey, byte(seq), byte(layer), byte(key))
		if err != nil {
			return Key{}, err
		}
		if len(chunk) == 0 {
			return Key{}, fmt.Errorf("key %d: empty data chunk", key)
		}
		if chunk[0] == endMarker {
			return DecodeKey(h[1], h[2], h[3], decodeUTF16BE(h[4:]), data), nil
		}
		data = append(data, chunk[1:]...)
	}
	return Key{}, fmt.Errorf("key %d: data did not end", key)
}

// Layer is one keypad layer: a name and one Key per physical key (9 or 4).
type Layer struct {
	Name string
	Keys []Key
}

// Snapshot is everything stored on the keypad.
type Snapshot struct {
	Info   Info
	Layers []Layer
}

// ReadLayer reads the name and the keys of one layer.
func (c *Client) ReadLayer(layer, keys int) (Layer, error) {
	l := Layer{Keys: make([]Key, keys)}
	var err error
	if l.Name, err = c.LayerName(layer); err != nil {
		return l, err
	}
	for k := range l.Keys {
		if l.Keys[k], err = c.ReadKey(layer, k); err != nil {
			return l, err
		}
	}
	return l, nil
}

// ReadAll reads the device info and every layer stored on the keypad.
func (c *Client) ReadAll() (Snapshot, error) {
	var s Snapshot
	info, err := c.Info()
	if err != nil {
		return s, err
	}
	if err := CheckKeyCount(info.Keys); err != nil {
		return s, err
	}
	if info.LayerCount < 1 || info.LayerCount > info.MaxLayers {
		return s, fmt.Errorf("the keypad reports %d layers (max %d); refusing to guess", info.LayerCount, info.MaxLayers)
	}
	s.Info = info
	for i := 0; i < info.LayerCount; i++ {
		l, err := c.ReadLayer(i, info.Keys)
		if err != nil {
			return s, fmt.Errorf("layer %d: %w", i+1, err)
		}
		s.Layers = append(s.Layers, l)
	}
	return s, nil
}

// SetActiveLayer makes the keypad use another of its stored layers.
func (c *Client) SetActiveLayer(layer int) error {
	_, err := c.Do(cmdSetLayer, byte(layer))
	return err
}

// WriteKey stores one key: header (type + label), data chunks, end marker.
func (c *Client) WriteKey(layer, key int, k Key) error {
	keyType, sub, data, err := k.Encode()
	if err != nil {
		return fmt.Errorf("key %d: %w", key, err)
	}
	head := append([]byte{startMarker, byte(layer), byte(key), keyType, sub, 0}, encodeUTF16BE(k.Label)...)
	if _, err := c.Do(cmdWriteKey, head...); err != nil {
		return err
	}
	for seq, off := 0, 0; off < len(data); seq, off = seq+1, off+chunkLen {
		end := min(off+chunkLen, len(data))
		if _, err := c.Do(cmdWriteKey, append([]byte{byte(seq % 16)}, data[off:end]...)...); err != nil {
			return err
		}
	}
	_, err = c.Do(cmdWriteKey, endMarker)
	return err
}

// WriteLayers replaces everything on the keypad with these layers (1 to maxLayers).
// Per layer, as the vendor app does it: layer name, its keys, commit. Every name and commit
// carries maxLayerIndex = the highest layer index in use; with one layer that is 0, which is
// confirmed on firmware 1.1.5.
func (c *Client) WriteLayers(layers []Layer, maxLayers int) error {
	if len(layers) < 1 || len(layers) > maxLayers {
		return fmt.Errorf("the keypad takes 1 to %d layers, not %d", maxLayers, len(layers))
	}
	keys := len(layers[0].Keys)
	if err := CheckKeyCount(keys); err != nil {
		return err
	}
	for i, l := range layers {
		if len(l.Keys) != keys {
			return fmt.Errorf("layer %d has %d keys, layer 1 has %d", i+1, len(l.Keys), keys)
		}
		for k, key := range l.Keys {
			if err := key.Validate(); err != nil {
				return fmt.Errorf("layer %d, key %d: %w", i+1, k+1, err)
			}
		}
	}
	maxLayerIndex := byte(len(layers) - 1)
	if _, err := c.Do(cmdInit); err != nil {
		return err
	}
	for i, l := range layers {
		layer := byte(i)
		if _, err := c.Do(cmdLayerName, append([]byte{layer, maxLayerIndex}, encodeUTF16BE(l.Name)...)...); err != nil {
			return fmt.Errorf("layer %d name: %w", i+1, err)
		}
		for k, key := range l.Keys {
			if err := c.WriteKey(i, k, key); err != nil {
				return fmt.Errorf("layer %d: %w", i+1, err)
			}
		}
		if _, err := c.Do(cmdCommit, layer, maxLayerIndex); err != nil {
			return fmt.Errorf("layer %d commit: %w", i+1, err)
		}
	}
	return nil
}

func encodeUTF16BE(s string) []byte {
	var out []byte
	for _, r := range s {
		if r > 0xFFFF {
			r = '?'
		}
		out = append(out, byte(r>>8), byte(r))
	}
	return out
}

func decodeUTF16BE(b []byte) string {
	var rs []rune
	for i := 0; i+1 < len(b); i += 2 {
		r := rune(b[i])<<8 | rune(b[i+1])
		if r == 0 {
			break
		}
		rs = append(rs, r)
	}
	return string(rs)
}
