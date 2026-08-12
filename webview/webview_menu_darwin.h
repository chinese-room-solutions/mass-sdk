// Main-menu installer for the macOS webview backend.
//
// Implemented in webview_menu_darwin.m as a single Objective-C translation
// unit, for the same reason as the tray and nav helpers: defining the class in
// a cgo preamble emits it into more than one object file and the macOS linker
// rejects the duplicate Objective-C class symbols.
#ifndef MASS_WEBVIEW_MENU_DARWIN_H
#define MASS_WEBVIEW_MENU_DARWIN_H

// installMainMenu gives the process the standard App/Edit/Window menus.
// win is an NSWindow* — the window the Quit item closes. Must run on the main
// thread, and before the window starts taking input.
void installMainMenu(void *win);

#endif // MASS_WEBVIEW_MENU_DARWIN_H
