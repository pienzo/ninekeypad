#!/usr/bin/env python3

import re
import sys
from pathlib import Path

from PySide6.QtCore import QProcess, QUrl
from PySide6.QtGui import QAction, QIcon
from PySide6.QtWidgets import (
    QApplication,
    QMainWindow,
    QMessageBox,
    QMenu,
    QStyle,
    QSystemTrayIcon,
)
from PySide6.QtWebEngineWidgets import QWebEngineView


START_MINIMIZED = "--minimized" in sys.argv
if START_MINIMIZED:
    sys.argv.remove("--minimized")


class NineKeypadWindow(QMainWindow):
    def __init__(self):
        super().__init__()

        self.quitting = False
        self.url_loaded = False
        self.output_buffer = ""

        self.setWindowTitle("NineKeypad")
        self.resize(1180, 820)

        self.web = QWebEngineView(self)
        self.setCentralWidget(self.web)

        self.setup_tray()
        self.start_backend()

    def setup_tray(self):
        icon = QIcon.fromTheme("fcitx-unikey")

        if icon.isNull():
            icon = self.style().standardIcon(
                QStyle.StandardPixmap.SP_ComputerIcon
            )

        self.setWindowIcon(icon)

        self.tray = QSystemTrayIcon(icon, self)
        self.tray.setToolTip("NineKeypad")

        menu = QMenu()

        self.action_show = QAction("Apri NineKeypad", self)
        self.action_show.triggered.connect(self.show_window)
        menu.addAction(self.action_show)

        self.action_hide = QAction("Nascondi", self)
        self.action_hide.triggered.connect(self.hide)
        menu.addAction(self.action_hide)

        menu.addSeparator()

        self.action_quit = QAction("Esci", self)
        self.action_quit.triggered.connect(self.quit_app)
        menu.addAction(self.action_quit)

        self.tray.setContextMenu(menu)
        self.tray.activated.connect(self.tray_activated)
        self.tray.show()

    def start_backend(self):
        repo = Path(__file__).resolve().parent.parent
        binary = repo / "build" / "ninekeypad"
        data_dir = Path.home() / ".local" / "share" / "ninekeypad"

        data_dir.mkdir(parents=True, exist_ok=True)

        if not binary.is_file():
            QMessageBox.critical(
                self,
                "NineKeypad",
                f"Backend non trovato:\n{binary}",
            )
            QApplication.quit()
            return

        self.backend = QProcess(self)
        self.backend.setProgram(str(binary))
        self.backend.setArguments(["--dir", str(data_dir)])
        self.backend.setWorkingDirectory(str(repo))

        self.backend.readyReadStandardOutput.connect(
            self.read_backend_output
        )
        self.backend.readyReadStandardError.connect(
            self.read_backend_error
        )
        self.backend.errorOccurred.connect(self.backend_error)

        self.backend.start()

    def read_backend_output(self):
        data = bytes(
            self.backend.readAllStandardOutput()
        ).decode("utf-8", errors="replace")

        print(data, end="")
        self.output_buffer += data

        if self.url_loaded:
            return

        pattern = (
            r"http:"
            + r"//127[.]0[.]0[.]1:\d+/\?t=[0-9a-fA-F]+"
        )

        match = re.search(pattern, self.output_buffer)

        if match:
            self.url_loaded = True
            self.web.setUrl(QUrl(match.group(0)))

    def read_backend_error(self):
        data = bytes(
            self.backend.readAllStandardError()
        ).decode("utf-8", errors="replace")

        print(data, end="", file=sys.stderr)

    def backend_error(self, error):
        QMessageBox.critical(
            self,
            "NineKeypad backend",
            f"Errore nell'avvio del backend:\n{error}",
        )

    def show_window(self):
        self.show()
        self.raise_()
        self.activateWindow()

    def tray_activated(self, reason):
        if reason == QSystemTrayIcon.ActivationReason.Trigger:
            if self.isVisible():
                self.hide()
            else:
                self.show_window()

    def quit_app(self):
        self.quitting = True

        if (
            hasattr(self, "backend")
            and self.backend.state()
            != QProcess.ProcessState.NotRunning
        ):
            self.backend.terminate()

            if not self.backend.waitForFinished(3000):
                self.backend.kill()
                self.backend.waitForFinished(1000)

        self.tray.hide()
        QApplication.quit()

    def closeEvent(self, event):
        if self.quitting:
            event.accept()
            return

        event.ignore()
        self.hide()

        self.tray.showMessage(
            "NineKeypad",
            "NineKeypad continua a funzionare nel vassoio di sistema.",
            QSystemTrayIcon.MessageIcon.Information,
            2500,
        )


app = QApplication(sys.argv)
app.setApplicationName("NineKeypad")
app.setQuitOnLastWindowClosed(False)

window = NineKeypadWindow()

if not START_MINIMIZED:
    window.show()

sys.exit(app.exec())
