NINEKEYPAD - programmer for Vaydeer macro keypads (Windows)
===========================================================

Tested only with the Vaydeer 9-key keypad (model JP-1011, firmware 1.1.5).
The 4-key keypad is supported but NOT tested. Not connected to Vaydeer.

Start:   double-click ninekeypad.exe
         A black window opens (the program). It shows the address of the keypad page.
         Press Enter in that window to copy the address, then paste it into the
         address bar of the browser you like.
         The program is not signed, so at the first start Windows may say "Windows
         protected your PC": click "More info", then "Run anyway".
Stop:    press "Quit" on the page, or close the black window.
         Closing the page does NOT stop the program: it keeps running (minimized is fine)
         because it works the layer-switch keys.
         Do not stop it while the page says "Writing..."; if you do, it finishes the
         write first (a few seconds).

Bookmark: the address stays the same at every start, so you can bookmark the page.
         Start ninekeypad.exe first, then open the bookmark.

Layers: the keypad stores up to 6 layers but cannot switch between them by itself.
         Give a key the setting "Layer switch" (in every layer). While ninekeypad.exe runs,
         pressing that key switches the layer. "Use on keypad" on the page switches too.

Portable: the program installs nothing and writes only into this folder:
   backups\                 the whole keypad, saved automatically before every write
   layouts\                 layouts you save with "Save selected layer"
   ninekeypad-settings.json the page address (port and secret key). Delete it to get a
                            new address; old bookmarks then stop working.

If the page says the keypad is in use: close the Vaydeer app (or a second copy of this
program), then press "Read from keypad".

For troubleshooting, in a terminal in this folder:
   ninekeypad.exe --list    shows the keypad's USB parts
   ninekeypad.exe --read    prints the keypad's layers

No warranty: the program writes to the keypad's memory. It makes a backup first and
checks every write, but use it at your own risk.
