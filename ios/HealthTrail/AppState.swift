import Foundation
import SwiftUI

enum L10n {
    static var isChinese: Bool {
        Locale.preferredLanguages.first?.lowercased().hasPrefix("zh") == true
    }

    static func pick(zh: String, en: String) -> String {
        isChinese ? zh : en
    }
}

enum ConnectionMode: String, CaseIterable, Identifiable {
    case local
    case remote

    var id: String { rawValue }
}

struct PreviewItem: Identifiable, Equatable {
    let id = UUID()
    let url: URL
    let title: String
}

struct DownloadedFileItem: Identifiable, Equatable {
    let id = UUID()
    let url: URL
}

struct ParsedServerInput {
    let baseURL: String
    let extractedToken: String?
}

@MainActor
final class AppState: ObservableObject {
    private enum DefaultsKey {
        static let connectionMode = "healthtrail.connectionMode"
        static let remoteBaseURL = "healthtrail.remoteBaseURL"
    }

    @Published var mode: ConnectionMode
    @Published var remoteBaseURL: String
    @Published var remoteToken: String

    @Published var activeURL: URL?
    @Published var webViewReloadToken = UUID()
    @Published var isStartingLocal = false
    @Published var loadError: String?
    @Published var showConnectionSettings = false
    @Published var previewItem: PreviewItem?
    @Published var downloadedFile: DownloadedFileItem?

    let speechManager = SpeechManager()
    private var localURL: URL?
    private var hasStartedOnce = false

