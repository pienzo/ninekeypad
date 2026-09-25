NINEKEYPAD - programmer for Vaydeer macro keypads (Linux)
=========================================================

Tested on Ubuntu 26.04.1 (x64) and on Windows with the Vaydeer 9-key keypad (model
JP-1011, firmware 1.1.5). Other Linux systems are untested.
The 4-key keypad is supported but NOT tested. Not connected to Vaydeer.

Start (in a terminal, in this folder):
   sudo ./ninekeypad
   "sudo" is needed because Linux lets only the administrator talk to USB devices
   directly. The program prints the address of the keypad page.
   Press Enter to copy the address (needs wl-copy, xclip or xsel; a standard Ubuntu has
   none of them), then paste it into your browser's address bar.
   To copy by hand: select the address with the mouse, then right-click > Copy.
   In the terminal, Ctrl+C does NOT copy: it stops the program.

If you see "Permission denied" for ./ninekeypad itself, the file lost its "program" mark
while copying. On a normal disk, fix it once with:
   chmod +x ninekeypad
On a FAT32 USB stick this does not work (FAT32 has no such mark). Copy the folder to
your home folder first, or use a stick formatted as exFAT, NTFS or ext4.

Stop: press "Quit" on the page, or press Ctrl+C in the terminal.
   Closing the page does NOT stop the program: it keeps running because it works the
   layer-switch keys. If a write is running, Ctrl+C waits for it to finish.

Bookmark: the address stays the same at every start, so you can bookmark the page.
   Start the program first, then open the bookmark.

Layers: the keypad stores up to 6 layers but cannot switch between them by itself.
   Give a key the setting "Layer switch" (in every layer). While the program runs,
   pressing that key switches the layer. "Use on keypad" on the page switches too.

Portable: the program installs nothing and writes only into this folder:
   backups/                 the whole keypad, saved automatically before every write
   layouts/                 layouts you save with "Save selected layer"
   ninekeypad-settings.json the page address (port and secret key), readable only by you.
                            Delete it to get a new address.

Keys not working on Linux?
   The tested 9-key keypad types nothing on Linux until a program reads its key presses.
   While ninekeypad runs, it does that. Without it:
      sudo ./ninekeypad --keepalive
   (keeps the keys working until you press Ctrl+C)

Troubleshooting (please send the output if something does not work):
   sudo ./ninekeypad --list    shows the keypad's USB parts
   sudo ./ninekeypad --read    prints the keypad's layers

No warranty: the program writes to the keypad's memory. It makes a backup first and
checks every write, but use it at your own risk.
