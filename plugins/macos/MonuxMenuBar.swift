import AppKit
import Foundation
import Darwin

struct MonuxInput: Codable, Equatable {
    let name: String
    let value: String
    let connector: String
}

struct MonuxMenuCache: Codable, Equatable {
    let configPath: String
    let inputs: [MonuxInput]
    let currentLabel: String
    let currentName: String?
    let currentValue: String?
    let updatedAt: Date
}

struct CommandResult {
    let status: Int32
    let standardOutput: String
    let standardError: String

    var succeeded: Bool { status == 0 }

    var errorDescription: String {
        let detail = standardError.trimmingCharacters(in: .whitespacesAndNewlines)
        if !detail.isEmpty { return detail }
        let output = standardOutput.trimmingCharacters(in: .whitespacesAndNewlines)
        if !output.isEmpty { return output }
        return "monux exited with status \(status)"
    }
}

enum MonuxOutputParser {
    private static let inputPattern = try! NSRegularExpression(
        pattern: #"^(0x[0-9a-fA-F]+)\s+(.+?)\s+(yes|no|unknown)\s+(yes|no|unknown)(?:\s+(.*))?$"#
    )
    private static let namedStatusPattern = try! NSRegularExpression(
        pattern: #"^(.+)\s+\((0x[0-9a-fA-F]+)\)$"#
    )

    static func inputs(_ output: String) -> [MonuxInput] {
        var inputs: [MonuxInput] = []
        for rawLine in output.split(whereSeparator: \Character.isNewline) {
            let line = String(rawLine).trimmingCharacters(in: .whitespaces)
            let range = NSRange(line.startIndex..<line.endIndex, in: line)
            guard let match = inputPattern.firstMatch(in: line, range: range),
                  let value = capture(1, from: match, in: line),
                  let connector = capture(2, from: match, in: line),
                  let names = capture(5, from: match, in: line) else {
                continue
            }
            for name in names.split(separator: ",") {
                let trimmed = name.trimmingCharacters(in: .whitespaces)
                if !trimmed.isEmpty {
                    inputs.append(MonuxInput(name: trimmed, value: value.lowercased(), connector: connector))
                }
            }
        }
        return inputs.sorted {
            $0.name.localizedCaseInsensitiveCompare($1.name) == .orderedAscending
        }
    }

    static func status(_ output: String) -> (label: String, name: String?, value: String?)? {
        let line = output.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !line.isEmpty else { return nil }
        let range = NSRange(line.startIndex..<line.endIndex, in: line)
        if let match = namedStatusPattern.firstMatch(in: line, range: range),
           let name = capture(1, from: match, in: line),
           let value = capture(2, from: match, in: line) {
            return (line, name, value.lowercased())
        }
        if line.range(of: #"^0x[0-9a-fA-F]+$"#, options: .regularExpression) != nil {
            return (line.lowercased(), nil, line.lowercased())
        }
        return (line, nil, nil)
    }

    private static func capture(_ index: Int, from match: NSTextCheckingResult, in value: String) -> String? {
        let range = match.range(at: index)
        guard range.location != NSNotFound, let swiftRange = Range(range, in: value) else { return nil }
        return String(value[swiftRange]).trimmingCharacters(in: .whitespaces)
    }
}

final class MonuxClient {
    private let executableURL: URL
    let configPath: String

    init(executableURL: URL, configPath: String) {
        self.executableURL = executableURL
        self.configPath = configPath
    }

    func run(_ arguments: [String]) -> CommandResult {
        guard FileManager.default.isExecutableFile(atPath: executableURL.path) else {
            return CommandResult(status: 127, standardOutput: "", standardError: "monux helper is missing from the application bundle")
        }

        let process = Process()
        let outputPipe = Pipe()
        let errorPipe = Pipe()
        process.executableURL = executableURL
        process.arguments = ["--config", configPath] + arguments
        process.standardOutput = outputPipe
        process.standardError = errorPipe

        do {
            try process.run()
            process.waitUntilExit()
            let output = String(decoding: outputPipe.fileHandleForReading.readDataToEndOfFile(), as: UTF8.self)
            let error = String(decoding: errorPipe.fileHandleForReading.readDataToEndOfFile(), as: UTF8.self)
            return CommandResult(status: process.terminationStatus, standardOutput: output, standardError: error)
        } catch {
            return CommandResult(status: 126, standardOutput: "", standardError: error.localizedDescription)
        }
    }
}

final class AppDelegate: NSObject, NSApplicationDelegate {
    private static let refreshInterval: TimeInterval = 5 * 60
    private static let cacheKey = "com.dyike.monux.menubar.cache.v1"

    private let client: MonuxClient
    private let defaults: UserDefaults
    private let worker = DispatchQueue(label: "com.dyike.monux.menubar.commands", qos: .userInitiated)
    private let menu = NSMenu()
    private var statusItem: NSStatusItem!
    private var refreshTimer: Timer?
    private var inputs: [MonuxInput] = []
    private var currentLabel = "Loading…"
    private var currentName: String?
    private var currentValue: String?
    private var lastError: String?
    private var statusError: String?
    private var busy = false

