// ninekeypad programs Vaydeer macro keypads (the 9-key one is tested; the 4-key one is not).
// It starts a small web page on this computer and prints its address. It writes files only
// into its own folder.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"ninekeypad/internal/app"
	"ninekeypad/internal/hid"
	"ninekeypad/internal/layout"
	"ninekeypad/internal/vaydeer"
)

func main() {
	var (
		list      = flag.Bool("list", false, "list the keypad's USB interfaces and exit (for troubleshooting)")
		read      = flag.Bool("read", false, "print the keypad's layers as JSON and exit")
		keepalive = flag.Bool("keepalive", false, "Linux: keep the keypad's keys working without the page (runs until Ctrl+C)")
		port      = flag.Int("port", 0, "port for the page, not saved (default: the port saved in "+app.SettingsFile+")")
		dir       = flag.String("dir", "", "folder for layouts and backups (default: the program's own folder)")
	)
	flag.Parse()
	log.SetFlags(0)

	switch {
	case *list:
		exit(listInterfaces())
	case *read:
		exit(printLayout())
	case *keepalive:
		exit(keepAlive())
	}

	root := *dir
	if root == "" {
		root = programFolder()
	}
	fmt.Printf("ninekeypad %s\n\n", app.Version)
	if err := serve(root, *port); err != nil {
		// Started by double-click, the window would close before the message can be read.
		fmt.Printf("\nError: %v\n\nPress Enter to close.\n", err)
		bufio.NewScanner(os.Stdin).Scan()
		os.Exit(1)
	}
}

// serve runs the page on the port and token saved next to the program, so the address
// stays the same and can be bookmarked. portFlag (for tests) overrides the saved port.
func serve(root string, portFlag int) error {
	settings, changed := app.LoadSettings(root)
	want := settings.Port
	if portFlag != 0 {
		want = portFlag
	}
	srv := app.New(layout.Store{Root: root}, settings.Token)
	ln, got, err := srv.Listen(want)
	if err != nil {
		if app.AlreadyRunning(want, settings.Token) {
			url := app.PageURL(want, settings.Token)
			fmt.Printf("ninekeypad is already running. Its page is at:\n\n  %s\n\n"+
				"Press Enter to copy the address and close this window.\n", url)
			if bufio.NewScanner(os.Stdin).Scan() {
				if copyToClipboard(url) == nil {
					fmt.Println("Copied.")
					time.Sleep(time.Second)
				}
			}
			return nil
		}
		if portFlag != 0 {
			return fmt.Errorf("port %d is in use by another program", portFlag)
		}
		// Another program took our port: move to a free one, with a new secret key (the old
		// one may have been seen by that program), and remember both.
		srv = app.New(layout.Store{Root: root}, app.NewToken())
		if ln, got, err = srv.Listen(0); err != nil {
			return err
		}
		settings.Token, changed = srv.Token(), true
		fmt.Printf("NOTE: port %d is used by another program, so the address has changed.\n"+
			"      Please update your bookmark.\n\n", want)
	}
	if portFlag == 0 && (changed || got != settings.Port) {
		settings.Port = got
		if err := app.SaveSettings(root, settings); err != nil {
			fmt.Printf("NOTE: could not save %s (%v).\n      The address will change at the next start.\n\n", app.SettingsFile, err)
		}
	}
	url := app.PageURL(got, settings.Token)
	copyHint := "Press Enter here to copy the address, then paste it into the browser.\n\n"
	if runtime.GOOS == "linux" {
		copyHint = "Press Enter here to copy the address, then paste it into the browser.\n" +
			"To copy by hand: select it, then right-click > Copy. Ctrl+C here STOPS the program.\n\n"
	}
	fmt.Printf("Open this address in your browser (you can bookmark it, it stays the same):\n\n  %s\n\n"+
		copyHint+
		"Files are kept in: %s\n"+
		"Keep this window open (minimized is fine): it works the layer-switch keys.\n"+
		"Closing the page does not stop it. To stop: press Quit on the page, or close this window.\n\n", url, root)
	go copyOnEnter(url)
	go stopCleanly(srv)
	return srv.Serve(ln)
}

