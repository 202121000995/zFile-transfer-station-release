QT += widgets network
CONFIG += c++11 release
TEMPLATE = app
TARGET = transfer-qt-win7

DEFINES += WINVER=0x0601 _WIN32_WINNT=0x0601

SOURCES += main.cpp

win32 {
    QMAKE_LFLAGS += -Wl,-subsystem,windows
    RC_ICONS = ../tauri/src-tauri/icons/icon.ico
}