    init() {
        let savedModeRaw = UserDefaults.standard.string(forKey: DefaultsKey.connectionMode) ?? ConnectionMode.local.rawValue
        let savedURL = UserDefaults.standard.string(forKey: DefaultsKey.remoteBaseURL) ?? ""
        let savedToken = KeychainStore.loadRemoteToken()

        // If remote mode was saved without a valid URL, fall back to local mode safely.
        if savedModeRaw == ConnectionMode.remote.rawValue, !savedURL.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            self.mode = .remote
        } else {
            self.mode = .local
        }
        self.remoteBaseURL = savedURL
        self.remoteToken = savedToken
    }

    var remoteHostDisplay: String {
        guard mode == .remote, let url = URL(string: remoteBaseURL), let host = url.host else {
            return ""
        }
        if let port = url.port {
            return "\(host):\(port)"
        }
        return host
    }

    var injectedBridgeScript: String {
        let modeJSON = mode.rawValue
        let hostEscaped = remoteHostDisplay
            .replacingOccurrences(of: "\\", with: "\\\\")
            .replacingOccurrences(of: "\"", with: "\\\"")
        return """
        (function() {
          var info = { version: 1, mode: "\(modeJSON)", remoteHost: "\(hostEscaped)", nativeRemoteBanner: true, speech: true };
          window.__HEALTH_TRAIL_IOS__ = info;
          try {
            window.dispatchEvent(new CustomEvent("healthtrail:connectionchange", { detail: info }));
          } catch (e) {}
        })();
        """
    }

    static func localDataDirectory() throws -> URL {
        let fm = FileManager.default
        let appSupport = try fm.url(
            for: .applicationSupportDirectory,
            in: .userDomainMask,
            appropriateFor: nil,
            create: true
        )
        let dir = appSupport.appendingPathComponent("HealthTrail", isDirectory: true)
        if !fm.fileExists(atPath: dir.path) {
            try fm.createDirectory(at: dir, withIntermediateDirectories: true)
        }
        return dir
    }

    static func parseServerInput(_ raw: String) -> ParsedServerInput {
        var trimmed = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else {
            return ParsedServerInput(baseURL: "", extractedToken: nil)
        }
        if !trimmed.contains("://") {
            trimmed = "http://" + trimmed
        }
        guard var components = URLComponents(string: trimmed) else {
            return ParsedServerInput(baseURL: trimmed, extractedToken: nil)
        }

        var token: String?
        if let items = components.queryItems {
            if let t = items.first(where: { $0.name == "token" })?.value, !t.isEmpty {
                token = t
            }
        }
        components.query = nil
        components.fragment = nil
        if components.path == "/" {
            components.path = ""
        } else if components.path.hasSuffix("/") {
            components.path = String(components.path.dropLast())
        }
        let cleanBase = components.string ?? trimmed
        return ParsedServerInput(baseURL: cleanBase, extractedToken: token)
    }

    func startIfNeeded() async {
        guard !hasStartedOnce else { return }
        hasStartedOnce = true
        await activateCurrentMode(forceReload: true)
        #if DEBUG
        if CommandLine.arguments.contains("--open-connection-settings") {
            showConnectionSettings = true
        }
        #endif
    }

    func handleBecomeActive() async {
        guard hasStartedOnce else {
            await startIfNeeded()
            return
        }
        guard mode == .local else { return }

        // Check if the embedded local Go server is still alive and responding after backgrounding.
        if let current = localURL, LocalCoreBridge.isRunning(), await isLocalServerHealthy(baseURL: current) {
            return
        }

        // Restart local server cleanly if iOS reclaimed sockets while suspended.
        LocalCoreBridge.stop()
        localURL = nil
        await startLocalServer(forceReload: true)
    }

    func switchToLocal() async {
        speechManager.stop(emitStopped: false)
        mode = .local
        UserDefaults.standard.set(ConnectionMode.local.rawValue, forKey: DefaultsKey.connectionMode)
        loadError = nil
        await startLocalServer(forceReload: true)
    }

    func saveAndConnectRemote(rawURL: String, token: String) async throws {
        let parsed = Self.parseServerInput(rawURL)
        let finalToken = token.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
            ? (parsed.extractedToken ?? "")
            : token.trimmingCharacters(in: .whitespacesAndNewlines)

        guard buildRemoteLaunchURL(baseURLString: parsed.baseURL) != nil else {
            throw SimpleError(L10n.pick(
                zh: "请输入有效的服务器地址（例如 http://192.168.1.10:8721 或 http://mac.tailnet.ts.net:8721）。",
                en: "Enter a valid server URL (for example http://192.168.1.10:8721 or http://mac.tailnet.ts.net:8721)."
            ))
        }

        // Persist token to Keychain BEFORE switching mode or updating UserDefaults.
        guard KeychainStore.saveRemoteToken(finalToken) else {
            throw SimpleError(L10n.pick(
                zh: "无法将访问令牌保存到 iPhone 钥匙串，请重试。",
                en: "Failed to save access token to the iOS Keychain. Please try again."
            ))
        }

        speechManager.stop(emitStopped: false)
        remoteBaseURL = parsed.baseURL
        remoteToken = finalToken
        mode = .remote

        UserDefaults.standard.set(ConnectionMode.remote.rawValue, forKey: DefaultsKey.connectionMode)
        UserDefaults.standard.set(remoteBaseURL, forKey: DefaultsKey.remoteBaseURL)

        loadError = nil
        await activateCurrentMode(forceReload: true)
    }

    func retryCurrentConnection() async {
        loadError = nil
        if mode == .local {
            if localURL == nil || !LocalCoreBridge.isRunning() {
                await startLocalServer(forceReload: true)
                return
            }
        }
        webViewReloadToken = UUID()
    }

    private func activateCurrentMode(forceReload: Bool) async {
        switch mode {
        case .local:
            await startLocalServer(forceReload: forceReload)
        case .remote:
            BreakReminder.shared.unbind()
            guard let target = buildRemoteLaunchURL(baseURLString: remoteBaseURL) else {
                loadError = L10n.pick(
                    zh: "家用电脑地址无效，请在连接设置中重新填写。",
                    en: "Invalid Home Computer URL. Please update it in Connection Settings."
                )
                return
            }
            activeURL = target
            if forceReload {
                webViewReloadToken = UUID()
            }
        }
    }

    private func startLocalServer(forceReload: Bool) async {
        if let existing = localURL, LocalCoreBridge.isRunning(), await isLocalServerHealthy(baseURL: existing) {
            activeURL = existing
            loadError = nil
            BreakReminder.shared.bind(baseURL: existing)
            if forceReload {
                webViewReloadToken = UUID()
            }
            return
        }

        isStartingLocal = true
        loadError = nil
        defer { isStartingLocal = false }

        do {
            let dir = try Self.localDataDirectory()
            let startedURL = try await Task.detached(priority: .userInitiated) {
                try LocalCoreBridge.start(dataDir: dir.path)
            }.value
            localURL = startedURL
            activeURL = startedURL
            loadError = nil
            BreakReminder.shared.bind(baseURL: startedURL)
            if forceReload {
                webViewReloadToken = UUID()
            }
        } catch {
            loadError = error.localizedDescription
        }
    }

    func armBreakReminder() {
        guard mode == .local else { return }
        BreakReminder.shared.syncNow()
    }

    private func isLocalServerHealthy(baseURL: URL) async -> Bool {
        let healthURL = baseURL.appendingPathComponent("api/health")
        var req = URLRequest(url: healthURL)
        req.timeoutInterval = 2.0
        req.cachePolicy = .reloadIgnoringLocalAndRemoteCacheData
        do {
            let (_, response) = try await URLSession.shared.data(for: req)
            if let http = response as? HTTPURLResponse {
                return (200..<300).contains(http.statusCode)
            }
            return false
        } catch {
            return false
        }
    }

    private func buildRemoteLaunchURL(baseURLString: String) -> URL? {
        let parsed = Self.parseServerInput(baseURLString)
        guard !parsed.baseURL.isEmpty, var comps = URLComponents(string: parsed.baseURL), comps.host != nil else {
            return nil
        }
        if comps.path.isEmpty {
            comps.path = "/"
        }
        // Keep URL clean without ?token=; WebViewContainer injects the hv_token HttpOnly-compatible cookie via WKHTTPCookieStore.
        comps.queryItems = nil
        return comps.url
    }

    static var atsBlockedMessage: String {
        L10n.pick(
            zh: "iOS 阻止了明文 HTTP 连接：局域网请使用 http://192.168.x.x:端口，Tailscale 请使用 MagicDNS（http://*.ts.net:端口）或 HTTPS（tailscale serve）。",
            en: "iOS blocked plaintext HTTP for this address. Use a LAN IP (http://192.168.x.x:port), Tailscale MagicDNS (http://*.ts.net:port), or HTTPS (tailscale serve)."
        )
    }

    func testRemoteConnection(rawURL: String, token: String) async -> Result<String, Error> {
        let parsed = Self.parseServerInput(rawURL)
        let effectiveToken = token.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
            ? (parsed.extractedToken ?? "")
            : token.trimmingCharacters(in: .whitespacesAndNewlines)

        guard !parsed.baseURL.isEmpty, let base = URL(string: parsed.baseURL), base.host != nil else {
            return .failure(SimpleError(L10n.pick(
                zh: "请输入有效的服务器地址（例如 http://192.168.1.10:8721）",
                en: "Enter a valid server URL (for example http://192.168.1.10:8721)"
            )))
        }

        let infoURL = base.appendingPathComponent("api/info")
        var req = URLRequest(url: infoURL)
        req.httpMethod = "GET"
        req.timeoutInterval = 6.0
        req.cachePolicy = .reloadIgnoringLocalAndRemoteCacheData
        if !effectiveToken.isEmpty {
            req.setValue("Bearer \(effectiveToken)", forHTTPHeaderField: "Authorization")
        }

        do {
            let (data, response) = try await URLSession.shared.data(for: req)
            guard let http = response as? HTTPURLResponse else {
                return .failure(SimpleError(L10n.pick(
                    zh: "收到无效的服务器响应",
                    en: "Received an invalid response from the server"
                )))
            }
            if http.statusCode == 401 || http.statusCode == 403 {
                return .failure(SimpleError(L10n.pick(
                    zh: "访问令牌无效或缺失，请从电脑端「连接」页面复制最新访问链接或令牌。",
                    en: "Access token is missing or invalid. Copy the phone link or token from Connect AI on your computer."
                )))
            }
            guard (200..<300).contains(http.statusCode) else {
                return .failure(SimpleError(L10n.pick(
                    zh: "服务器返回错误状态码：\(http.statusCode)",
                    en: "Server returned HTTP status \(http.statusCode)"
                )))
            }
            // Verify the response is JSON from Health Trail
            if (try? JSONSerialization.jsonObject(with: data)) == nil {
                return .failure(SimpleError(L10n.pick(
                    zh: "该地址不是健康足迹服务，请检查端口号。",
                    en: "This address does not appear to be a Health Trail server. Check the port."
                )))
            }
            return .success(L10n.pick(
                zh: "连接成功！已验证家用电脑上的健康足迹服务。",
                en: "Connected! Verified Health Trail on your Home Computer."
            ))
        } catch let urlError as URLError {
            let message: String
            switch urlError.code {
            case .appTransportSecurityRequiresSecureConnection:
                message = Self.atsBlockedMessage
            case .timedOut:
                message = L10n.pick(
                    zh: "连接超时：请确认电脑已开机、健康足迹正在运行，且手机与电脑在同一 Wi-Fi 或 Tailscale 网络中。",
                    en: "Connection timed out. Make sure your computer is awake, Health Trail is running, and both devices are on the same Wi-Fi or Tailscale network."
                )
            case .cannotConnectToHost, .cannotFindHost:
                message = L10n.pick(
                    zh: "无法连接到该地址：请检查 IP、端口号，或是否已开启「手机访问」。",
                    en: "Cannot reach this address. Check the IP/port and make sure phone access is enabled on your computer."
                )
            case .notConnectedToInternet, .networkConnectionLost:
                message = L10n.pick(
                    zh: "当前无网络连接，请检查 Wi-Fi 或蜂窝网络。",
                    en: "No network connection. Check your Wi-Fi or cellular settings."
                )
            default:
                message = urlError.localizedDescription
            }
            return .failure(SimpleError(message))
        } catch {
            return .failure(error)
        }
    }
}

private struct SimpleError: LocalizedError {
    let message: String
    init(_ message: String) { self.message = message }
    var errorDescription: String? { message }
}