    init(client: MonuxClient, defaults: UserDefaults = .standard) {
        self.client = client
        self.defaults = defaults
        super.init()
    }

    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.setActivationPolicy(.accessory)
        statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.squareLength)
        if let button = statusItem.button {
            let image = NSImage(systemSymbolName: "arrow.left.arrow.right", accessibilityDescription: "Monux monitor input switcher")
            image?.isTemplate = true
            button.image = image
            if image == nil { button.title = "M" }
            button.toolTip = "Monux monitor input switcher"
        }
        menu.autoenablesItems = false
        statusItem.menu = menu
        let cachedAt = restoreCache()
        rebuildMenu()
        if cachedAt.map({ Date().timeIntervalSince($0) >= Self.refreshInterval }) ?? true {
            refresh(loadInputs: true)
        }
        refreshTimer = Timer.scheduledTimer(withTimeInterval: Self.refreshInterval, repeats: true) { [weak self] _ in
            self?.refresh(loadInputs: true)
        }
        refreshTimer?.tolerance = 30
    }

    func applicationWillTerminate(_ notification: Notification) {
        refreshTimer?.invalidate()
    }

    private func refresh(loadInputs: Bool) {
        guard !busy else { return }
        busy = true
        lastError = nil
        statusError = nil
        rebuildMenu()

        worker.async { [weak self] in
            guard let self else { return }
            let inputsResult = loadInputs ? self.client.run(["inputs"]) : nil
            let statusResult = self.client.run(["status"])
            DispatchQueue.main.async {
                var cacheChanged = false
                if let inputsResult {
                    if inputsResult.succeeded {
                        self.inputs = MonuxOutputParser.inputs(inputsResult.standardOutput)
                        cacheChanged = true
                    } else {
                        self.lastError = self.compact(inputsResult.errorDescription)
                    }
                }
                if statusResult.succeeded, let status = MonuxOutputParser.status(statusResult.standardOutput) {
                    self.currentLabel = status.label
                    self.currentName = status.name
                    self.currentValue = status.value
                    cacheChanged = true
                } else {
                    // DDC reads are unreliable on some Apple Silicon HDMI
                    // paths even though writes still work. Keep a successful
                    // selection visible and expose the read error only as a
                    // tooltip instead of turning routine polling into a warning.
                    if self.currentName == nil && self.currentValue == nil {
                        self.currentLabel = "Unavailable"
                    }
                    self.statusError = self.compact(statusResult.errorDescription)
                }
                self.busy = false
                if cacheChanged {
                    self.saveCache()
                }
                self.rebuildMenu()
            }
        }
    }

    @objc private func switchInput(_ sender: NSMenuItem) {
        guard let name = sender.representedObject as? String, !busy else { return }
        let previousLabel = currentLabel
        busy = true
        lastError = nil
        statusError = nil
        currentLabel = "Switching to \(name)…"
        rebuildMenu()

        worker.async { [weak self] in
            guard let self else { return }
            let result = self.client.run(["switch", name])
            DispatchQueue.main.async {
                self.busy = false
                if result.succeeded {
                    self.currentName = name
                    self.currentValue = self.inputs.first(where: { $0.name == name })?.value
                    self.currentLabel = self.currentValue.map { "\(name) (\($0))" } ?? name
                    self.saveCache()
                } else {
                    self.currentLabel = previousLabel
                    self.lastError = self.compact(result.errorDescription)
                }
                self.rebuildMenu()
            }
        }
    }

    @objc private func refreshNow(_ sender: Any?) {
        refresh(loadInputs: true)
    }

    @objc private func openConfiguration(_ sender: Any?) {
        let path = client.configPath
        if FileManager.default.fileExists(atPath: path) {
            NSWorkspace.shared.open(URL(fileURLWithPath: path))
        } else {
            lastError = "Configuration does not exist: \(path)"
            rebuildMenu()
        }
    }

    @objc private func quit(_ sender: Any?) {
        NSApp.terminate(nil)
    }

    private func rebuildMenu() {
        menu.removeAllItems()

        let current = NSMenuItem(title: "Current: \(currentLabel)", action: nil, keyEquivalent: "")
        current.isEnabled = false
        if let statusError {
            current.toolTip = "Current input could not be read: \(statusError)"
        }
        menu.addItem(current)

        if let lastError {
            let error = NSMenuItem(title: "Warning: \(lastError)", action: nil, keyEquivalent: "")
            error.isEnabled = false
            menu.addItem(error)
        }

        menu.addItem(.separator())
        if inputs.isEmpty {
            let empty = NSMenuItem(title: busy ? "Loading inputs…" : "No configured inputs", action: nil, keyEquivalent: "")
            empty.isEnabled = false
            menu.addItem(empty)
        } else {
            for input in inputs {
                let item = NSMenuItem(title: "\(input.name)  ·  \(input.connector)", action: #selector(switchInput(_:)), keyEquivalent: "")
                item.target = self
                item.representedObject = input.name
                item.isEnabled = !busy
                item.state = input.name == currentName || input.value == currentValue ? .on : .off
                menu.addItem(item)
            }
        }

        menu.addItem(.separator())
        let refresh = NSMenuItem(title: busy ? "Refreshing…" : "Refresh", action: #selector(refreshNow(_:)), keyEquivalent: "r")
        refresh.target = self
        refresh.isEnabled = !busy
        menu.addItem(refresh)

        let config = NSMenuItem(title: "Open Configuration", action: #selector(openConfiguration(_:)), keyEquivalent: ",")
        config.target = self
        menu.addItem(config)

        menu.addItem(.separator())
        let quit = NSMenuItem(title: "Quit Monux", action: #selector(quit(_:)), keyEquivalent: "q")
        quit.target = self
        menu.addItem(quit)
    }

    private func compact(_ value: String) -> String {
        let firstLine = value.split(whereSeparator: \Character.isNewline).first.map(String.init) ?? "unknown error"
        if firstLine.count <= 100 { return firstLine }
        return String(firstLine.prefix(97)) + "…"
    }

    @discardableResult
    private func restoreCache() -> Date? {
        guard let data = defaults.data(forKey: Self.cacheKey),
              let cache = try? JSONDecoder().decode(MonuxMenuCache.self, from: data),
              cache.configPath == client.configPath else {
            return nil
        }
        inputs = cache.inputs
        currentLabel = cache.currentLabel
        currentName = cache.currentName
        currentValue = cache.currentValue
        return cache.updatedAt
    }

    private func saveCache() {
        let cache = MonuxMenuCache(
            configPath: client.configPath,
            inputs: inputs,
            currentLabel: currentLabel,
            currentName: currentName,
            currentValue: currentValue,
            updatedAt: Date()
        )
        guard let data = try? JSONEncoder().encode(cache) else { return }
        defaults.set(data, forKey: Self.cacheKey)
    }
}

func configuredPath() -> String {
    if let environmentPath = ProcessInfo.processInfo.environment["MONUX_CONFIG"]?.trimmingCharacters(in: .whitespacesAndNewlines),
       !environmentPath.isEmpty {
        return NSString(string: environmentPath).expandingTildeInPath
    }
    if let resource = Bundle.main.url(forResource: "config-path", withExtension: nil),
       let value = try? String(contentsOf: resource, encoding: .utf8).trimmingCharacters(in: .whitespacesAndNewlines),
       !value.isEmpty {
        return NSString(string: value).expandingTildeInPath
    }
    return FileManager.default.homeDirectoryForCurrentUser
        .appendingPathComponent(".config/monux/config.yaml").path
}

func runSelfTests() -> Bool {
    let fixture = """
    VALUE  CONNECTOR      REPORTED  CURRENT  NAME
    0x0f   DisplayPort 1  yes       no       linux
    0x11   HDMI 1         yes       yes      mac,desk
    0x12   HDMI 2         yes       no
    """
    let parsed = MonuxOutputParser.inputs(fixture)
    let expected = [
        MonuxInput(name: "desk", value: "0x11", connector: "HDMI 1"),
        MonuxInput(name: "linux", value: "0x0f", connector: "DisplayPort 1"),
        MonuxInput(name: "mac", value: "0x11", connector: "HDMI 1"),
    ]
    guard parsed == expected else {
        fputs("input parser self-test failed: \(parsed)\n", stderr)
        return false
    }
    guard let named = MonuxOutputParser.status("mac (0x11)\n"), named.name == "mac", named.value == "0x11" else {
        fputs("named status parser self-test failed\n", stderr)
        return false
    }
    guard let raw = MonuxOutputParser.status("0x08\n"), raw.name == nil, raw.value == "0x08" else {
        fputs("raw status parser self-test failed\n", stderr)
        return false
    }
    let cache = MonuxMenuCache(
        configPath: "/tmp/monux.yaml",
        inputs: expected,
        currentLabel: "mac (0x11)",
        currentName: "mac",
        currentValue: "0x11",
        updatedAt: Date(timeIntervalSince1970: 1_700_000_000)
    )
    guard let cacheData = try? JSONEncoder().encode(cache),
          let decodedCache = try? JSONDecoder().decode(MonuxMenuCache.self, from: cacheData),
          decodedCache == cache else {
        fputs("menu cache self-test failed\n", stderr)
        return false
    }
    return true
}

if CommandLine.arguments.contains("--self-test") {
    exit(runSelfTests() ? 0 : 1)
}

let bundleURL = Bundle.main.bundleURL
let helperURL = bundleURL.appendingPathComponent("Contents/Helpers/monux")
let client = MonuxClient(executableURL: helperURL, configPath: configuredPath())
let application = NSApplication.shared
let delegate = AppDelegate(client: client)
application.delegate = delegate
application.run()