// stopCleanly lets a write to the keypad finish before the program ends on Ctrl+C or when
// its window is closed (Windows gives a few seconds for that), so the keypad is never left
// half-written by stopping the program.
func stopCleanly(srv *app.Server) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	if srv.Busy() {
		fmt.Println("\nFinishing the work on the keypad, please wait ...")
	}
	if !srv.WaitIdle(30 * time.Second) {
		fmt.Println("The keypad did not finish in time.")
	}
	fmt.Println("\nStopped. The page no longer works until you start the program again.")
	os.Exit(0)
}

// copyOnEnter copies the page address to the clipboard each time Enter is pressed.
// The program never opens a browser itself: it would often pick the wrong one.
func copyOnEnter(url string) {
	in := bufio.NewScanner(os.Stdin)
	for in.Scan() {
		if err := copyToClipboard(url); err != nil && runtime.GOOS == "linux" {
			fmt.Printf("Could not copy (%v).\nSelect the address above with the mouse, then right-click it > Copy,\n"+
				"or press Ctrl+Shift+C. (In a terminal, Ctrl+C does not copy: it stops this program.)\n", err)
		} else if err != nil {
			fmt.Printf("Could not copy (%v). Select the address above with the mouse and copy it.\n", err)
		} else {
			fmt.Println("Copied. Paste it into your browser's address bar.")
		}
	}
}

func exit(err error) {
	if err != nil {
		log.Println("Error:", err)
		os.Exit(1)
	}
	os.Exit(0)
}

// programFolder is where the executable lives: the only place the program writes.
func programFolder() string {
	exe, err := os.Executable()
	if err != nil {
		log.Fatal("cannot find the program's folder: ", err)
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	return filepath.Dir(exe)
}

func listInterfaces() error {
	list, err := hid.Enumerate(vaydeer.VendorID, vaydeer.ProductID)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		fmt.Println("No Vaydeer keypad (USB 0483:5752) found.")
		return nil
	}
	for _, in := range list {
		fmt.Printf("interface %2d  usage page %04X  usage %02X  %s\n", in.Interface, in.UsagePage, in.Usage, in.Path)
	}
	return nil
}

func printLayout() error {
	c, err := vaydeer.Open()
	if errors.Is(err, hid.ErrPermission) && runtime.GOOS == "linux" {
		return errors.New("no permission to use the keypad; start with: sudo ./ninekeypad --read")
	}
	if err != nil {
		return err
	}
	defer c.Close()
	s, err := c.ReadAll()
	if err != nil {
		return err
	}
	tested := "tested"
	if !vaydeer.KeyCounts[s.Info.Keys] {
		tested = "NOT tested"
	}
	fmt.Printf("%d-key keypad (%s), firmware %s, bootloader %s, %d of %d layers, layer %d active\n",
		s.Info.Keys, tested, s.Info.FirmwareString(), s.Info.BootloaderString(), s.Info.LayerCount, s.Info.MaxLayers, s.Info.ActiveLayer+1)
	out, _ := json.MarshalIndent(layout.FromSnapshot(s), "", "  ")
	fmt.Println(string(out))
	return nil
}

func keepAlive() error {
	fmt.Println("Keeping the keypad's keys working. Press Ctrl+C to stop.")
	fail := make(chan error, 1)
	go app.WatchKeys(
		func() { fmt.Println("Keypad connected.") },
		func(ev vaydeer.KeyEvent) {
			if ev.Down {
				fmt.Printf("key %d pressed\n", ev.Key+1)
			}
		},
		func(err error) {
			if errors.Is(err, hid.ErrPermission) {
				fail <- errors.New("no permission to use the keypad; start with: sudo ./ninekeypad --keepalive")
			}
		})
	return <-fail
}
