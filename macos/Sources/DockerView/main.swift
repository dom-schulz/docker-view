import AppKit
import SwiftTerm

// MARK: - Theme (mirrors Ghostty `theme = light:my-light,dark:my-dark`)

struct Theme {
    let palette: [String]   // ANSI 0-15
    let background, foreground, cursor, selection: String
    let cursorText: String?

    static let dark = Theme(
        palette: ["#241f31", "#c01c28", "#2ec27e", "#f5c211", "#1e78e4", "#9841bb", "#0ab9dc", "#c0bfbc",
                  "#5e5c64", "#ed333b", "#57e389", "#f8e45c", "#51a1ff", "#c061cb", "#4fd2fd", "#f6f5f4"],
        background: "#1d1d20", foreground: "#ffffff", cursor: "#ffffff", selection: "#3d4a5c", cursorText: nil)

    static let light = Theme(
        palette: ["#454b58", "#f74840", "#25a343", "#cc8100", "#0087c1", "#b50da9", "#009ab7", "#c5b2b3",
                  "#0e131f", "#ff3711", "#00b90e", "#ec9900", "#1065de", "#e500d8", "#00b4dd", "#ffffff"],
        background: "#fafafa", foreground: "#383a43", cursor: "#1a1919", selection: "#454e5e", cursorText: "#dbdfe5")
}

func rgb(_ hex: String) -> (r: Int, g: Int, b: Int) {
    let v = Int(hex.dropFirst(), radix: 16) ?? 0
    return ((v >> 16) & 0xff, (v >> 8) & 0xff, v & 0xff)
}

func termColor(_ hex: String) -> SwiftTerm.Color {
    let c = rgb(hex)  // 8-bit -> 16-bit (x * 257 maps 0xff to 0xffff)
    return SwiftTerm.Color(red: UInt16(c.r * 257), green: UInt16(c.g * 257), blue: UInt16(c.b * 257))
}

func nsColor(_ hex: String) -> NSColor {
    let c = rgb(hex)
    return NSColor(srgbRed: CGFloat(c.r) / 255, green: CGFloat(c.g) / 255, blue: CGFloat(c.b) / 255, alpha: 1)
}

// MARK: - App

@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate, LocalProcessTerminalViewDelegate {
    var window: NSWindow!
    var terminal: LocalProcessTerminalView!
    var appearanceObserver: NSKeyValueObservation?

    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.mainMenu = makeMenu()

        window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 1200, height: 760),
                          styleMask: [.titled, .closable, .miniaturizable, .resizable],
                          backing: .buffered, defer: false)
        window.title = "DockerView"
        window.isReleasedWhenClosed = false
        window.center()
        window.setFrameAutosaveName("DockerViewMainWindow")

        terminal = LocalProcessTerminalView(frame: window.contentView!.bounds)
        terminal.autoresizingMask = [.width, .height]
        terminal.processDelegate = self
        terminal.font = NSFont(name: "JetBrainsMono-Regular", size: 13)
            ?? NSFont(name: "JetBrains Mono", size: 13)
            ?? NSFont.monospacedSystemFont(ofSize: 13, weight: .regular)
        terminal.setCursorStyle(.steadyBlock)
        window.contentView = terminal

        appearanceObserver = NSApp.observe(\.effectiveAppearance, options: [.initial, .new]) { [weak self] _, _ in
            Task { @MainActor in self?.applyTheme() }
        }
        applyTheme()

        window.makeKeyAndOrderFront(nil)
        window.makeFirstResponder(terminal)
        NSApp.activate(ignoringOtherApps: true)
        startDkrv()
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { true }

    func applyTheme() {
        let isDark = NSApp.effectiveAppearance.bestMatch(from: [.darkAqua, .aqua]) == .darkAqua
        let t = isDark ? Theme.dark : Theme.light
        terminal.installColors(t.palette.map(termColor))
        terminal.nativeBackgroundColor = nsColor(t.background)
        terminal.nativeForegroundColor = nsColor(t.foreground)
        terminal.caretColor = nsColor(t.cursor)
        terminal.caretTextColor = t.cursorText.map(nsColor)
        terminal.selectedTextBackgroundColor = nsColor(t.selection)
        window.backgroundColor = nsColor(t.background)
        terminal.needsDisplay = true
    }

    func startDkrv() {
        let exe = Bundle.main.bundleURL.appendingPathComponent("Contents/MacOS/dkrv").path
        guard FileManager.default.isExecutableFile(atPath: exe) else {
            fail("dkrv binary not found at \(exe). Rebuild with ./macos/build.sh.")
            return
        }
        var env = ProcessInfo.processInfo.environment
        let home = env["HOME"] ?? NSHomeDirectory()
        env["HOME"] = home
        if env["SHELL"] == nil { env["SHELL"] = "/bin/zsh" }
        if env["LANG"] == nil { env["LANG"] = "en_US.UTF-8" }
        env["TERM"] = "xterm-256color"
        env["COLORTERM"] = "truecolor"
        env["PATH"] = "/opt/homebrew/bin:/usr/local/bin:" + (env["PATH"] ?? "/usr/bin:/bin:/usr/sbin:/sbin")
        terminal.startProcess(executable: exe, args: [], environment: env.map { "\($0.key)=\($0.value)" },
                              execName: "dkrv", currentDirectory: home)
    }

    func fail(_ message: String) {
        let alert = NSAlert()
        alert.messageText = "DockerView"
        alert.informativeText = message
        alert.runModal()
        NSApp.terminate(nil)
    }

    func makeMenu() -> NSMenu {
        let main = NSMenu()
        let appItem = NSMenuItem(), editItem = NSMenuItem()
        main.addItem(appItem)
        main.addItem(editItem)

        let appMenu = NSMenu()
        appMenu.addItem(withTitle: "Hide DockerView", action: #selector(NSApplication.hide(_:)), keyEquivalent: "h")
        appMenu.addItem(.separator())
        appMenu.addItem(withTitle: "Quit DockerView", action: #selector(NSApplication.terminate(_:)), keyEquivalent: "q")
        appItem.submenu = appMenu

        let editMenu = NSMenu(title: "Edit")
        editMenu.addItem(withTitle: "Copy", action: #selector(NSText.copy(_:)), keyEquivalent: "c")
        editMenu.addItem(withTitle: "Paste", action: #selector(NSText.paste(_:)), keyEquivalent: "v")
        editMenu.addItem(withTitle: "Select All", action: #selector(NSText.selectAll(_:)), keyEquivalent: "a")
        editItem.submenu = editMenu
        return main
    }

    // MARK: LocalProcessTerminalViewDelegate

    func sizeChanged(source: LocalProcessTerminalView, newCols: Int, newRows: Int) {}

    func setTerminalTitle(source: LocalProcessTerminalView, title: String) {
        window.title = title.isEmpty ? "DockerView" : title
    }

    func hostCurrentDirectoryUpdate(source: TerminalView, directory: String?) {}

    func processTerminated(source: TerminalView, exitCode: Int32?) {
        NSApp.terminate(nil)
    }
}

MainActor.assumeIsolated {
    let app = NSApplication.shared
    let delegate = AppDelegate()
    app.delegate = delegate
    app.setActivationPolicy(.regular)
    withExtendedLifetime(delegate) { app.run() }
}
